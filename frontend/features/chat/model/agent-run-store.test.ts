import assert from "node:assert/strict";
import test from "node:test";

// @ts-expect-error Node's TypeScript runner requires an explicit extension.
import { applyAgentExecutionEvents, getAgentRunSnapshot, hasComposerAgentActivity, setAgentRunContext } from "./agent-run-store.ts";

test("applies a historical event page as one ordered batch", () => {
  setAgentRunContext("test-context", "conversation-1");

  const accepted = applyAgentExecutionEvents([
    {
      runID: "run-1",
      seq: 3,
      kind: "turn/completed",
      payload: { turn: { status: "completed" } },
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
  assert.equal(getAgentRunSnapshot("run-1").status, "completed");
  assert.deepEqual(getAgentRunSnapshot("run-1").plan.map((step) => step.text), ["Inspect"]);
  assert.equal(applyAgentExecutionEvents([{
    runID: "run-1",
    seq: 3,
    kind: "turn/completed",
    payload: { turn: { status: "completed" } },
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
        itemID: "file-item",
        item: {
          itemID: "file-item",
          kind: "fileChange",
          status: "inProgress",
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
        itemID: "file-item",
        item: {
          itemID: "file-item",
          kind: "fileChange",
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
  assert.equal(item.seq, 1);
  assert.equal(item.diff, "diff --git a/src/a.ts b/src/a.ts\n-old\n+new");
  assert.equal(item.truncated, true);
  assert.deepEqual(item.files.map((file) => file.path), ["src/a.ts", "src/b.ts"]);
});

test("keeps activity items in their first event order while applying later updates", () => {
  setAgentRunContext("timeline-test-context", "conversation-timeline");

  applyAgentExecutionEvents([
    {
      runID: "run-timeline",
      seq: 1,
      kind: "turn/plan/updated",
      payload: { plan: [{ step: "Inspect", status: "inProgress" }] },
      occurredAt: "2026-08-25T00:00:01Z",
    },
    {
      runID: "run-timeline",
      seq: 2,
      kind: "item/started",
      payload: { itemID: "reasoning-1", item: { itemID: "reasoning-1", kind: "reasoning", status: "inProgress", summary: ["Think"] } },
      occurredAt: "2026-08-25T00:00:02Z",
    },
    {
      runID: "run-timeline",
      seq: 3,
      kind: "item/started",
      payload: { itemID: "command-1", item: { itemID: "command-1", kind: "commandExecution", status: "inProgress", command: "pnpm test" } },
      occurredAt: "2026-08-25T00:00:03Z",
    },
    {
      runID: "run-timeline",
      seq: 4,
      kind: "item/fileChange/patchUpdated",
      payload: { itemID: "file-1", changes: [{ path: "src/app.ts", change: "update" }], patch: "+new" },
      occurredAt: "2026-08-25T00:00:04Z",
    },
    {
      runID: "run-timeline",
      seq: 5,
      kind: "item/started",
      payload: { itemID: "reasoning-2", item: { itemID: "reasoning-2", kind: "reasoning", status: "inProgress", summary: ["Reconsider"] } },
      occurredAt: "2026-08-25T00:00:05Z",
    },
    {
      runID: "run-timeline",
      seq: 6,
      kind: "item/completed",
      payload: { itemID: "command-1", item: { itemID: "command-1", kind: "commandExecution", command: "pnpm test", status: "completed" } },
      occurredAt: "2026-08-25T00:00:06Z",
    },
    {
      runID: "run-timeline",
      seq: 7,
      kind: "turn/plan/updated",
      payload: { plan: [{ step: "Inspect", status: "completed" }] },
      occurredAt: "2026-08-25T00:00:07Z",
    },
  ], "conversation-timeline");

  const run = getAgentRunSnapshot("run-timeline");
  assert.deepEqual(run.items.map((item) => `${item.kind}:${item.itemID}:${item.seq}`), [
    "reasoning:reasoning-1:2",
    "command:command-1:3",
    "file:file-1:4",
    "reasoning:reasoning-2:5",
  ]);
  assert.equal(run.planSeq, 1);
});

test("keeps compacted command output and reasoning when terminal items omit text", () => {
  setAgentRunContext("compact-history-context", "conversation-compact");
  applyAgentExecutionEvents([
    { runID: "run-compact", seq: 1, kind: "item/started", payload: { itemID: "command-1", item: { itemID: "command-1", kind: "commandExecution", status: "inProgress", command: "pnpm test" } }, occurredAt: "2026-08-26T00:00:01Z" },
    { runID: "run-compact", seq: 2, kind: "item/commandExecution/outputDelta", payload: { itemID: "command-1", outputDelta: "passed" }, occurredAt: "2026-08-26T00:00:02Z" },
    { runID: "run-compact", seq: 3, kind: "item/completed", payload: { itemID: "command-1", item: { itemID: "command-1", kind: "commandExecution", status: "completed" } }, occurredAt: "2026-08-26T00:00:03Z" },
    { runID: "run-compact", seq: 4, kind: "item/started", payload: { itemID: "reasoning-1", item: { itemID: "reasoning-1", kind: "reasoning", status: "inProgress", summary: [] } }, occurredAt: "2026-08-26T00:00:04Z" },
    { runID: "run-compact", seq: 5, kind: "item/reasoning/summaryTextDelta", payload: { itemID: "reasoning-1", delta: "checked history" }, occurredAt: "2026-08-26T00:00:05Z" },
    { runID: "run-compact", seq: 6, kind: "item/completed", payload: { itemID: "reasoning-1", item: { itemID: "reasoning-1", kind: "reasoning", status: "completed", summary: [] } }, occurredAt: "2026-08-26T00:00:06Z" },
  ], "conversation-compact");

  const [command, reasoning] = getAgentRunSnapshot("run-compact").items;
  assert.equal(command?.kind === "command" ? command.output : "", "passed");
  assert.equal(reasoning?.kind === "reasoning" ? reasoning.text : "", "checked history");
});

test("bounds command output accumulated from live events", () => {
  setAgentRunContext("command-output-context", "conversation-output");
  applyAgentExecutionEvents([
    { runID: "run-output", seq: 1, kind: "item/started", payload: { itemID: "command-1", item: { itemID: "command-1", kind: "commandExecution", status: "inProgress", command: "rg pattern" } }, occurredAt: "2026-08-27T00:00:01Z" },
    { runID: "run-output", seq: 2, kind: "item/commandExecution/outputDelta", payload: { itemID: "command-1", outputDelta: "x".repeat(256 * 1024 + 1) }, occurredAt: "2026-08-27T00:00:02Z" },
    { runID: "run-output", seq: 3, kind: "item/commandExecution/outputDelta", payload: { itemID: "command-1", outputDelta: "ignored" }, occurredAt: "2026-08-27T00:00:03Z" },
  ], "conversation-output");

  const item = getAgentRunSnapshot("run-output").items[0];
  assert.equal(item?.kind, "command");
  if (item?.kind !== "command") return;
  assert.equal(item.output.length, 256 * 1024);
  assert.equal(item.outputTruncated, true);
});

test("accumulates per-call token usage for one run", () => {
  setAgentRunContext("usage-context", "conversation-usage");
  applyAgentExecutionEvents([
    {
      runID: "run-usage",
      seq: 1,
      kind: "thread/tokenUsage/updated",
      payload: { tokenUsage: { last: { inputTokens: 10, cachedInputTokens: 8, outputTokens: 2, reasoningTokens: 1, totalTokens: 12 } } },
      occurredAt: "2026-08-27T00:00:01Z",
    },
    {
      runID: "run-usage",
      seq: 2,
      kind: "thread/tokenUsage/updated",
      payload: { tokenUsage: { last: { inputTokens: 20, cachedInputTokens: 16, outputTokens: 4, reasoningTokens: 2, totalTokens: 24 } } },
      occurredAt: "2026-08-27T00:00:02Z",
    },
  ], "conversation-usage");

  assert.deepEqual(getAgentRunSnapshot("run-usage").usage, {
    inputTokens: 30,
    cachedInputTokens: 24,
    outputTokens: 6,
    reasoningTokens: 3,
    totalTokens: 36,
    scope: "run",
  });
});

test("normalizes canonical tool items into the shared activity timeline", () => {
  setAgentRunContext("tool-context", "conversation-tool");
  applyAgentExecutionEvents([
    {
      runID: "run-tool",
      seq: 1,
      kind: "item/started",
      payload: {
        itemID: "tool-1",
        item: {
          itemID: "tool-1",
          kind: "dynamicToolCall",
          status: "inProgress",
          tool: "web_search",
          arguments: "latest Codex docs",
        },
      },
      occurredAt: "2026-08-27T00:00:01Z",
    },
    {
      runID: "run-tool",
      seq: 2,
      kind: "item/completed",
      payload: {
        itemID: "tool-1",
        item: {
          itemID: "tool-1",
          kind: "dynamicToolCall",
          status: "completed",
          tool: "web_search",
          result: "docs found",
        },
      },
      occurredAt: "2026-08-27T00:00:02Z",
    },
  ], "conversation-tool");

  const item = getAgentRunSnapshot("run-tool").items[0];
  assert.equal(item?.kind, "tool");
  if (item?.kind !== "tool") return;
  assert.equal(item.name, "web_search");
  assert.equal(item.input, "latest Codex docs");
  assert.equal(item.output, "docs found");
  assert.equal(item.status, "completed");
});
