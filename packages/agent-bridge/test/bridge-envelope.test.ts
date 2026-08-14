import assert from "node:assert/strict";
import test from "node:test";
import {
	BRIDGE_VERSION,
	parseBridgeAuthChallenge,
	parseBridgeAuthReady,
	parseBridgeServerFrame,
	parseBridgeWelcome,
} from "../src/protocol/bridge-envelope.js";

const deviceId = "agd_f6f910e920934def9a5cda479fc25251";

test("parseBridgeWelcome accepts the expected device", () => {
	assert.deepEqual(
		parseBridgeWelcome({ version: BRIDGE_VERSION, type: "welcome", deviceId, heartbeatSeconds: 30 }, deviceId),
		{ version: BRIDGE_VERSION, type: "welcome", deviceId, heartbeatSeconds: 30 },
	);
});

test("command frames require explicit artifact grants", () => {
	assert.deepEqual(parseBridgeServerFrame({
		version: BRIDGE_VERSION,
		type: "command",
		serverSeq: 1,
		commandId: "command_1",
		command: { kind: "resource.refresh" },
		artifacts: [],
	}), {
		version: BRIDGE_VERSION,
		type: "command",
		serverSeq: 1,
		commandId: "command_1",
		command: { kind: "resource.refresh" },
		artifacts: [],
	});
	assert.throws(() => parseBridgeServerFrame({
		version: BRIDGE_VERSION, type: "command", serverSeq: 1, commandId: "command_1",
		command: { kind: "resource.refresh" },
	}), /missing/);
});

test("parseBridgeWelcome rejects device substitution", () => {
	assert.throws(
		() => parseBridgeWelcome({ version: BRIDGE_VERSION, type: "welcome", deviceId: "agd_0123456789abcdef0123456789abcdef", heartbeatSeconds: 30 }, deviceId),
		/invalid/,
	);
});

test("runtime auth frames bind the expected profile", () => {
	const profileId = "profile_1";
	const expiresAt = new Date(Date.now() + 60_000).toISOString();
	const challenge = [
		"deeix-runtime-auth-proof-v1",
		"f6f910e920934def9a5cda479fc25251",
		deviceId,
		profileId,
		"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		"nonce",
		String(Math.floor(Date.now() / 1000) + 60),
	].join("\n");
	assert.equal(parseBridgeAuthChallenge({
		version: BRIDGE_VERSION, type: "auth.challenge", profileId,
		challengeId: "agp_0123456789abcdef0123456789abcdef", challenge, expiresAt,
	}, profileId).challenge, challenge);
	assert.equal(parseBridgeAuthReady({
		version: BRIDGE_VERSION, type: "auth.ready", profileId, leaseExpiresAt: expiresAt,
	}, profileId).profileId, profileId);
	assert.throws(() => parseBridgeAuthReady({
		version: BRIDGE_VERSION, type: "auth.ready", profileId: "profile_2", leaseExpiresAt: expiresAt,
	}, profileId), /invalid/);
});
