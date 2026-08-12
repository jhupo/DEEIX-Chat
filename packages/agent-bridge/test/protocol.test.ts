import assert from "node:assert/strict";
import test from "node:test";
import { parseAgentCommand } from "../src/protocol/agent-command.js";

const turnStart = {
	kind: "turn.start",
	deviceId: "device_1",
	profileId: "profile_1",
	workspaceId: "workspace_1",
	threadId: "thread_1",
	sourceThreadRef: "source_thread_1",
	input: [{ kind: "text", text: "inspect the workspace" }],
	settings: {
		model: "gpt-5.6-sol",
		reasoningEffort: "high",
		sandboxPolicy: "workspace-write",
	},
};

test("parseAgentCommand returns a normalized discriminated command", () => {
	assert.deepEqual(parseAgentCommand(turnStart), turnStart);
});

test("parseAgentCommand rejects provider protocol and local path fields", () => {
	assert.throws(
		() =>
			parseAgentCommand({
				...turnStart,
				method: "turn/start",
				cwd: "C:\\private",
				providerThreadId: "raw",
			}),
		/unexpected field/,
	);
});

test("parseAgentCommand validates interaction scope fields", () => {
	assert.throws(
		() =>
			parseAgentCommand({
				kind: "interaction.respond",
				scope: "thread",
				deviceId: "device_1",
				profileId: "profile_1",
				workspaceId: "workspace_1",
				threadId: "thread_1",
				sourceThreadRef: "source_thread_1",
				turnId: "turn_1",
				interactionId: "interaction_1",
				sourceRequestRef: "request_1",
				response: { kind: "approval", decision: "accept" },
			}),
		/unexpected field: turnId/,
	);
});
