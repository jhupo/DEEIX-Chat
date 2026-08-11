import assert from "node:assert/strict";
import test from "node:test";

// @ts-expect-error Node's TypeScript runner requires an explicit extension.
import { calculateTopUpPreview } from "./top-up-preview.ts";

test("top-up preview applies the payment currency, fee, and credit multiplier", () => {
  assert.deepEqual(calculateTopUpPreview(10, "CNY", 7.2, 5, 0.03), { paymentAmount: 74.16, creditedUSD: 50 });
});

test("top-up preview ignores invalid metadata", () => {
  assert.deepEqual(calculateTopUpPreview(10, "USD", null, 0, -1), { paymentAmount: 10, creditedUSD: 10 });
});
