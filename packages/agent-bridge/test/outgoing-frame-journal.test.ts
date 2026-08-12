import assert from "node:assert/strict";
import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { DurableWalStore } from "../src/wal/durable-wal-store.js";
import { OutgoingFrameJournal } from "../src/wal/outgoing-frame-journal.js";

test("outgoing terminal frames restore and acknowledge contiguously", async () => {
	const directory = await mkdtemp(join(tmpdir(), "deeix-outgoing-"));
	try {
		let wal = await DurableWalStore.open(directory);
		let journal = OutgoingFrameJournal.restore(wal);
		const first = await journal.appendTerminal(4, "command_4", {
			kind: "result",
			result: { kind: "accepted" },
		});
		assert.equal(first.bridgeSeq, 1);
		assert.equal((await journal.appendTerminal(4, "command_4", first.outcome)).bridgeSeq, 1);
		const second = await journal.appendEvent({
			kind: "item/agentMessage/delta",
			sourceThreadRef: "thread_1",
			occurredAt: "2026-08-13T00:00:00.000Z",
			payload: { delta: "hello" },
		});
		assert.equal(second.bridgeSeq, 2);
		wal.close();

		wal = await DurableWalStore.open(directory);
		journal = OutgoingFrameJournal.restore(wal);
		assert.deepEqual(journal.pending(), [first, second]);
		await journal.acknowledge(1);
		assert.deepEqual(journal.pending(), [second]);
		assert.equal(journal.acknowledgedSequence(), 1);
		await journal.acknowledge(2);
		assert.deepEqual(journal.pending(), []);
		wal.close();
	} finally {
		await rm(directory, { recursive: true, force: true });
	}
});
