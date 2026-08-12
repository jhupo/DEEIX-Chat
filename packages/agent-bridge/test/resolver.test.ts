import assert from "node:assert/strict";
import { mkdir, mkdtemp, rm, symlink } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import {
	resolveProviderCommand,
	SourceRefRegistry,
} from "../src/commands/resolve-provider-command.js";
import { WorkspaceRegistry } from "../src/config/workspace-registry.js";
import { parseAgentCommand } from "../src/protocol/agent-command.js";

test("resolver keeps canonical paths and raw provider IDs local", async (context) => {
	const root = await mkdtemp(join(tmpdir(), "deeix-workspace-"));
	context.after(() => rm(root, { recursive: true, force: true }));
	const workspaces = new WorkspaceRegistry();
	const canonical = await workspaces.register("workspace_1", root);
	const sources = new SourceRefRegistry();
	await sources.register("profile_1", "thread", "source_thread_1", "raw-thread-id");

	const command = parseAgentCommand({
		kind: "turn.start",
		deviceId: "device_1",
		profileId: "profile_1",
		workspaceId: "workspace_1",
		threadId: "thread_1",
		sourceThreadRef: "source_thread_1",
		input: [{ kind: "text", text: "run checks" }],
		settings: {},
	});
	const resolved = await resolveProviderCommand(
		"command_1",
		command,
		workspaces,
		sources,
	);

	assert.equal(resolved.kind, "turn.start");
	if (resolved.kind !== "turn.start")
		throw new Error("expected a resolved turn.start command");
	assert.equal(resolved.canonicalCwd, canonical);
	assert.equal(resolved.providerThreadId, "raw-thread-id");
	assert.equal("deviceId" in resolved, false);
	assert.equal("workspaceId" in resolved, false);
});

test("workspace resolver rejects traversal and a symlink that exits the root", async (context) => {
	const parent = await mkdtemp(join(tmpdir(), "deeix-boundary-"));
	context.after(() => rm(parent, { recursive: true, force: true }));
	const root = join(parent, "root");
	const outside = join(parent, "outside");
	await Promise.all([mkdir(root), mkdir(outside)]);
	const workspaces = new WorkspaceRegistry();
	await workspaces.register("workspace_1", root);

	await assert.rejects(
		() => workspaces.resolvePath("workspace_1", "../outside"),
		/escapes/,
	);
	await symlink(
		outside,
		join(root, "link"),
		process.platform === "win32" ? "junction" : "dir",
	);
	await assert.rejects(
		() => workspaces.resolvePath("workspace_1", "link/file.txt"),
		/escapes/,
	);
});

test("source references are immutable", async () => {
	const sources = new SourceRefRegistry();
	await sources.register("profile_1", "thread", "thread_ref", "raw_1");
	await sources.register("profile_1", "thread", "thread_ref", "raw_1");
	await assert.rejects(
		() => sources.register("profile_1", "thread", "thread_ref", "raw_2"),
		/cannot be rebound/,
	);
});

test("source references restore from the durable WAL", async (context) => {
	const directory = await mkdtemp(join(tmpdir(), "deeix-sources-"));
	context.after(() => rm(directory, { recursive: true, force: true }));
	const { DurableWalStore } = await import("../src/wal/durable-wal-store.js");
	const wal = await DurableWalStore.open(directory);
	const sources = SourceRefRegistry.restore(wal);
	const sourceRef = await sources.publish(
		"profile_1",
		"thread",
		"provider-thread-1",
	);
	wal.close();

	const restoredWal = await DurableWalStore.open(directory);
	const restored = SourceRefRegistry.restore(restoredWal);
	assert.equal(
		restored.resolve("profile_1", "thread", sourceRef),
		"provider-thread-1",
	);
	assert.equal(
		await restored.publish("profile_1", "thread", "provider-thread-1"),
		sourceRef,
	);
	restoredWal.close();
});
