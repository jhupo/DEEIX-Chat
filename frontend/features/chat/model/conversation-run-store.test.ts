import assert from "node:assert/strict";
import test from "node:test";

// @ts-expect-error Node's TypeScript runner requires an explicit extension.
import { ConversationRunStore } from "./conversation-run-store.ts";

test("confirms a locally registered run only after the server snapshot contains it", () => {
  const store = new ConversationRunStore();
  let notifications = 0;
  store.subscribeRun("run_1", () => {
    notifications += 1;
  });

  store.register("run_1", "conversation_1");
  assert.equal(store.isServerRunActive("run_1", "conversation_1"), false);

  store.synchronize([{ runID: "run_1", conversationPublicID: "conversation_1" }]);
  assert.equal(store.isServerRunActive("run_1", "conversation_1"), true);
  assert.equal(notifications, 1);
});

test("does not treat a desktop-only pending run as a resumable server run", () => {
  const store = new ConversationRunStore();
  store.synchronize([]);

  assert.equal(store.isServerRunActive("run_desktop", "conversation_1"), false);
});

test("removes server run authority when it disappears from the snapshot", () => {
  const store = new ConversationRunStore();
  store.applyStarted("run_1", "conversation_1");
  assert.equal(store.isServerRunActive("run_1", "conversation_1"), true);

  store.synchronize([]);
  assert.equal(store.isServerRunActive("run_1", "conversation_1"), false);
});
