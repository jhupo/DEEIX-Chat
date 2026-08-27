import assert from "node:assert/strict";
import test from "node:test";

// @ts-expect-error Node's TypeScript runner requires an explicit extension.
import { parseAppVersionPayload } from "./app-version-payload.ts";

const version = {
  product: "DEEIX Chat",
  version: "0.4.105",
  commit: "abcdef12",
  buildTime: "2026-08-27T00:00:00Z",
  buildID: "0.4.105-abcdef12",
};

test("accepts the canonical version response", () => {
  assert.deepEqual(parseAppVersionPayload(version), version);
});

test("rejects the removed response envelope", () => {
  assert.throws(() => parseAppVersionPayload({ errorMsg: "", data: version }), /version response is invalid/);
});
