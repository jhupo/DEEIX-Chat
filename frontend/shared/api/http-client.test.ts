import assert from "node:assert/strict";
import test from "node:test";

// @ts-expect-error Node's TypeScript runner requires an explicit extension.
import { parseApiEnvelope } from "./http-client.ts";

test("accepts the canonical API response envelope", () => {
  assert.deepEqual(parseApiEnvelope({ errorMsg: "", data: { id: 7 } }), {
    data: { id: 7 },
    details: undefined,
    errorCode: undefined,
    errorMsg: "",
    requestId: undefined,
  });
});

test("rejects bare or malformed API responses", () => {
  assert.throws(() => parseApiEnvelope({ id: 7 }), /invalid api response envelope/);
  assert.throws(() => parseApiEnvelope({ errorMsg: null, data: null }), /invalid api response envelope/);
});
