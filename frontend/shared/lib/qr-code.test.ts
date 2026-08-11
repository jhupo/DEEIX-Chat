import assert from "node:assert/strict";
import test from "node:test";

// @ts-expect-error Node's TypeScript runner requires an explicit extension.
import { createQRCodeSVG } from "./qr-code.ts";

test("creates QR codes for long checkout URLs", () => {
  const checkoutURL = `https://pay.example.test/checkout?token=${"a".repeat(700)}`;

  assert.match(createQRCodeSVG(checkoutURL), /^<svg /);
});

test("rejects payloads beyond the supported QR capacity", () => {
  assert.equal(createQRCodeSVG("a".repeat(859)), "");
});
