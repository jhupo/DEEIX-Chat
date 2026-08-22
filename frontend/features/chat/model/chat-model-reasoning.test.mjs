import assert from "node:assert/strict";
import test from "node:test";

import {
  resolveChatModelReasoningSetting,
  setChatModelReasoningEffort,
} from "./chat-model-reasoning.ts";

test("resolves and updates a nested reasoning effort", () => {
  const setting = resolveChatModelReasoningSetting({
    options: { reasoning: { effort: "high" } },
    defaultOptions: {},
    optionControls: [{ path: "reasoning.effort", type: "select", options: ["low", "high"] }],
    lockedOptionPaths: [],
  });

  assert.deepEqual(setting, {
    path: "reasoning.effort",
    value: "high",
    options: ["low", "high"],
    disabled: false,
  });
  assert.deepEqual(
    setChatModelReasoningEffort({ temperature: 0.2 }, "reasoning.effort", "low"),
    { temperature: 0.2, reasoning: { effort: "low" } },
  );
});

test("uses the locked default instead of a stale user value", () => {
  const setting = resolveChatModelReasoningSetting({
    options: { reasoning: { effort: "low" } },
    defaultOptions: { reasoning: { effort: "high" } },
    optionControls: [{ path: "reasoning.effort", type: "select", options: ["low", "high"] }],
    lockedOptionPaths: ["reasoning.effort"],
  });

  assert.equal(setting?.value, "high");
  assert.equal(setting?.disabled, true);
});

test("selects a reasoning path allowed by the active protocol", () => {
  const setting = resolveChatModelReasoningSetting({
    options: {
      reasoning: { effort: "high" },
      reasoning_effort: "medium",
    },
    defaultOptions: {},
    optionControls: [
      { path: "reasoning.effort", type: "select" },
      { path: "reasoning_effort", type: "select" },
    ],
    lockedOptionPaths: [],
    isPathAvailable: (path) => path === "reasoning_effort",
  });

  assert.equal(setting?.path, "reasoning_effort");
  assert.equal(setting?.value, "medium");
});
