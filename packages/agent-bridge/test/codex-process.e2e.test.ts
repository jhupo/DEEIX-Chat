import assert from "node:assert/strict";
import test from "node:test";
import { SourceRefRegistry } from "../src/commands/resolve-provider-command.js";
import { CodexAdapter } from "../src/providers/codex/codex-adapter.js";
import {
	assertCodexVersion,
	startCodexAppServer,
} from "../src/providers/codex/codex-process.js";

const executable = process.env.CODEX_E2E_EXECUTABLE;

test(
	"pinned Codex app-server initializes and serves locked resources",
	{ skip: executable ? false : "CODEX_E2E_EXECUTABLE is not set" },
	async () => {
		assert.ok(executable);
		await assertCodexVersion(executable);
		const appServer = startCodexAppServer(executable);
		const adapter = new CodexAdapter({
			profileId: "profile_e2e",
			rpc: appServer.rpc,
			sources: new SourceRefRegistry(),
			closeProcess: appServer.close,
		});
		try {
			const manifest = await adapter.start(
				async () => undefined,
				AbortSignal.timeout(15_000),
			);
			assert.equal(manifest.runtimeVersion, "0.147.0");
			assert.equal(manifest.protocolVersion, "0.147.0/stable");

			for (const resource of [
				"models",
				"model-capabilities",
				"permission-profiles",
				"apps",
				"mcp",
				"plugins",
				"auth-status",
			] as const) {
				const result = await adapter.execute(
					{
						commandId: `command_e2e_${resource}`,
						profileRef: "profile_e2e",
						kind: "resource.refresh",
						resource: { scope: "profile", name: resource },
					},
					AbortSignal.timeout(15_000),
				);
				assert.equal(result.kind, "resource");
				assert.equal(result.resource, resource);
			}

			for (const resource of ["sessions", "skills", "hooks"] as const) {
				const result = await adapter.execute(
					{
						commandId: `command_e2e_${resource}`,
						profileRef: "profile_e2e",
						kind: "resource.refresh",
						canonicalCwd: process.cwd(),
						resource: { scope: "workspace", name: resource },
					},
					AbortSignal.timeout(15_000),
				);
				assert.equal(result.kind, "resource");
				assert.equal(result.resource, resource);
			}
		} finally {
			await adapter.close();
		}
	},
);
