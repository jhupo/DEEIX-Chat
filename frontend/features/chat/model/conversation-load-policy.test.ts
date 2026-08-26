import assert from "node:assert/strict";
import test from "node:test";

// @ts-expect-error Node's TypeScript runner requires an explicit extension.
import { isConversationStreamDisconnect, shouldPollConversationHistory, shouldRefreshMessagesAfterHistory, shouldReloadMessagesForExecutionBoundary, shouldRetryConversationStream, shouldSurfaceConversationLoadError } from "./conversation-load-policy.ts";

test("keeps persisted messages visible when gateway history synchronization fails", () => {
  assert.equal(shouldSurfaceConversationLoadError(12), false);
  assert.equal(shouldSurfaceConversationLoadError(0), true);
});

test("refreshes messages only when history could add the initial page", () => {
  assert.equal(shouldRefreshMessagesAfterHistory(12), false);
  assert.equal(shouldRefreshMessagesAfterHistory(0), true);
});

test("polls gateway history only while an empty conversation is being hydrated", () => {
  assert.equal(shouldPollConversationHistory(12, "syncing"), false);
  assert.equal(shouldPollConversationHistory(0, "syncing"), true);
  assert.equal(shouldPollConversationHistory(0, "loaded"), false);
  assert.equal(shouldPollConversationHistory(0, "error"), false);
});

test("reloads messages only when a gateway turn crosses the local message boundary", () => {
  assert.equal(shouldReloadMessagesForExecutionBoundary("turn/started", undefined), false);
  assert.equal(shouldReloadMessagesForExecutionBoundary("turn/started", "pending"), false);
  assert.equal(shouldReloadMessagesForExecutionBoundary("turn/completed", "pending"), true);
  assert.equal(shouldReloadMessagesForExecutionBoundary("turn/completed", undefined), true);
  assert.equal(shouldReloadMessagesForExecutionBoundary("turn/completed", "success"), false);
  assert.equal(shouldReloadMessagesForExecutionBoundary("item/agentMessage/delta", undefined), false);
});

test("reconnects recoverable conversation stream failures without surfacing an interruption", () => {
  assert.equal(isConversationStreamDisconnect({ name: "ConversationStreamDisconnectError" }), true);
  assert.equal(isConversationStreamDisconnect({ name: "ApiNetworkError" }), false);
  assert.equal(isConversationStreamDisconnect(new TypeError("render failed")), false);
  assert.equal(shouldRetryConversationStream({ name: "ApiNetworkError" }), true);
  assert.equal(shouldRetryConversationStream({ name: "ApiError", status: 404 }), true);
  assert.equal(
    shouldRetryConversationStream({
      name: "ApiError",
      status: 200,
      errorCode: "conversation_run.stream_interrupted",
    }),
    true,
  );
  assert.equal(shouldRetryConversationStream({ name: "ApiError", status: 400 }), false);
  assert.equal(shouldRetryConversationStream(new Error("invalid stream event")), false);
});
