import { createHash } from "node:crypto";
import { mkdir, open, rename, rm } from "node:fs/promises";
import { basename, extname, join } from "node:path";
import type { WorkspaceRegistry } from "../config/workspace-registry.js";
import type { AgentCommand } from "../protocol/agent-command.js";
import type { ArtifactGrant } from "../protocol/bridge-envelope.js";

export class ArtifactDownloader {
	readonly #baseUrl: string;
	readonly #workspaces: WorkspaceRegistry;
	readonly #fetch: typeof fetch;

	constructor(baseUrl: string, workspaces: WorkspaceRegistry, fetchImplementation: typeof fetch = fetch) {
		this.#baseUrl = baseUrl;
		this.#workspaces = workspaces;
		this.#fetch = fetchImplementation;
	}

	async prepare(commandId: string, command: AgentCommand, grants: readonly ArtifactGrant[], signal: AbortSignal): Promise<Map<string, { path: string; mimeType: string }>> {
		if (command.kind !== "turn.start" && command.kind !== "turn.steer") {
			if (grants.length > 0) throw new Error("command does not accept artifacts");
			return new Map();
		}
		const refs = [...new Set(command.input.filter((item) => item.kind === "artifact").map((item) => item.artifactRef))];
		if (refs.length !== grants.length) throw new Error("artifact grants do not match command input");
		const byRef = new Map(grants.map((grant) => [grant.artifactRef, grant]));
		const result = new Map<string, { path: string; mimeType: string }>();
		for (const ref of refs) {
			const grant = byRef.get(ref);
			if (!grant) throw new Error(`artifact grant is missing: ${ref}`);
			const root = await this.#workspaces.resolvePath(command.workspaceId, ".deeix/artifacts");
			await mkdir(root, { recursive: true, mode: 0o700 });
			const extension = extname(basename(grant.fileName)).slice(0, 16);
			const target = join(root, `${grant.artifactRef}${extension}`);
			const temporary = `${target}.partial`;
			const url = new URL(`/api/v1/agent/bridge/artifacts/${encodeURIComponent(ref)}/content`, this.#baseUrl);
			url.searchParams.set("command", commandId);
			url.searchParams.set("expires", grant.expiresAt);
			try {
				const response = await this.#fetch(url, {
					signal,
					headers: { authorization: `Bearer deeix_artifact_${grant.grant}` },
				});
				if (!response.ok || !response.body) throw new Error(`artifact download failed (${response.status})`);
				const file = await open(temporary, "w", 0o600);
				const hash = createHash("sha256");
				let size = 0;
				try {
					for await (const chunk of response.body) {
						const bytes = Buffer.from(chunk);
						size += bytes.length;
						if (size > grant.sizeBytes) throw new Error("artifact exceeds declared size");
						hash.update(bytes);
						await file.write(bytes);
					}
					await file.sync();
				} finally {
					await file.close();
				}
				if (size !== grant.sizeBytes || hash.digest("hex") !== grant.sha256)
					throw new Error("artifact integrity check failed");
				await rm(target, { force: true });
				await rename(temporary, target);
			} catch (error) {
				await rm(temporary, { force: true });
				throw error;
			}
			result.set(ref, { path: target, mimeType: grant.mimeType });
		}
		return result;
	}
}
