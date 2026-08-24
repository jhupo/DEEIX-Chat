import assert from "node:assert/strict";
import test from "node:test";

// @ts-expect-error Node's TypeScript runner requires an explicit extension.
import { shouldRefreshMessagesAfterHistory, shouldSurfaceConversationLoadError } from "./conversation-load-policy.ts";

test("keeps persisted messages visible when gateway history synchronization fails", () => {
  assert.equal(shouldSurfaceConversationLoadError(12), false);
  assert.equal(shouldSurfaceConversationLoadError(0), true);
});

test("refreshes messages only when history could add the initial page", () => {
  assert.equal(shouldRefreshMessagesAfterHistory(12, true), false);
  assert.equal(shouldRefreshMessagesAfterHistory(12, false), true);
  assert.equal(shouldRefreshMessagesAfterHistory(0, true), true);
});
