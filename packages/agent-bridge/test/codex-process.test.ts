import assert from "node:assert/strict";
import test from "node:test";
import {
	BUNDLED_CODEX_EXECUTABLE,
	parseCodexVersion,
	resolveCodexExecutable,
} from "../src/providers/codex/codex-process.js";

test("Codex version parser preserves the locked desktop prerelease suffix", () => {
	assert.equal(parseCodexVersion("codex-cli 0.147.0\n"), "0.147.0");
	assert.equal(
		parseCodexVersion("codex-cli 0.147.0-alpha.6.6\r\n"),
		"0.147.0-alpha.6.6",
	);
	assert.equal(parseCodexVersion("codex-cli unknown\n"), undefined);
});

test("bundled Codex resolves inside the Agent Bridge package", () => {
	const executable = resolveCodexExecutable(BUNDLED_CODEX_EXECUTABLE);
	assert.match(executable.replaceAll("\\", "/"), /\/codex\/bin\/codex(?:\.exe)?$/);
	assert.equal(resolveCodexExecutable("/opt/codex/bin/codex"), "/opt/codex/bin/codex");
});
