import assert from "node:assert/strict";
import test from "node:test";

// @ts-expect-error Node's TypeScript runner requires an explicit extension.
import { applyAgentExecutionEvents, getAgentExecutionRecoverySnapshot, getAgentRunSnapshot, hasComposerAgentActivity, setAgentRunContext } from "./agent-run-store.ts";

test("applies a historical event page as one ordered batch", () => {
  setAgentRunContext("test-context", "conversation-1");

  const accepted = applyAgentExecutionEvents([
    {
      runID: "run-1",
      seq: 3,
      kind: "turn/completed",
      payload: { status: "completed" },
      occurredAt: "2026-08-24T00:00:03Z",
    },
    {
      runID: "run-1",
      seq: 1,
      kind: "turn/started",
      payload: {},
      occurredAt: "2026-08-24T00:00:01Z",
    },
    {
      runID: "run-1",
      seq: 2,
      kind: "turn/plan/updated",
      payload: { plan: [{ step: "Inspect", status: "completed" }] },
      occurredAt: "2026-08-24T00:00:02Z",
    },
  ], "conversation-1");

  assert.equal(accepted, 3);
  assert.deepEqual(getAgentExecutionRecoverySnapshot(), {
    contiguousSeq: 3,
    highestSeq: 3,
    hasGap: false,
  });
  assert.equal(getAgentRunSnapshot("run-1").status, "completed");
  assert.deepEqual(getAgentRunSnapshot("run-1").plan.map((step) => step.text), ["Inspect"]);
  assert.equal(applyAgentExecutionEvents([{
    runID: "run-1",
    seq: 3,
    kind: "turn/completed",
    payload: { status: "completed" },
    occurredAt: "2026-08-24T00:00:03Z",
  }], "conversation-1"), 0);
});

test("shows composer activity only for active plans or unresolved interactions", () => {
  assert.equal(hasComposerAgentActivity({
    status: "running",
    plan: [{ key: "step-1", text: "Inspect", status: "inProgress" }],
    interactions: [],
  }), true);
  assert.equal(hasComposerAgentActivity({
    status: "completed",
    plan: [{ key: "step-1", text: "Inspect", status: "completed" }],
    interactions: [],
  }), false);
  assert.equal(hasComposerAgentActivity({
    status: "completed",
    plan: [],
    interactions: [{ status: "pending" }],
  }), true);
});

test("preserves a file patch when the completed item omits it", () => {
  setAgentRunContext("file-test-context", "conversation-files");

  applyAgentExecutionEvents([
    {
      runID: "run-files",
      seq: 1,
      kind: "item/started",
      payload: {
        item: {
          itemID: "file-item",
          type: "fileChange",
          changes: [{ path: "src/a.ts", change: "update" }, { path: "src/b.ts", change: "update" }],
        },
      },
      occurredAt: "2026-08-25T00:00:01Z",
    },
    {
      runID: "run-files",
      seq: 2,
      kind: "item/fileChange/patchUpdated",
      payload: {
        itemID: "file-item",
        patch: "diff --git a/src/a.ts b/src/a.ts\n-old\n+new",
        truncated: true,
      },
      occurredAt: "2026-08-25T00:00:02Z",
    },
    {
      runID: "run-files",
      seq: 3,
      kind: "item/completed",
      payload: {
        item: {
          itemID: "file-item",
          type: "fileChange",
          status: "completed",
          changes: [{ path: "src/a.ts", change: "update" }, { path: "src/b.ts", change: "update" }],
        },
      },
      occurredAt: "2026-08-25T00:00:03Z",
    },
  ], "conversation-files");

  const item = getAgentRunSnapshot("run-files").items[0];
  assert.equal(item?.kind, "file");
  if (item?.kind !== "file") return;
  assert.equal(item.diff, "diff --git a/src/a.ts b/src/a.ts\n-old\n+new");
  assert.equal(item.truncated, true);
  assert.deepEqual(item.files.map((file) => file.path), ["src/a.ts", "src/b.ts"]);
});
