import assert from "node:assert/strict";
import test from "node:test";
import { parseBridgeConfig } from "../src/config/bridge-config.js";

test("bridge configuration pins identity and workspaces", () => {
	const config = parseBridgeConfig({
		version: 1,
		cloudUrl: "https://deeix.example/",
		userPublicID: "f6f910e920934def9a5cda479fc25251",
		deviceId: "agd_0123456789abcdef0123456789abcdef",
		profileId: "codex-default",
		codexExecutable: "codex",
		workspaces: [{ workspaceId: "workspace-main", root: process.cwd(), name: "main" }],
	});
	assert.equal(config.cloudUrl, "https://deeix.example");
	assert.equal(config.workspaces[0]?.workspaceId, "workspace-main");
	assert.throws(() => parseBridgeConfig({ ...config, workspaces: [] }), /workspaces/);
});
