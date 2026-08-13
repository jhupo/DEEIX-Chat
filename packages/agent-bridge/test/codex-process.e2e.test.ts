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
	"pinned Codex app-server completes the real initialize handshake",
	{ skip: executable ? false : "CODEX_E2E_EXECUTABLE is not set" },
	async () => {
		assert.ok(executable);
		await assertCodexVersion(executable);
		const process = startCodexAppServer(executable);
		const adapter = new CodexAdapter({
			profileId: "profile_e2e",
			rpc: process.rpc,
			sources: new SourceRefRegistry(),
			closeProcess: process.close,
		});
		const manifest = await adapter.start(
			async () => undefined,
			AbortSignal.timeout(15_000),
		);
		assert.equal(manifest.runtimeVersion, "0.147.0");
		assert.equal(manifest.protocolVersion, "0.147.0/stable");
		await adapter.close();
	},
);
