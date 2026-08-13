import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { mkdtemp, readFile, readdir, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { WorkspaceRegistry } from "../src/config/workspace-registry.js";
import type { AgentCommand } from "../src/protocol/agent-command.js";
import { ArtifactDownloader } from "../src/transport/artifact-downloader.js";
import { CommandJournal } from "../src/wal/command-journal.js";
import { DurableWalStore } from "../src/wal/durable-wal-store.js";

test("artifact downloader verifies content and keeps grants out of the command WAL", async (context) => {
	const directory = await mkdtemp(join(tmpdir(), "deeix-artifact-"));
	context.after(() => rm(directory, { recursive: true, force: true }));
	const content = Buffer.from("image fixture");
	const sha256 = createHash("sha256").update(content).digest("hex");
	const grant = "A".repeat(43);
	const workspaces = new WorkspaceRegistry();
	await workspaces.register("workspace_1", directory);
	const downloader = new ArtifactDownloader(
		"https://deeix.test",
		workspaces,
		async (_input, init) => {
			assert.equal(new Headers(init?.headers).get("authorization"), `Bearer deeix_artifact_${grant}`);
			return new Response(content, { status: 200 });
		},
	);
	const command: AgentCommand = {
		kind: "turn.start",
		deviceId: "device_1",
		profileId: "profile_1",
		workspaceId: "workspace_1",
		threadId: "thread_1",
		sourceThreadRef: "thread_ref",
		input: [{ kind: "artifact", artifactRef: "agart_0123456789abcdef0123456789abcdef" }],
		settings: {},
	};
	const wal = await DurableWalStore.open(join(directory, "wal"));
	await CommandJournal.restore(wal).receive(1, "command_1", command);
	const prepared = await downloader.prepare("command_1", command, [{
		artifactRef: "agart_0123456789abcdef0123456789abcdef",
		fileName: "fixture.png",
		mimeType: "image/png",
		sizeBytes: content.length,
		sha256,
		expiresAt: new Date(Date.now() + 60_000).toISOString(),
		grant,
	}], AbortSignal.timeout(1000));
	const artifact = prepared.get("agart_0123456789abcdef0123456789abcdef");
	assert.ok(artifact);
	assert.equal(artifact.mimeType, "image/png");
	assert.deepEqual(await readFile(artifact.path), content);
	wal.close();
	const walContent = await Promise.all(
		(await readdir(join(directory, "wal"))).map((name) =>
			readFile(join(directory, "wal", name), "utf8"),
		),
	);
	assert.equal(walContent.some((value) => value.includes(grant)), false);
});
