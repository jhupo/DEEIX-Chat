import assert from "node:assert/strict";
import test from "node:test";

// @ts-expect-error Node's TypeScript runner requires an explicit extension.
import { resolveChatProtocol } from "./chat-protocol.ts";

test("resolves the administrator model protocol allowed by the selected key group", () => {
  assert.equal(resolveChatProtocol("openai", ["openai_chat_completions", "openai_responses"]), "openai_responses");
  assert.equal(resolveChatProtocol("anthropic", ["anthropic_messages"]), "anthropic_messages");
  assert.equal(resolveChatProtocol("gemini", ["google_generate_content"]), "google_generate_content");
  assert.equal(resolveChatProtocol("grok", ["xai_responses"]), "xai_responses");
  assert.equal(resolveChatProtocol("composite", ["anthropic_messages", "openai_responses"]), "openai_responses");
});

test("rejects missing keys and model protocols outside the key group", () => {
  assert.equal(resolveChatProtocol("", ["openai_responses"]), "");
  assert.equal(resolveChatProtocol("anthropic", ["openai_responses"]), "");
  assert.equal(resolveChatProtocol("unknown", ["openai_responses"]), "");
});
