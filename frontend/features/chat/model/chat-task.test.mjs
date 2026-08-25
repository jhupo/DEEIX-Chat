import assert from "node:assert/strict";
import test from "node:test";

import { resolveChatSubmitDecision } from "./chat-task.ts";

const imageAttachment = {
  fileID: "file_image",
  fileName: "source.png",
  mimeType: "image/png",
  fileCategory: "image",
  sizeBytes: 128,
};

function model(overrides = {}) {
  return {
    platformModelName: "gpt-5.6-sol",
    icon: "openai",
    vendor: "openai",
    vendorName: "OpenAI",
    vendorIcon: "openai",
    displayGroupID: null,
    displayGroupName: "OpenAI",
    displayGroupIcon: "openai",
    kinds: ["chat", "image_edit"],
    protocols: ["openai_responses"],
    defaultOptions: {},
    optionControls: [],
    lockedOptionPaths: [],
    ...overrides,
  };
}

const imagePlugin = {
  kind: "app-mention",
  name: "image_generation",
  description: "Create or edit images",
  resourceRef: "plugin:image_generation",
};

test("keeps image editing in chat when the image Plugin is selected", () => {
  const decision = resolveChatSubmitDecision(model(), [imageAttachment], {}, [imagePlugin]);
  assert.equal(decision.task, "chat");
  assert.equal(decision.blockedReason, null);
});

test("uses the dedicated edit task when the image Plugin is not selected", () => {
  const decision = resolveChatSubmitDecision(model(), [imageAttachment]);
  assert.equal(decision.task, "image_edit");
});
