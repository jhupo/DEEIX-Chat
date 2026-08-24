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
