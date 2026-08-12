import { assertOpaqueRef, parseAgentCommand, type AgentCommand } from "./agent-command.js";
import type { TerminalOutcome } from "../wal/command-journal.js";
import type { ProviderEvent } from "../providers/provider-adapter.js";

export type BridgeHello = {
	version: 1;
	type: "hello";
	profileId: string;
	ackServerSeq: number;
	ackBridgeSeq: number;
};

export type BridgeAuthChallenge = {
	version: 1;
	type: "auth.challenge";
	profileId: string;
	challengeId: string;
	challenge: string;
	expiresAt: string;
};

export type BridgeAuthReady = {
	version: 1;
	type: "auth.ready";
	profileId: string;
	leaseExpiresAt: string;
};

export type BridgeWelcome = {
	version: 1;
	type: "welcome";
	deviceId: string;
	heartbeatSeconds: number;
	ackServerSeq?: number;
	ackBridgeSeq?: number;
};

export type BridgeCommandFrame = {
	version: 1;
	type: "command";
	serverSeq: number;
	commandId: string;
	command: unknown;
};

export type BridgeServerFrame =
	| BridgeCommandFrame
	| { version: 1; type: "pong" }
	| { version: 1; type: "ack.bridge"; ackBridgeSeq: number };

export type BridgeClientFrame =
	| BridgeHello
	| {
		version: 1;
		type: "auth.proof";
		profileId: string;
		challengeId: string;
		proof: string;
		workspaces: Array<{ workspaceId: string; name: string }>;
	  }
	| { version: 1; type: "ping" }
	| { version: 1; type: "ack.server"; ackServerSeq: number }
	| {
			version: 1;
			type: "terminal";
			bridgeSeq: number;
			serverSeq: number;
			commandId: string;
			outcome: TerminalOutcome;
	  }
	| {
			version: 1;
			type: "event";
			bridgeSeq: number;
			event: ProviderEvent;
	  };

export function parseBridgeWelcome(value: unknown, expectedDeviceId: string): BridgeWelcome {
	const source = object(value, "bridge welcome");
	exact(source, ["version", "type", "deviceId", "heartbeatSeconds"], ["ackServerSeq", "ackBridgeSeq"]);
	const version = source.version;
	const type = source.type;
	const deviceId = source.deviceId;
	const heartbeatSeconds = source.heartbeatSeconds;
	if (
		version !== 1 || type !== "welcome" || deviceId !== expectedDeviceId ||
		typeof heartbeatSeconds !== "number" || !Number.isSafeInteger(heartbeatSeconds) ||
		heartbeatSeconds < 5 || heartbeatSeconds > 300
	) {
		throw new TypeError("bridge welcome is invalid");
	}
	const result: BridgeWelcome = { version, type, deviceId, heartbeatSeconds };
	if (source.ackServerSeq !== undefined)
		result.ackServerSeq = cursor(source.ackServerSeq, "welcome.ackServerSeq");
	if (source.ackBridgeSeq !== undefined)
		result.ackBridgeSeq = cursor(source.ackBridgeSeq, "welcome.ackBridgeSeq");
	return result;
}

export function parseBridgeAuthChallenge(value: unknown, expectedProfileId: string): BridgeAuthChallenge {
	const source = object(value, "bridge auth challenge");
	exact(source, ["version", "type", "profileId", "challengeId", "challenge", "expiresAt"]);
	if (
		source.version !== 1 || source.type !== "auth.challenge" || source.profileId !== expectedProfileId ||
		typeof source.challengeId !== "string" || !/^agp_[a-f0-9]{32}$/.test(source.challengeId) ||
		typeof source.challenge !== "string" || source.challenge.length > 1024 ||
		!source.challenge.startsWith("deeix-runtime-auth-proof-v1\n") || source.challenge.split("\n").length !== 7 ||
		typeof source.expiresAt !== "string" || !validTime(source.expiresAt)
	) {
		throw new TypeError("bridge auth challenge is invalid");
	}
	return {
		version: 1, type: "auth.challenge", profileId: source.profileId,
		challengeId: source.challengeId, challenge: source.challenge, expiresAt: source.expiresAt,
	};
}

export function parseBridgeAuthReady(value: unknown, expectedProfileId: string): BridgeAuthReady {
	const source = object(value, "bridge auth ready");
	exact(source, ["version", "type", "profileId", "leaseExpiresAt"]);
	if (
		source.version !== 1 || source.type !== "auth.ready" || source.profileId !== expectedProfileId ||
		typeof source.leaseExpiresAt !== "string" || !validTime(source.leaseExpiresAt)
	) {
		throw new TypeError("bridge auth ready is invalid");
	}
	return { version: 1, type: "auth.ready", profileId: source.profileId, leaseExpiresAt: source.leaseExpiresAt };
}

export function parseBridgeServerFrame(value: unknown): BridgeServerFrame {
	const source = object(value, "bridge frame");
	if (source.version !== 1 || typeof source.type !== "string")
		throw new TypeError("bridge frame metadata is invalid");
	switch (source.type) {
		case "pong":
			exact(source, ["version", "type"]);
			return { version: 1, type: "pong" };
		case "ack.bridge":
			exact(source, ["version", "type", "ackBridgeSeq"]);
			return { version: 1, type: "ack.bridge", ackBridgeSeq: positiveCursor(source.ackBridgeSeq, "ack.bridge.ackBridgeSeq") };
		case "command":
			exact(source, ["version", "type", "serverSeq", "commandId", "command"]);
			assertOpaqueRef(source.commandId, "commandId");
			return {
				version: 1, type: "command",
				serverSeq: positiveCursor(source.serverSeq, "command.serverSeq"),
				commandId: source.commandId,
				command: source.command,
			};
		default:
			throw new TypeError(`unsupported bridge frame type: ${source.type}`);
	}
}

export function parseBridgeCommand(frame: BridgeCommandFrame): AgentCommand {
	return parseAgentCommand(frame.command);
}

function object(value: unknown, field: string): Record<string, unknown> {
	if (typeof value !== "object" || value === null || Array.isArray(value))
		throw new TypeError(`${field} must be an object`);
	return value as Record<string, unknown>;
}

function exact(value: Record<string, unknown>, required: string[], optional: string[] = []): void {
	const allowed = new Set([...required, ...optional]);
	for (const key of Object.keys(value)) {
		if (!allowed.has(key)) throw new TypeError(`unexpected bridge frame field: ${key}`);
	}
	for (const key of required) {
		if (!(key in value)) throw new TypeError(`missing bridge frame field: ${key}`);
	}
}

function cursor(value: unknown, field: string): number {
	if (typeof value !== "number" || !Number.isSafeInteger(value) || value < 0)
		throw new TypeError(`${field} is invalid`);
	return value;
}

function positiveCursor(value: unknown, field: string): number {
	const result = cursor(value, field);
	if (result === 0) throw new TypeError(`${field} is invalid`);
	return result;
}

function validTime(value: string): boolean {
	return Number.isFinite(Date.parse(value));
}
