import assert from "node:assert/strict";
import test from "node:test";
import { SourceRefRegistry } from "../src/commands/resolve-provider-command.js";
import type { ProviderEvent } from "../src/providers/provider-adapter.js";
import { CodexAdapter } from "../src/providers/codex/codex-adapter.js";
import {
	assertCodexVersion,
	startCodexAppServer,
} from "../src/providers/codex/codex-process.js";
import type { ThreadReadResponse } from "../generated/codex-app-server-v0.147.0/ts/v2/ThreadReadResponse.js";
import type { AppsInstalledResponse } from "../generated/codex-app-server-v0.147.0/ts/v2/AppsInstalledResponse.js";
import type { CommandExecResponse } from "../generated/codex-app-server-v0.147.0/ts/v2/CommandExecResponse.js";
import type { ConfigReadResponse } from "../generated/codex-app-server-v0.147.0/ts/v2/ConfigReadResponse.js";
import type { FsGetMetadataResponse } from "../generated/codex-app-server-v0.147.0/ts/v2/FsGetMetadataResponse.js";
import type { ThreadLoadedListResponse } from "../generated/codex-app-server-v0.147.0/ts/v2/ThreadLoadedListResponse.js";

const executable = process.env.CODEX_E2E_EXECUTABLE;
const authenticated = executable && process.env.CODEX_E2E_AUTHENTICATED === "1";

test(
	"pinned Codex app-server initializes without an authenticated upstream",
	{ skip: executable ? false : "CODEX_E2E_EXECUTABLE is not set" },
	async () => {
		assert.ok(executable);
		await assertCodexVersion(executable);
		const appServer = startCodexAppServer(executable);
		const adapter = new CodexAdapter({
			profileId: "profile_smoke",
			rpc: appServer.rpc,
			sources: new SourceRefRegistry(),
			closeProcess: appServer.close,
		});
		try {
			const manifest = await adapter.start(async () => undefined, AbortSignal.timeout(15_000));
			assert.equal(manifest.runtimeVersion, "0.147.0");
			assert.ok(manifest.commands.includes("turn.start"));
			const auth = await adapter.execute(
				{
					commandId: "command_smoke_auth_status",
					profileRef: "profile_smoke",
					kind: "resource.refresh",
					resource: { scope: "profile", name: "auth-status" },
				},
				AbortSignal.timeout(15_000),
			);
			assert.equal(auth.kind, "resource");
		} finally {
			await adapter.close();
		}
	},
);

test(
	"pinned Codex app-server initializes, serves resources, and completes a thread lifecycle",
	{ skip: authenticated ? false : "authenticated Codex E2E is not enabled" },
	async () => {
		assert.ok(executable);
		await assertCodexVersion(executable);
		const appServer = startCodexAppServer(executable);
		const sources = new SourceRefRegistry();
		const events: ProviderEvent[] = [];
		const createdThreadIds = new Set<string>();
		const adapter = new CodexAdapter({
			profileId: "profile_e2e",
			rpc: appServer.rpc,
			sources,
			closeProcess: appServer.close,
		});
		try {
			const manifest = await adapter.start(
				async (event) => {
					events.push(event);
				},
				AbortSignal.timeout(15_000),
			);
			assert.equal(manifest.runtimeVersion, "0.147.0");
			assert.equal(manifest.protocolVersion, "0.147.0/stable");
			const proof = await adapter.proveRuntimeAuth(
				[
					"deeix-runtime-auth-proof-v1",
					"00000000000000000000000000000000",
					"agd_00000000000000000000000000000000",
					"profile_e2e",
					"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
					"e2e-nonce",
					"1786550460",
				].join("\n"),
				AbortSignal.timeout(15_000),
			);
			assert.match(proof, /^[A-Za-z0-9_-]{43}$/);

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

			for (const resource of ["skills", "hooks"] as const) {
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

			const account = await appServer.rpc.request<{ account: unknown; requiresOpenaiAuth: boolean }>(
				"account/read",
				{ refreshToken: false },
				AbortSignal.timeout(15_000),
			);
			assert.equal(typeof account.requiresOpenaiAuth, "boolean");
			const config = await appServer.rpc.request<ConfigReadResponse>(
				"config/read",
				{ includeLayers: false, cwd: process.cwd() },
				AbortSignal.timeout(15_000),
			);
			assert.equal(typeof config.config, "object");
			const installedApps = await appServer.rpc.request<AppsInstalledResponse>(
				"app/installed",
				{ forceRefresh: false },
				AbortSignal.timeout(15_000),
			);
			assert.ok(Array.isArray(installedApps.apps));
			const metadata = await appServer.rpc.request<FsGetMetadataResponse>(
				"fs/getMetadata",
				{ path: process.cwd() },
				AbortSignal.timeout(15_000),
			);
			assert.equal(metadata.isDirectory, true);
			const command = await appServer.rpc.request<CommandExecResponse>(
				"command/exec",
				{
					command: [process.execPath, "-e", "process.stdout.write('DEEIX_COMMAND_E2E_OK')"],
					cwd: process.cwd(),
					timeoutMs: 15_000,
					sandboxPolicy: { type: "readOnly", networkAccess: false },
				},
				AbortSignal.timeout(30_000),
			);
			assert.equal(command.exitCode, 0);
			assert.equal(command.stdout, "DEEIX_COMMAND_E2E_OK");

			const created = await adapter.execute(
				{
					commandId: "command_e2e_thread_create",
					profileRef: "profile_e2e",
					kind: "thread.create",
					canonicalCwd: process.cwd(),
					settings: {
						approvalPolicy: "never",
						sandboxPolicy: "read-only",
					},
				},
				AbortSignal.timeout(15_000),
			);
			assert.equal(created.kind, "thread-created");
			if (created.kind !== "thread-created") throw new Error("expected thread result");
			const threadId = sources.resolve("profile_e2e", "thread", created.sourceThreadRef);
			createdThreadIds.add(threadId);

			await adapter.execute(
				{
					commandId: "command_e2e_thread_rename",
					profileRef: "profile_e2e",
					kind: "thread.rename",
					canonicalCwd: process.cwd(),
					providerThreadId: threadId,
					name: "DEEIX app-server E2E",
				},
				AbortSignal.timeout(15_000),
			);
			await adapter.execute(
				{
					commandId: "command_e2e_thread_metadata",
					profileRef: "profile_e2e",
					kind: "thread.metadata.update",
					canonicalCwd: process.cwd(),
					providerThreadId: threadId,
					gitInfo: { branch: "deeix-app-server-e2e" },
				},
				AbortSignal.timeout(15_000),
			);

			const startedTurn = await adapter.execute(
				{
					commandId: "command_e2e_turn_start",
					profileRef: "profile_e2e",
					kind: "turn.start",
					canonicalCwd: process.cwd(),
					providerThreadId: threadId,
					input: [{ kind: "text", text: "Reply with exactly DEEIX_APP_SERVER_E2E_OK and nothing else." }],
					settings: {
						approvalPolicy: "never",
						sandboxPolicy: "read-only",
					},
				},
				AbortSignal.timeout(30_000),
			);
			assert.equal(startedTurn.kind, "turn-started");
			if (startedTurn.kind !== "turn-started") throw new Error("expected turn result");
			const completedTurn = await waitForEvent(
				events,
				(event) => event.kind === "turn/completed" && event.sourceTurnRef === startedTurn.sourceTurnRef,
				120_000,
			);
			assert.equal(turnStatus(completedTurn), "completed");
			const compactionStartIndex = events.length;
			const compacted = await adapter.execute(
				{
					commandId: "command_e2e_thread_compact",
					profileRef: "profile_e2e",
					kind: "thread.compact",
					canonicalCwd: process.cwd(),
					providerThreadId: threadId,
				},
				AbortSignal.timeout(15_000),
			);
			assert.equal(compacted.kind, "accepted");
			const compactedTurn = await waitForEvent(
				events,
				(event) =>
					event.kind === "turn/completed" &&
					event.sourceThreadRef === created.sourceThreadRef &&
					events.indexOf(event) >= compactionStartIndex,
				120_000,
			);
			assert.equal(turnStatus(compactedTurn), "completed");

			const read = await appServer.rpc.request<ThreadReadResponse>(
				"thread/read",
				{ threadId, includeTurns: true },
				AbortSignal.timeout(15_000),
			);
			assert.equal(read.thread.name, "DEEIX app-server E2E");
			assert.equal(read.thread.gitInfo?.branch, "deeix-app-server-e2e");
			const receivedExpectedReply = read.thread.turns.some((turn) =>
					turn.items.some(
						(item) => item.type === "agentMessage" && item.text.includes("DEEIX_APP_SERVER_E2E_OK"),
					),
				);
			assert.ok(receivedExpectedReply, JSON.stringify(read.thread.turns));

			const sessions = await adapter.execute(
				{
					commandId: "command_e2e_sessions",
					profileRef: "profile_e2e",
					kind: "resource.refresh",
					canonicalCwd: process.cwd(),
					resource: { scope: "workspace", name: "sessions" },
				},
				AbortSignal.timeout(30_000),
			);
			assert.equal(sessions.kind, "resource");
			assert.equal(sessions.resource, "sessions");
			assert.ok(JSON.stringify(sessions.data).includes("DEEIX_APP_SERVER_E2E_OK"));

			const forked = await adapter.execute(
				{
					commandId: "command_e2e_thread_fork",
					profileRef: "profile_e2e",
					kind: "thread.lifecycle",
					action: "fork",
					canonicalCwd: process.cwd(),
					providerThreadId: threadId,
				},
				AbortSignal.timeout(30_000),
			);
			assert.equal(forked.kind, "thread-forked");
			if (forked.kind !== "thread-forked") throw new Error("expected fork result");
			const forkedThreadId = sources.resolve("profile_e2e", "thread", forked.sourceThreadRef);
			createdThreadIds.add(forkedThreadId);

			const steeredTurn = await adapter.execute(
				{
					commandId: "command_e2e_turn_steer_start",
					profileRef: "profile_e2e",
					kind: "turn.start",
					canonicalCwd: process.cwd(),
					providerThreadId: forkedThreadId,
					input: [{ kind: "text", text: "Run this command and wait for it to finish: node -e \"setTimeout(()=>console.log('DEEIX_WAIT_DONE'),15000)\"" }],
					settings: {
						approvalPolicy: "on-request",
						sandboxPolicy: "read-only",
					},
				},
				AbortSignal.timeout(30_000),
			);
			assert.equal(steeredTurn.kind, "turn-started");
			if (steeredTurn.kind !== "turn-started") throw new Error("expected steer turn result");
			const steeredTurnId = sources.resolve("profile_e2e", "turn", steeredTurn.sourceTurnRef);
			await waitForEvent(
				events,
				(event) => event.kind === "item/started" && event.sourceTurnRef === steeredTurn.sourceTurnRef,
				60_000,
			);
			await adapter.execute(
				{
					commandId: "command_e2e_turn_steer",
					profileRef: "profile_e2e",
					kind: "turn.steer",
					canonicalCwd: process.cwd(),
					providerThreadId: forkedThreadId,
					providerTurnId: steeredTurnId,
					input: [{ kind: "text", text: "Stop inspecting and reply with exactly DEEIX_STEER_E2E_OK." }],
				},
				AbortSignal.timeout(30_000),
			);
			await adapter.execute(
				{
					commandId: "command_e2e_turn_interrupt",
					profileRef: "profile_e2e",
					kind: "turn.interrupt",
					canonicalCwd: process.cwd(),
					providerThreadId: forkedThreadId,
					providerTurnId: steeredTurnId,
				},
				AbortSignal.timeout(30_000),
			);
			const interruptedTurn = await waitForEvent(
				events,
				(event) => event.kind === "turn/completed" && event.sourceTurnRef === steeredTurn.sourceTurnRef,
				30_000,
			);
			assert.equal(turnStatus(interruptedTurn), "interrupted");

			const review = await adapter.execute(
				{
					commandId: "command_e2e_review_start",
					profileRef: "profile_e2e",
					kind: "review.start",
					canonicalCwd: process.cwd(),
					providerThreadId: forkedThreadId,
					target: { kind: "working-tree" },
				},
				AbortSignal.timeout(30_000),
			);
			assert.equal(review.kind, "turn-started");
			if (review.kind !== "turn-started") throw new Error("expected review turn result");
			const reviewedTurn = await waitForEvent(
				events,
				(event) => event.kind === "turn/completed" && event.sourceTurnRef === review.sourceTurnRef,
				120_000,
			);
			assert.equal(turnStatus(reviewedTurn), "completed");
			await adapter.execute(
				{
					commandId: "command_e2e_thread_delete",
					profileRef: "profile_e2e",
					kind: "thread.lifecycle",
					action: "delete",
					canonicalCwd: process.cwd(),
					providerThreadId: forkedThreadId,
				},
				AbortSignal.timeout(30_000),
			);
			createdThreadIds.delete(forkedThreadId);

			const loaded = await appServer.rpc.request<ThreadLoadedListResponse>(
				"thread/loaded/list",
				{},
				AbortSignal.timeout(15_000),
			);
			assert.ok(loaded.data.includes(threadId));

			for (const action of ["archive", "unarchive", "resume"] as const) {
				await adapter.execute(
					{
						commandId: `command_e2e_thread_${action}`,
						profileRef: "profile_e2e",
						kind: "thread.lifecycle",
						action,
						canonicalCwd: process.cwd(),
						providerThreadId: threadId,
					},
					AbortSignal.timeout(30_000),
				);
			}
		} finally {
			for (const threadId of createdThreadIds) {
				try {
					await appServer.rpc.request(
						"thread/delete",
						{ threadId },
						AbortSignal.timeout(15_000),
					);
				} catch {
					// A failed assertion may leave the thread in a transient provider state.
				}
			}
			await adapter.close();
		}
	},
);

async function waitForEvent(
	events: ProviderEvent[],
	predicate: (event: ProviderEvent) => boolean,
	timeoutMs: number,
): Promise<ProviderEvent> {
	const deadline = Date.now() + timeoutMs;
	while (Date.now() < deadline) {
		const event = events.find(predicate);
		if (event) return event;
		await new Promise((resolve) => setTimeout(resolve, 25));
	}
	assert.fail(`Codex event did not arrive within ${timeoutMs}ms`);
}

function turnStatus(event: ProviderEvent): string | undefined {
	const turn = event.payload.turn;
	return typeof turn === "object" && turn !== null && !Array.isArray(turn)
		? (turn as Record<string, unknown>).status as string | undefined
		: undefined;
}
