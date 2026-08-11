import assert from "node:assert/strict";
import test from "node:test";

// @ts-expect-error Node's TypeScript runner requires an explicit extension.
import { createIdempotencyKey } from "./idempotency-key.ts";

test("creates canonical v4 UUIDs with getRandomValues fallback", () => {
  const key = createIdempotencyKey({
    getRandomValues(bytes) {
      bytes.fill(0xff);
      return bytes;
    },
  });

  assert.match(key, /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/);
  assert.equal(key[14], "4");
  assert.match(key[19], /^[89ab]$/);
});
