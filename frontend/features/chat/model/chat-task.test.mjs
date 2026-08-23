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
    nativeToolKeys: ["openai.image_generation"],
    nativeTools: [{
      id: "openai.image_generation",
      key: "openai.image_generation",
      protocol: "openai_responses",
      protocols: ["openai_responses"],
      type: "image_generation",
      label: "Image Generation",
      enabled: true,
      defaultEnabled: true,
      payload: { type: "image_generation" },
    }],
    ...overrides,
  };
}

test("keeps image editing in chat when the model has a Responses image tool", () => {
  const decision = resolveChatSubmitDecision(model(), [imageAttachment]);
  assert.equal(decision.task, "chat");
  assert.equal(decision.blockedReason, null);
});

test("uses the dedicated edit task when no chat image tool is available", () => {
  const decision = resolveChatSubmitDecision(model({ nativeToolKeys: [], nativeTools: [] }), [imageAttachment]);
  assert.equal(decision.task, "image_edit");
});
