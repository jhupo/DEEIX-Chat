import assert from "node:assert/strict";
import { PassThrough } from "node:stream";
import test from "node:test";
import { SourceRefRegistry } from "../src/commands/resolve-provider-command.js";
import { CodexAdapter } from "../src/providers/codex/codex-adapter.js";
import { JsonLineRpcClient } from "../src/providers/codex/rpc-client.js";

test("Codex adapter initializes, maps IDs, redacts auth tokens, and resolves approvals", async () => {
	const input = new PassThrough();
	const output = new PassThrough();
	const sent = lineReader(input);
	const sources = new SourceRefRegistry();
	const events: Array<{ kind: string; sourceRequestRef?: string; payload: unknown }> = [];
	const rpc = new JsonLineRpcClient(input, output);
	const adapter = new CodexAdapter({
		profileId: "profile_1",
		rpc,
		sources,
	});

	const started = adapter.start(async (event) => {
		events.push(event);
	}, AbortSignal.timeout(1000));
	const initialize = await sent.next();
	assert.equal(initialize.method, "initialize");
	assert.deepEqual(initialize.params.capabilities, {
		experimentalApi: false,
		requestAttestation: false,
		mcpServerOpenaiFormElicitation: true,
	});
	respond(output, initialize.id, {
		userAgent: "codex-cli/0.147.0",
		codexHome: "/home/test/.codex",
		platformFamily: "unix",
		platformOs: "linux",
	});
	const initialized = await sent.next();
	assert.deepEqual(initialized, { method: "initialized" });
	assert.equal((await started).schemaHash.length, 64);

	const create = adapter.execute(
		{
			kind: "thread.create",
			commandId: "command_1",
			profileRef: "profile_1",
			canonicalCwd: "/work/project",
			settings: { model: "gpt-5.6-sol", sandboxPolicy: "workspace-write" },
		},
		AbortSignal.timeout(1000),
	);
	const threadStart = await sent.next();
	assert.equal(threadStart.method, "thread/start");
	assert.equal(threadStart.params.cwd, "/work/project");
	respond(output, threadStart.id, { thread: { id: "provider-thread-1" } });
	const created = await create;
	assert.equal(created.kind, "thread-created");
	if (created.kind !== "thread-created") throw new Error("expected thread result");
	assert.equal(
		sources.resolve("profile_1", "thread", created.sourceThreadRef),
		"provider-thread-1",
	);
	assert.equal(
		await sources.publish("profile_1", "thread", "provider-thread-1"),
		created.sourceThreadRef,
	);

	const metadata = adapter.execute({
		kind: "thread.metadata.update",
		commandId: "command_metadata",
		profileRef: "profile_1",
		canonicalCwd: "/work/project",
		providerThreadId: "provider-thread-1",
		gitInfo: { sha: null, branch: "main" },
	}, AbortSignal.timeout(1000));
	const metadataRequest = await sent.next();
	assert.equal(metadataRequest.method, "thread/metadata/update");
	assert.deepEqual(metadataRequest.params, {
		threadId: "provider-thread-1",
		gitInfo: { sha: null, branch: "main" },
	});
	respond(output, metadataRequest.id, {});
	assert.deepEqual(await metadata, { kind: "accepted" });

	const auth = adapter.execute(
		{
			kind: "resource.refresh",
			commandId: "command_2",
			profileRef: "profile_1",
			resource: { scope: "profile", name: "auth-status" },
		},
		AbortSignal.timeout(1000),
	);
	const authRequest = await sent.next();
	assert.deepEqual(authRequest.params, { includeToken: false, refreshToken: false });
	respond(output, authRequest.id, {
		authMethod: "apikey",
		authToken: "must-not-leave-device",
		requiresOpenaiAuth: false,
	});
	assert.deepEqual(await auth, {
		kind: "resource",
		resource: "auth-status",
		data: { authMethod: "apikey", requiresOpenaiAuth: false },
	});

	const sessions = adapter.execute(
		{
			kind: "resource.refresh",
			commandId: "command_sessions",
			profileRef: "profile_1",
			canonicalCwd: "/work/project",
			resource: { scope: "workspace", name: "sessions" },
		},
		AbortSignal.timeout(1000),
	);
	const sessionRequest = await sent.next();
	assert.equal(sessionRequest.method, "thread/list");
	assert.equal(sessionRequest.params.cwd, "/work/project");
	respond(output, sessionRequest.id, {
		data: [{
			id: "provider-thread-private",
			sessionId: "provider-session-private",
			forkedFromId: null,
			parentThreadId: null,
			preview: "Inspect the repository",
			ephemeral: false,
			section: null,
			sectionEnteredAt: null,
			modelProvider: "openai",
			createdAt: 1,
			updatedAt: 2,
			recencyAt: 2,
			status: { type: "idle" },
			path: "/home/test/.codex/sessions/private.jsonl",
			cwd: "/work/project",
			cliVersion: "0.147.0",
			source: "appServer",
			threadSource: null,
			agentNickname: null,
			agentRole: null,
			gitInfo: null,
			name: "Local session",
			turns: [],
		}],
		nextCursor: null,
		backwardsCursor: null,
	});
	const sessionReadRequest = await sent.next();
	assert.equal(sessionReadRequest.method, "thread/read");
	assert.deepEqual(sessionReadRequest.params, {
		threadId: "provider-thread-private",
		includeTurns: true,
	});
	respond(output, sessionReadRequest.id, {
		thread: {
			id: "provider-thread-private",
			turns: [{
				id: "provider-turn-private",
				items: [
					{ type: "userMessage", id: "private-user", clientId: null, content: [{ type: "text", text: "Inspect the repository", text_elements: [] }] },
					{ type: "reasoning", id: "private-reasoning", summary: ["Check the files"], content: [] },
					{ type: "agentMessage", id: "private-agent", text: "The repository is ready.", phase: null, memoryCitation: null },
				],
				itemsView: "full", status: "completed", error: null,
				startedAt: 1, completedAt: 2, durationMs: 1000,
			}],
		},
	});
	const sessionResult = await sessions;
	assert.equal(sessionResult.kind, "resource");
	const serializedSessions = JSON.stringify(sessionResult);
	assert.equal(serializedSessions.includes("provider-thread-private"), false);
	assert.equal(serializedSessions.includes("/work/project"), false);
	assert.equal(serializedSessions.includes("/home/test"), false);
	if (sessionResult.kind !== "resource") throw new Error("expected resource result");
	const projectedSessions = sessionResult.data as {
		data: Array<{ sourceThreadRef: string; messages: Array<{ role: string; content: string; reasoningContent?: string }> }>;
	};
	assert.equal(
		sources.resolve("profile_1", "thread", projectedSessions.data[0]!.sourceThreadRef),
		"provider-thread-private",
	);
	assert.deepEqual(projectedSessions.data[0]!.messages, [
		{ role: "user", content: "Inspect the repository", createdAt: 1 },
		{ role: "assistant", content: "The repository is ready.", reasoningContent: "Check the files", createdAt: 2 },
	]);

	const turn = adapter.execute({
		kind: "turn.start",
		commandId: "command_turn",
		profileRef: "profile_1",
		canonicalCwd: "/work/project",
		providerThreadId: "provider-thread-1",
		input: [{ kind: "text", text: "continue" }],
		settings: {},
	}, AbortSignal.timeout(1000));
	const turnRequest = await sent.next();
	assert.equal(turnRequest.method, "turn/start");
	assert.equal(turnRequest.params.threadId, "provider-thread-1");
	respond(output, turnRequest.id, { turn: { id: "provider-turn-2" } });
	assert.equal((await turn).kind, "turn-started");

	const restoredTurn = adapter.execute({
		kind: "turn.start",
		commandId: "command_restored_turn",
		profileRef: "profile_1",
		canonicalCwd: "/work/project",
		providerThreadId: "provider-thread-restored",
		input: [{ kind: "text", text: "continue restored thread" }],
		settings: {},
	}, AbortSignal.timeout(1000));
	const resumeRequest = await sent.next();
	assert.equal(resumeRequest.method, "thread/resume");
	assert.deepEqual(resumeRequest.params, { threadId: "provider-thread-restored", cwd: "/work/project" });
	respond(output, resumeRequest.id, { thread: { id: "provider-thread-restored" } });
	const restoredTurnRequest = await sent.next();
	assert.equal(restoredTurnRequest.method, "turn/start");
	assert.equal(restoredTurnRequest.params.threadId, "provider-thread-restored");
	respond(output, restoredTurnRequest.id, { turn: { id: "provider-turn-restored" } });
	assert.equal((await restoredTurn).kind, "turn-started");

	output.write(JSON.stringify({
		method: "thread/closed",
		params: { threadId: "provider-thread-1" },
	}) + "\n");
	await eventually(() => events.length === 1);
	assert.equal(events.shift()?.kind, "thread/closed");
	const resumedAfterClose = adapter.execute({
		kind: "turn.start",
		commandId: "command_resumed_after_close",
		profileRef: "profile_1",
		canonicalCwd: "/work/project",
		providerThreadId: "provider-thread-1",
		input: [{ kind: "text", text: "continue after close" }],
		settings: {},
	}, AbortSignal.timeout(1000));
	const resumeAfterCloseRequest = await sent.next();
	assert.equal(resumeAfterCloseRequest.method, "thread/resume");
	respond(output, resumeAfterCloseRequest.id, { thread: { id: "provider-thread-1" } });
	const resumedTurnRequest = await sent.next();
	assert.equal(resumedTurnRequest.method, "turn/start");
	respond(output, resumedTurnRequest.id, { turn: { id: "provider-turn-after-close" } });
	assert.equal((await resumedAfterClose).kind, "turn-started");

	const challenge = [
		"deeix-runtime-auth-proof-v1",
		"f6f910e920934def9a5cda479fc25251",
		"agd_f6f910e920934def9a5cda479fc25251",
		"profile_1",
		"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		"nonce",
		"1786550460",
	].join("\n");
	const proofPending = adapter.proveRuntimeAuth(challenge, AbortSignal.timeout(1000));
	const proofRequest = await sent.next();
	assert.equal(proofRequest.method, "getAuthStatus");
	assert.deepEqual(proofRequest.params, { includeToken: true, refreshToken: false });
	respond(output, proofRequest.id, {
		authMethod: "apikey",
		authToken: "must-not-leave-device",
		requiresOpenaiAuth: false,
	});
	assert.match(await proofPending, /^[A-Za-z0-9_-]{43}$/);

	output.write(
		`${JSON.stringify({
			id: 41,
			method: "item/commandExecution/requestApproval",
			params: {
				threadId: "provider-thread-1",
				turnId: "provider-turn-1",
				itemId: "provider-item-1",
				command: "git status",
			},
		})}\n`,
	);
	await eventually(() => events.length === 1);
	const interaction = events[0];
	assert.equal(interaction?.kind, "interaction.requested");
	assert.equal(
		JSON.stringify(interaction?.payload).includes("provider-thread-1"),
		false,
	);
	const sourceRequestRef = interaction?.sourceRequestRef;
	assert.ok(sourceRequestRef);
	await adapter.execute(
		{
			kind: "interaction.respond",
			commandId: "command_3",
			profileRef: "profile_1",
			canonicalCwd: "/work/project",
			providerThreadId: "provider-thread-1",
			providerTurnId: "provider-turn-1",
			providerRequestId: sources.resolve("profile_1", "request", sourceRequestRef),
			scope: "turn",
			response: { kind: "approval", decision: "accept" },
		},
		AbortSignal.timeout(1000),
	);
	assert.deepEqual(await sent.next(), { id: 41, result: { decision: "accept" } });
	await adapter.close();
});

test("Codex notification policy maps, redacts extensions, and drops disabled methods", async () => {
	const input = new PassThrough();
	const output = new PassThrough();
	const sent = lineReader(input);
	const events: Array<{
		kind: string;
		occurredAt: string;
		payload: Record<string, unknown>;
	}> = [];
	const adapter = new CodexAdapter({
		profileId: "profile_1",
		rpc: new JsonLineRpcClient(input, output),
		sources: new SourceRefRegistry(),
	});
	const started = adapter.start(async (event) => {
		events.push(event);
	}, AbortSignal.timeout(1000));
	const initialize = await sent.next();
	respond(output, initialize.id, {
		userAgent: "codex-cli/0.147.0",
		codexHome: "/home/test/.codex",
		platformFamily: "unix",
		platformOs: "linux",
	});
	await sent.next();
	await started;

	output.write(`${JSON.stringify({ method: "warning", params: { message: "mapped" } })}\n`);
	output.write(`${JSON.stringify({
		method: "deprecationNotice",
		params: { message: "extension", token: "redacted" },
	})}\n`);
	output.write(`${JSON.stringify({
		method: "thread/realtime/started",
		params: { threadId: "private-thread" },
	})}\n`);
	await eventually(() => events.length === 2);
	assert.equal(events[0]?.kind, "warning");
	assert.deepEqual(events[0]?.payload, { message: "mapped" });
	assert.equal(events[1]?.kind, "provider.extension");
	assert.deepEqual(events[1]?.payload, {
		method: "deprecationNotice",
		data: { message: "extension" },
	});
	assert.ok(events.every((event) => !Number.isNaN(Date.parse(event.occurredAt))));
	await adapter.close();
});

function lineReader(stream: PassThrough): { next: () => Promise<Record<string, any>> } {
	const lines: string[] = [];
	const waiters: Array<(value: string) => void> = [];
	let buffer = "";
	stream.setEncoding("utf8");
	stream.on("data", (chunk: string) => {
		buffer += chunk;
		let newline = buffer.indexOf("\n");
		while (newline >= 0) {
			const line = buffer.slice(0, newline);
			buffer = buffer.slice(newline + 1);
			const waiter = waiters.shift();
			if (waiter) waiter(line);
			else lines.push(line);
			newline = buffer.indexOf("\n");
		}
	});
	return {
		next: async () => {
			const line = lines.shift() ?? (await new Promise<string>((resolve) => waiters.push(resolve)));
			return JSON.parse(line) as Record<string, any>;
		},
	};
}

function respond(stream: PassThrough, id: number, result: unknown): void {
	stream.write(`${JSON.stringify({ id, result })}\n`);
}

async function eventually(predicate: () => boolean): Promise<void> {
	for (let index = 0; index < 100; index += 1) {
		if (predicate()) return;
		await new Promise((resolve) => setTimeout(resolve, 5));
	}
	assert.fail("condition was not met");
}
