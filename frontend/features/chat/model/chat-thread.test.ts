import assert from "node:assert/strict";
import test from "node:test";

// @ts-expect-error Node's TypeScript runner requires an explicit extension.
import { mapServerMessage } from "./chat-thread.ts";
import type { MessageDTO } from "@/shared/api/conversation.types";

function userMessage(publicID: string): MessageDTO {
  return {
    id: publicID === "message-1" ? 1 : 2,
    publicID,
    parentPublicID: "",
    sourcePublicID: "",
    role: "user",
    contentType: "text",
    content: publicID,
    attachments: "",
    branchReason: "default",
    runID: "shared-run",
    status: "success",
    errorCode: "",
    errorMessage: "",
    createdAt: "2026-09-01T00:00:00Z",
    updatedAt: "2026-09-01T00:00:00Z",
  } as MessageDTO;
}

test("uses the message identity when one run contains multiple messages with the same role", () => {
  const first = mapServerMessage(userMessage("message-1"));
  const second = mapServerMessage(userMessage("message-2"));

  assert.equal(first.key, "user-message-message-1");
  assert.equal(second.key, "user-message-message-2");
  assert.notEqual(first.key, second.key);
});
