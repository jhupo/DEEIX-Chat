import assert from "node:assert/strict";
import { mkdtemp, readFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { DeviceIdentity } from "../src/identity/device-identity.js";

test("device identity is persisted and reused", async () => {
	const directory = await mkdtemp(join(tmpdir(), "deeix-device-"));
	try {
		const path = join(directory, "device.key");
		const first = await DeviceIdentity.loadOrCreate(path);
		const second = await DeviceIdentity.loadOrCreate(path);
		assert.equal(second.publicKeyBase64Url(), first.publicKeyBase64Url());
		assert.equal(first.publicKeyBase64Url().length, 43);
		assert.match(first.signBase64Url("challenge"), /^[A-Za-z0-9_-]{86}$/);
		const stored = await readFile(path, "utf8");
		assert.doesNotMatch(stored, /publicKey/);
	} finally {
		await rm(directory, { recursive: true, force: true });
	}
});
