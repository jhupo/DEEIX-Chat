import assert from "node:assert/strict";
import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { GatewayCommandExecutor } from "../src/commands/gateway-command-executor.js";
import type { ProviderCommand } from "../src/commands/resolve-provider-command.js";
import { SourceRefRegistry } from "../src/commands/resolve-provider-command.js";
import { WorkspaceRegistry } from "../src/config/workspace-registry.js";
import type {
	ProviderAdapter,
	ProviderEvent,
	ProviderManifest,
	ProviderResult,
} from "../src/providers/provider-adapter.js";
import { ProviderRegistry } from "../src/providers/provider-registry.js";
import { ArtifactDownloader } from "../src/transport/artifact-downloader.js";
import { CommandJournal } from "../src/wal/command-journal.js";
import { DurableWalStore } from "../src/wal/durable-wal-store.js";

const createCommand = {
	kind: "thread.create",
	deviceId: "device_1",
	profileId: "profile_1",
	workspaceId: "workspace_1",
	settings: { model: "gpt-5.6-sol" },
};

test("executor resolves, routes, persists, and replays without a second provider call", async (context) => {
	const directory = await mkdtemp(join(tmpdir(), "deeix-executor-"));
	context.after(() => rm(directory, { recursive: true, force: true }));
	const wal = await DurableWalStore.open(join(directory, "wal"));
	const journal = CommandJournal.restore(wal);
	const workspaces = new WorkspaceRegistry();
	await workspaces.register("workspace_1", directory);
	const providers = new ProviderRegistry();
	const adapter = new FakeAdapter();
	providers.register("profile_1", adapter);
	const executor = new GatewayCommandExecutor(
		journal,
		workspaces,
		new SourceRefRegistry(),
		providers,
	);

	const first = await executor.dispatch(
		1,
		"command_1",
		createCommand,
		AbortSignal.timeout(1000),
	);
	const replay = await executor.dispatch(
		1,
		"command_1",
		createCommand,
		AbortSignal.timeout(1000),
	);
	assert.deepEqual(first, {
		kind: "result",
		result: { kind: "thread-created", sourceThreadRef: "source_thread_1" },
	});
	assert.deepEqual(replay, first);
	assert.equal(adapter.calls, 1);
	wal.close();
	await providers.close();
});

test("executor terminalizes an indeterminate restored invocation without replay", async (context) => {
	const directory = await mkdtemp(join(tmpdir(), "deeix-recovery-"));
	context.after(() => rm(directory, { recursive: true, force: true }));
	const wal = await DurableWalStore.open(join(directory, "wal"));
	const journal = CommandJournal.restore(wal);
	await journal.receive(1, "command_1", createCommand);
	await journal.start("command_1", { commandKind: "thread.create" });
	wal.close();

	const restoredWal = await DurableWalStore.open(join(directory, "wal"));
	const restored = CommandJournal.restore(restoredWal);
	const workspaces = new WorkspaceRegistry();
	await workspaces.register("workspace_1", directory);
	const providers = new ProviderRegistry();
	const adapter = new FakeAdapter();
	providers.register("profile_1", adapter);
	const executor = new GatewayCommandExecutor(
		restored,
		workspaces,
		new SourceRefRegistry(),
		providers,
	);

	const outcome = await executor.dispatch(
		1,
		"command_1",
		createCommand,
		AbortSignal.timeout(1000),
	);
	assert.equal(outcome.kind, "error");
	if (outcome.kind !== "error") throw new Error("expected an error outcome");
	assert.equal(outcome.error.code, "outcome_unknown");
	assert.equal(adapter.calls, 0);
	restoredWal.close();
	await providers.close();
});

test("executor replays a read-only resource refresh after a crash", async (context) => {
	const directory = await mkdtemp(join(tmpdir(), "deeix-recovery-read-"));
	context.after(() => rm(directory, { recursive: true, force: true }));
	const command = {
		kind: "resource.refresh",
		deviceId: "device_1",
		profileId: "profile_1",
		resource: { scope: "profile", name: "models" },
	} as const;
	const wal = await DurableWalStore.open(join(directory, "wal"));
	const journal = CommandJournal.restore(wal);
	await journal.receive(1, "command_1", command);
	await journal.start("command_1", {
		commandKind: "resource.refresh",
		replay: "safe",
	});
	wal.close();

	const restoredWal = await DurableWalStore.open(join(directory, "wal"));
	const restored = CommandJournal.restore(restoredWal);
	const providers = new ProviderRegistry();
	const adapter = new FakeAdapter();
	providers.register("profile_1", adapter);
	const executor = new GatewayCommandExecutor(
		restored,
		new WorkspaceRegistry(),
		new SourceRefRegistry(),
		providers,
	);
	const outcome = await executor.dispatch(
		1,
		"command_1",
		command,
		AbortSignal.timeout(1000),
	);
	assert.deepEqual(outcome, {
		kind: "result",
		result: { kind: "resource", resource: "models", data: [] },
	});
	assert.equal(adapter.calls, 1);
	restoredWal.close();
	await providers.close();
});

test("artifact failure persists the command without advancing its receipt", async (context) => {
	const directory = await mkdtemp(join(tmpdir(), "deeix-artifact-receipt-"));
	context.after(() => rm(directory, { recursive: true, force: true }));
	const wal = await DurableWalStore.open(join(directory, "wal"));
	const journal = CommandJournal.restore(wal);
	const workspaces = new WorkspaceRegistry();
	await workspaces.register("workspace_1", directory);
	const providers = new ProviderRegistry();
	const adapter = new BlockingAdapter();
	providers.register("profile_1", adapter);
	const executor = new GatewayCommandExecutor(
		journal,
		workspaces,
		new SourceRefRegistry(),
		providers,
		new ArtifactDownloader("https://deeix.test", workspaces, async () =>
			new Response(null, { status: 503 })),
	);
	const command = {
		kind: "turn.start",
		deviceId: "device_1",
		profileId: "profile_1",
		workspaceId: "workspace_1",
		threadId: "thread_1",
		sourceThreadRef: "thread_ref",
		input: [{ kind: "artifact", artifactRef: "agart_0123456789abcdef0123456789abcdef" }],
		settings: {},
	};

	await assert.rejects(
		() => executor.dispatch(1, "command_1", command, AbortSignal.timeout(1000), undefined, [{
			artifactRef: "agart_0123456789abcdef0123456789abcdef",
			fileName: "fixture.png",
			mimeType: "image/png",
			sizeBytes: 1,
			sha256: "0".repeat(64),
			expiresAt: new Date(Date.now() + 60_000).toISOString(),
			grant: "A".repeat(43),
		}]),
		/artifact download failed/,
	);
	assert.equal(journal.get("command_1")?.state, "received");
	assert.equal(journal.contiguousReceipt(), 0);
	wal.close();
	await providers.close();
});

test("interaction response can complete while a turn command is waiting", async (context) => {
	const directory = await mkdtemp(join(tmpdir(), "deeix-interaction-"));
	context.after(() => rm(directory, { recursive: true, force: true }));
	const wal = await DurableWalStore.open(join(directory, "wal"));
	const journal = CommandJournal.restore(wal);
	const workspaces = new WorkspaceRegistry();
	await workspaces.register("workspace_1", directory);
	const sources = new SourceRefRegistry();
	await sources.register("profile_1", "thread", "thread_ref", "provider_thread");
	await sources.register("profile_1", "request", "request_ref", "provider_request");
	const providers = new ProviderRegistry();
	const adapter = new BlockingAdapter();
	providers.register("profile_1", adapter);
	const executor = new GatewayCommandExecutor(journal, workspaces, sources, providers);
	const controller = new AbortController();

	const turn = executor.dispatch(1, "command_turn", {
		kind: "turn.start",
		deviceId: "device_1",
		profileId: "profile_1",
		workspaceId: "workspace_1",
		threadId: "thread_1",
		sourceThreadRef: "thread_ref",
		input: [{ kind: "text", text: "run" }],
		settings: {},
	}, controller.signal);
	await adapter.turnStarted;
	const interaction = await executor.dispatch(2, "command_interaction", {
		kind: "interaction.respond",
		deviceId: "device_1",
		profileId: "profile_1",
		workspaceId: "workspace_1",
		threadId: "thread_1",
		sourceThreadRef: "thread_ref",
		scope: "thread",
		interactionId: "interaction_1",
		sourceRequestRef: "request_ref",
		response: { kind: "approval", decision: "accept" },
	}, controller.signal);
	assert.equal(interaction.kind, "result");
	assert.equal((await turn).kind, "result");
	wal.close();
	await providers.close();
});

const TEST_MANIFEST_CAPABILITIES: Pick<
	ProviderManifest,
	"resources" | "inputKinds" | "threadSettings" | "interactionKinds"
> = {
	resources: { profile: ["models"], workspace: ["sessions"] },
	inputKinds: ["text"],
	threadSettings: {
		model: true,
		reasoningEffort: ["low"],
		approvalPolicy: ["on-request"],
		sandboxPolicy: ["workspace-write"],
	},
	interactionKinds: ["command_approval"],
};

class FakeAdapter implements ProviderAdapter {
	readonly kind = "codex";
	calls = 0;
	readonly #manifest: ProviderManifest = {
		provider: "codex",
		runtimeVersion: "0.147.0",
		protocolVersion: "v2",
		schemaHash: "fixture",
		commands: ["thread.create", "resource.refresh"],
		...TEST_MANIFEST_CAPABILITIES,
	};

	async start(
		_onEvent: (event: ProviderEvent) => Promise<void>,
		_signal: AbortSignal,
	): Promise<ProviderManifest> {
		return this.#manifest;
	}

	async proveRuntimeAuth(): Promise<string> {
		return "fixture";
	}

	async execute(
		command: ProviderCommand,
		_signal: AbortSignal,
	): Promise<ProviderResult> {
		this.calls += 1;
		if (command.kind === "resource.refresh")
			return { kind: "resource", resource: command.resource.name, data: [] };
		assert.equal(command.kind, "thread.create");
		return { kind: "thread-created", sourceThreadRef: "source_thread_1" };
	}

	capabilities(): ProviderManifest {
		return this.#manifest;
	}

	async close(): Promise<void> {}
}

class BlockingAdapter implements ProviderAdapter {
	readonly kind = "codex";
	readonly #manifest: ProviderManifest = {
		provider: "codex",
		runtimeVersion: "0.147.0",
		protocolVersion: "v2",
		schemaHash: "fixture",
		commands: ["turn.start", "interaction.respond"],
		...TEST_MANIFEST_CAPABILITIES,
	};
	#releaseTurn!: () => void;
	readonly turnStarted = new Promise<void>((resolve) => {
		this.#releaseTurn = resolve;
	});
	#turnDone!: () => void;
	readonly #waitForResponse = new Promise<void>((resolve) => {
		this.#turnDone = resolve;
	});

	async start(): Promise<ProviderManifest> {
		return this.#manifest;
	}

	async proveRuntimeAuth(): Promise<string> {
		return "fixture";
	}

	async execute(command: ProviderCommand): Promise<ProviderResult> {
		if (command.kind === "turn.start") {
			this.#releaseTurn();
			await this.#waitForResponse;
			return { kind: "turn-started", sourceTurnRef: "turn_ref" };
		}
		assert.equal(command.kind, "interaction.respond");
		this.#turnDone();
		return { kind: "accepted" };
	}

	capabilities(): ProviderManifest {
		return this.#manifest;
	}

	async close(): Promise<void> {}
}
