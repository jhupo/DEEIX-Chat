import assert from "node:assert/strict";
import { randomUUID } from "node:crypto";
import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import test from "node:test";
import { main } from "../src/cli.js";
import { readBridgeConfig } from "../src/config/bridge-config.js";
import { runGateway } from "../src/runtime/gateway-runtime.js";
import { CommandJournal } from "../src/wal/command-journal.js";
import { DurableWalStore } from "../src/wal/durable-wal-store.js";
import { OutgoingFrameJournal } from "../src/wal/outgoing-frame-journal.js";

const enabled = process.env.CODEX_GATEWAY_E2E === "1";

test("production WSS gateway round trip", { skip: !enabled, timeout: 10 * 60_000 }, async () => {
	const server = requiredEnvironment("CODEX_GATEWAY_E2E_URL").replace(/\/+$/, "");
	const userPublicId = requiredEnvironment("CODEX_GATEWAY_E2E_USER_PUBLIC_ID");
	const accessToken = requiredEnvironment("CODEX_GATEWAY_E2E_ACCESS_TOKEN").replace(/^Bearer\s+/i, "");
	const workspace = resolve(requiredEnvironment("CODEX_GATEWAY_E2E_WORKSPACE"));
	const marker = `.deeix-gateway-e2e-${randomUUID()}.txt`;
	const prompt = process.env.CODEX_GATEWAY_E2E_PROMPT?.trim() ||
		`Use the terminal to create ${marker} in the current workspace containing the text P0, then report completion.`;
	const codex = process.env.CODEX_GATEWAY_E2E_CODEX?.trim() || "codex";
	const dataDirectory = await mkdtemp(join(tmpdir(), "deeix-gateway-e2e-"));
	let deviceId = "";
	let conversationId = "";
	let running: { controller: AbortController; done: Promise<void> } | undefined;

	const api = async <T>(path: string, init: RequestInit = {}): Promise<T> => {
		const response = await fetch(`${server}${path}`, {
			...init,
			headers: {
				authorization: `Bearer ${accessToken}`,
				...(init.body ? { "content-type": "application/json" } : {}),
				...init.headers,
			},
		});
		const envelope = await response.json() as { errorMsg?: string; data?: T };
		if (!response.ok) throw new Error(`${init.method ?? "GET"} ${path}: ${response.status} ${envelope.errorMsg ?? "request failed"}`);
		return envelope.data as T;
	};

	try {
		await main([
			"install", "--server", server, "--user", userPublicId, "--workspace", workspace,
			"--name", `gateway-e2e-${Date.now()}`, "--codex", codex, "--data-dir", dataDirectory,
		]);
		const config = await readBridgeConfig(join(dataDirectory, "config.json"));
		deviceId = config.deviceId;
		const workspaceId = config.workspaces[0]?.workspaceId;
		assert.ok(workspaceId);

		running = startGateway(dataDirectory);
		await eventually(async () => (await api<{ online: boolean }>(`/api/v1/agent/devices/${deviceId}`)).online);
		const profiles = await api<Array<{ profileId: string; status: string; manifest: ProviderManifest }>>(
			`/api/v1/agent/devices/${deviceId}/profiles`,
		);
		const profile = profiles.find((item) => item.profileId === config.profileId);
		assert.equal(profile?.status, "ready");
		assert.equal(profile?.manifest.provider, "codex");
		assert.ok(profile?.manifest.interactionKinds.includes("command_approval"));

		const conversation = await api<{ publicID: string }>("/api/v1/conversations", {
			method: "POST",
			body: JSON.stringify({
				title: "Gateway P0 E2E",
				execution: {
					type: "gateway", deviceID: deviceId, profileID: config.profileId, workspaceID: workspaceId,
				},
			}),
		});
		conversationId = conversation.publicID;
		const runId = `run_${randomUUID().replaceAll("-", "")}`;
		await api(`/api/v1/conversations/${conversationId}/turns`, {
			method: "POST",
			body: JSON.stringify({
				contentType: "text", content: prompt, clientRunID: runId,
				options: { approvalPolicy: "on-request", sandboxPolicy: "read-only" },
			}),
		});

		let responded = false;
		await eventually(async () => {
			const interactions = await api<Interaction[]>(`/api/v1/conversations/${conversationId}/interactions?status=pending`);
			const interaction = interactions[0];
			if (interaction && !responded) {
				await api(`/api/v1/conversation-interactions/${interaction.interactionID}/respond`, {
					method: "POST",
					headers: { "Idempotency-Key": randomUUID() },
					body: JSON.stringify({ response: responseFor(interaction) }),
				});
				responded = true;
			}
			const page = await api<{ results: Message[] }>(`/api/v1/conversations/${conversationId}/messages?page=1&page_size=100&tail=true`);
			const assistant = page.results.find((message) => message.runID === runId && message.role === "assistant");
			if (assistant && ["error", "interrupted"].includes(assistant.status))
				throw new Error(`gateway turn ended as ${assistant.status}: ${assistant.errorMessage ?? "unknown error"}`);
			return assistant?.status === "success" && assistant.content.trim().length > 0;
		}, 4 * 60_000);
		assert.equal(responded, true, "the E2E prompt did not produce a server interaction");

		await stopGateway(running);
		running = undefined;
		const commandStore = await DurableWalStore.open(join(dataDirectory, "wal", "commands"));
		const outgoingStore = await DurableWalStore.open(join(dataDirectory, "wal", "outgoing"));
		try {
			assert.ok(CommandJournal.restore(commandStore).contiguousReceipt() > 0, "command WAL has no acknowledged server command");
			assert.ok(OutgoingFrameJournal.restore(outgoingStore).acknowledgedSequence() > 0, "outgoing WAL has no WSS acknowledgement");
		} finally {
			commandStore.close();
			outgoingStore.close();
		}

		running = startGateway(dataDirectory);
		await eventually(async () => (await api<{ online: boolean }>(`/api/v1/agent/devices/${deviceId}`)).online);
		const refreshStartedAt = Date.now();
		await api(`/api/v1/agent/devices/${deviceId}/workspaces/${workspaceId}/resources/sessions/refresh`, {
			method: "POST",
			headers: { "Idempotency-Key": randomUUID() },
		});
		await eventually(async () => {
			const snapshot = await api<{ data: unknown; refreshedAt: string }>(
				`/api/v1/agent/devices/${deviceId}/workspaces/${workspaceId}/resources/sessions`,
			);
			return Date.parse(snapshot.refreshedAt) >= refreshStartedAt - 1_000 && JSON.stringify(snapshot.data).includes(prompt);
		}, 90_000);
	} finally {
		if (running) await stopGateway(running);
		if (conversationId) await api(`/api/v1/conversations/${conversationId}`, { method: "DELETE" }).catch(() => undefined);
		if (deviceId) await api(`/api/v1/agent/devices/${deviceId}`, { method: "DELETE" }).catch(() => undefined);
		await rm(join(workspace, marker), { force: true });
		await rm(dataDirectory, { recursive: true, force: true });
	}
});

type ProviderManifest = {
	provider: string;
	interactionKinds: string[];
};

type Interaction = {
	interactionID: string;
	kind: "command_approval" | "file_approval" | "user_input" | "permission" | "mcp_elicitation" | "dynamic_tool";
	request: Record<string, unknown>;
};

type Message = {
	role: string;
	runID: string;
	status: string;
	content: string;
	errorMessage?: string;
};

function startGateway(dataDirectory: string): { controller: AbortController; done: Promise<void> } {
	const controller = new AbortController();
	const done = runGateway({ dataDirectory, reconnect: true }, controller.signal).catch((error) => {
		if (!controller.signal.aborted) throw error;
	});
	return { controller, done };
}

async function stopGateway(running: { controller: AbortController; done: Promise<void> }): Promise<void> {
	running.controller.abort(new Error("E2E reconnect checkpoint"));
	await running.done;
}

function responseFor(interaction: Interaction): Record<string, unknown> {
	switch (interaction.kind) {
		case "command_approval":
		case "file_approval":
			return { kind: "approval", decision: "accept" };
		case "permission":
			return { kind: "permission", decision: "accept", scope: "turn" };
		case "mcp_elicitation":
			return { kind: "mcp-elicitation", decision: "decline" };
		case "dynamic_tool":
			return { kind: "dynamic-tool", success: true, content: [{ kind: "text", text: "E2E response" }] };
		case "user_input": {
			const questions = Array.isArray(interaction.request.questions) ? interaction.request.questions : [];
			const answers = Object.fromEntries(questions.flatMap((question) => {
				if (typeof question !== "object" || question === null) return [];
				const questionRef = Reflect.get(question, "questionRef");
				return typeof questionRef === "string" ? [[questionRef, "yes"]] : [];
			}));
			assert.ok(Object.keys(answers).length > 0, "user input interaction has no question refs");
			return { kind: "user-input", answers };
		}
	}
}

async function eventually(check: () => Promise<boolean>, timeout = 60_000): Promise<void> {
	const deadline = Date.now() + timeout;
	let lastError: unknown;
	while (Date.now() < deadline) {
		try {
			if (await check()) return;
		} catch (error) {
			lastError = error;
		}
		await new Promise((resolve) => setTimeout(resolve, 1_000));
	}
	if (lastError) throw lastError;
	throw new Error(`condition was not met within ${timeout}ms`);
}

function requiredEnvironment(name: string): string {
	const value = process.env[name]?.trim();
	if (!value) throw new Error(`${name} is required when CODEX_GATEWAY_E2E=1`);
	return value;
}
