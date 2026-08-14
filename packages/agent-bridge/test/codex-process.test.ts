import assert from "node:assert/strict";
import test from "node:test";
import { parseCodexVersion } from "../src/providers/codex/codex-process.js";

test("Codex version parser preserves the locked desktop prerelease suffix", () => {
	assert.equal(parseCodexVersion("codex-cli 0.147.0\n"), "0.147.0");
	assert.equal(
		parseCodexVersion("codex-cli 0.147.0-alpha.6.6\r\n"),
		"0.147.0-alpha.6.6",
	);
	assert.equal(parseCodexVersion("codex-cli unknown\n"), undefined);
});
