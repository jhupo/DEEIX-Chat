import assert from "node:assert/strict";
import { randomUUID } from "node:crypto";
import { mkdtemp, readFile, rm } from "node:fs/promises";
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
	const marker = resolve(`.deeix-gateway-e2e-${randomUUID()}.txt`);
	const markerBase64 = Buffer.from(marker, "utf8").toString("base64");
	const prompt = process.env.CODEX_GATEWAY_E2E_PROMPT?.trim() ||
		`Use the terminal tool to run exactly: node -e "require('node:fs').writeFileSync(Buffer.from('${markerBase64}', 'base64').toString(), 'P0')". ` +
		"Do not simulate the command or report completion before the tool finishes.";
	const codex = process.env.CODEX_GATEWAY_E2E_CODEX?.trim() || "codex";
	const dataDirectory = await mkdtemp(join(tmpdir(), "deeix-gateway-e2e-"));
	let deviceId = "";
	let conversationId = "";
	let running: RunningGateway | undefined;

	const api = async <T>(path: string, init: RequestInit = {}): Promise<T> => {
		const response = await fetch(`${server}${path}`, {
			...init,
			headers: {
				authorization: `Bearer ${accessToken}`,
				...(init.body ? { "content-type": "application/json" } : {}),
				...init.headers,
			},
		});
		const rawBody = await response.text();
		let envelope: { errorMsg?: string; data?: T };
		try {
			envelope = JSON.parse(rawBody) as { errorMsg?: string; data?: T };
		} catch {
			const contentType = response.headers.get("content-type") ?? "unknown";
			const preview = [...rawBody]
				.map((character) => character.charCodeAt(0) < 32 ? " " : character)
				.join("")
				.slice(0, 256);
			throw new Error(`${init.method ?? "GET"} ${path}: ${response.status} non-JSON ${contentType}: ${preview}`);
		}
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
		await waitForGatewayOnline(api, deviceId, running, 120_000);
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
		let turnFailure: unknown;
		const turnRequest = api(`/api/v1/conversations/${conversationId}/turns`, {
			method: "POST",
			body: JSON.stringify({
				contentType: "text", content: prompt, clientRunID: runId,
				options: { approvalPolicy: "untrusted", sandboxPolicy: "workspace-write" },
			}),
		}).catch((error: unknown) => {
			turnFailure = error;
		});

		let responded = false;
		let completedAssistant: Message | undefined;
		await eventually(async () => {
			if (turnFailure) throw turnFailure;
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
			if (assistant?.status === "success") completedAssistant = assistant;
			return assistant?.status === "success" && assistant.content.trim().length > 0;
		}, 4 * 60_000);
		await turnRequest;
		if (turnFailure) throw turnFailure;
		const markerContent = await readFile(marker, "utf8").catch(() => "missing");
		assert.equal(responded, true,
			`the E2E prompt did not produce a server interaction; marker=${markerContent}; assistant=${completedAssistant?.content ?? "missing"}`);
		assert.equal(await readFile(marker, "utf8"), "P0");

		await stopGateway(running);
		running = undefined;
		await eventually(
			async () => !(await api<{ online: boolean }>(`/api/v1/agent/devices/${deviceId}`)).online,
			30_000,
		);
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
		await waitForGatewayOnline(api, deviceId, running, 120_000);
		const refreshStartedAt = Date.now();
		await api(`/api/v1/agent/devices/${deviceId}/workspaces/${workspaceId}/resources/sessions/refresh`, {
			method: "POST",
			headers: { "Idempotency-Key": randomUUID() },
		});
		try {
			await eventually(async () => {
				const snapshot = await api<{ data: unknown; refreshedAt: string }>(
					`/api/v1/agent/devices/${deviceId}/workspaces/${workspaceId}/resources/sessions`,
				);
				return Date.parse(snapshot.refreshedAt) >= refreshStartedAt - 1_000 && JSON.stringify(snapshot.data).includes(prompt);
			}, 90_000);
		} catch (error) {
			throw new Error(`sessions refresh failed; commandWal=${JSON.stringify(await resourceCommandDiagnostics(dataDirectory))}`, {
				cause: error,
			});
		}
	} finally {
		if (running) await stopGateway(running);
		if (conversationId) await api(`/api/v1/conversations/${conversationId}`, { method: "DELETE" }).catch(() => undefined);
		if (deviceId) await api(`/api/v1/agent/devices/${deviceId}`, { method: "DELETE" }).catch(() => undefined);
		await rm(marker, { force: true });
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

type RunningGateway = {
	controller: AbortController;
	done: Promise<void>;
	connectionErrors: string[];
};

function startGateway(dataDirectory: string): RunningGateway {
	const controller = new AbortController();
	const connectionErrors: string[] = [];
	const done = runGateway({
		dataDirectory,
		reconnect: true,
		onConnectionError: (error) => connectionErrors.push(error.message),
	}, controller.signal).catch((error) => {
		if (!controller.signal.aborted) throw error;
	});
	return { controller, done, connectionErrors };
}

async function stopGateway(running: RunningGateway): Promise<void> {
	running.controller.abort(new Error("E2E reconnect checkpoint"));
	await running.done;
}

async function waitForGatewayOnline(
	api: <T>(path: string, init?: RequestInit) => Promise<T>,
	deviceId: string,
	running: RunningGateway,
	timeout: number,
): Promise<void> {
	try {
		await eventually(
			async () => (await api<{ online: boolean }>(`/api/v1/agent/devices/${deviceId}`)).online,
			timeout,
		);
	} catch (error) {
		throw new Error(`gateway did not come online; connectionErrors=${JSON.stringify(running.connectionErrors.slice(-8))}`, {
			cause: error,
		});
	}
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

async function resourceCommandDiagnostics(dataDirectory: string): Promise<unknown[]> {
	const store = await DurableWalStore.open(join(dataDirectory, "wal", "commands"));
	try {
		return store.records()
			.filter((record) => {
				const command = Reflect.get(record.payload as object, "command");
				return record.kind === "command.terminal" ||
					(typeof command === "object" && command !== null && Reflect.get(command, "kind") === "resource.refresh");
			})
			.slice(-6);
	} finally {
		store.close();
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
