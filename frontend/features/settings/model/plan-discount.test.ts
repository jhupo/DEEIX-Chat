import assert from "node:assert/strict";
import test from "node:test";

// @ts-expect-error Node's TypeScript runner requires an explicit extension.
import { resolvePlanDiscountPercent } from "./plan-discount.ts";

test("plan discount is derived from the original and sale prices", () => {
  assert.equal(resolvePlanDiscountPercent(10000, 8000), 20);
  assert.equal(resolvePlanDiscountPercent(9999, 7499), 25);
});

test("plan discount is hidden for invalid or non-discounted prices", () => {
  assert.equal(resolvePlanDiscountPercent(0, 0), null);
  assert.equal(resolvePlanDiscountPercent(8000, 8000), null);
  assert.equal(resolvePlanDiscountPercent(8000, 9000), null);
  assert.equal(resolvePlanDiscountPercent(Number.NaN, 1000), null);
});
