import assert from "node:assert/strict";
import { appendFile, mkdtemp, readdir, readFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { CommandJournal } from "../src/wal/command-journal.js";
import { DurableWalStore } from "../src/wal/durable-wal-store.js";

const command = {
	kind: "thread.create",
	deviceId: "device_1",
	profileId: "profile_1",
	workspaceId: "workspace_1",
	settings: { model: "gpt-5.6-sol" },
};

test("WAL serializes concurrent appends and restores their sequence", async (context) => {
	const directory = await mkdtemp(join(tmpdir(), "deeix-wal-"));
	context.after(() => rm(directory, { recursive: true, force: true }));
	const wal = await DurableWalStore.open(directory, {
		maxRecordBytes: 1024,
		maxSegmentBytes: 2048,
	});
	await Promise.all(
		Array.from({ length: 24 }, (_, index) =>
			wal.append("test.record", { index }),
		),
	);
	wal.close();

	const restored = await DurableWalStore.open(directory, {
		maxRecordBytes: 1024,
		maxSegmentBytes: 2048,
	});
	assert.deepEqual(
		restored.records().map((record) => record.sequence),
		Array.from({ length: 24 }, (_, index) => index + 1),
	);
	restored.close();
});

test("WAL quarantines and truncates an incomplete crash tail", async (context) => {
	const directory = await mkdtemp(join(tmpdir(), "deeix-tail-"));
	context.after(() => rm(directory, { recursive: true, force: true }));
	const wal = await DurableWalStore.open(directory);
	await wal.append("test.record", { value: 1 });
	wal.close();
	const segment = (await readdir(directory)).find((name) =>
		name.endsWith(".jsonl"),
	);
	assert.ok(segment);
	const path = join(directory, segment);
	await appendFile(path, '{"version":1,"sequence":2');

	const restored = await DurableWalStore.open(directory);
	assert.equal(restored.records().length, 1);
	const data = await readFile(path, "utf8");
	assert.ok(data.endsWith("\n"));
	assert.ok(
		(await readdir(directory)).some((name) => name.includes(".corrupt-")),
	);
	restored.close();
});

test("command journal restores invocation state and replays a cached terminal", async (context) => {
	const directory = await mkdtemp(join(tmpdir(), "deeix-journal-"));
	context.after(() => rm(directory, { recursive: true, force: true }));
	const wal = await DurableWalStore.open(directory);
	const journal = CommandJournal.restore(wal);
	await journal.receive(1, "command_1", command);
	await journal.start("command_1", { commandKind: "thread.create" });
	assert.equal(journal.pendingRecovery().length, 1);
	wal.close();

	const restoredWal = await DurableWalStore.open(directory);
	const restored = CommandJournal.restore(restoredWal);
	assert.equal(restored.pendingRecovery()[0]?.commandId, "command_1");
	const terminal = await restored.complete("command_1", {
		kind: "result",
		result: { sourceThreadRef: "source_1" },
	});
	assert.equal(terminal.state, "terminal_cached");
	assert.deepEqual(await restored.receive(1, "command_1", command), terminal);
	await assert.rejects(
		() => restored.receive(2, "command_1", command),
		/different content/,
	);
	restoredWal.close();
});
