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
