import assert from "node:assert/strict";
import test from "node:test";

// @ts-expect-error Node's TypeScript runner requires an explicit extension.
import { isUsableChatKeyBinding, resolveDefaultChatKeyBinding } from "./chat-key-binding-validity.ts";

const NOW = Date.parse("2026-08-11T00:00:00.000Z");

test("accepts active bindings without expiry and rejects invalid bindings", () => {
  assert.equal(isUsableChatKeyBinding({ status: "active", expiresAt: null }, NOW), true);
  assert.equal(isUsableChatKeyBinding({ status: "active", expiresAt: "2026-08-10T23:59:59.000Z" }, NOW), false);
  assert.equal(isUsableChatKeyBinding({ status: "active", expiresAt: "not-a-date" }, NOW), false);
  assert.equal(isUsableChatKeyBinding({ status: "revoked", expiresAt: null }, NOW), false);
});

test("resolves the configured usable binding and falls back to the first usable binding", () => {
  const bindings = [
    { publicID: "expired", status: "active", expiresAt: "2026-08-10T23:59:59.000Z" },
    { publicID: "first", status: "active", expiresAt: null },
    { publicID: "configured", status: "active", expiresAt: null },
  ];

  assert.equal(resolveDefaultChatKeyBinding(bindings, "configured", NOW), "configured");
  assert.equal(resolveDefaultChatKeyBinding(bindings, "missing", NOW), "first");
  assert.equal(resolveDefaultChatKeyBinding(bindings.slice(0, 1), "expired", NOW), "");
});
