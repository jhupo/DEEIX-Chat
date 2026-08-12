import { createHash } from "node:crypto";
import { assertOpaqueRef } from "../protocol/agent-command.js";
import type { ProviderEvent } from "../providers/provider-adapter.js";
import type { TerminalOutcome } from "./command-journal.js";
import type { DurableWalStore, WalRecord } from "./durable-wal-store.js";

export type OutgoingTerminalFrame = {
	type: "terminal";
	bridgeSeq: number;
	serverSeq: number;
	commandId: string;
	outcome: TerminalOutcome;
};

export type OutgoingEventFrame = {
	type: "event";
	bridgeSeq: number;
	event: ProviderEvent;
};

export type OutgoingBridgeFrame = OutgoingTerminalFrame | OutgoingEventFrame;
type FrameRecord = OutgoingBridgeFrame & { payloadHash: string };
type AckRecord = { ackBridgeSeq: number };

export class OutgoingFrameJournal {
	readonly #wal: DurableWalStore;
	readonly #frames = new Map<number, FrameRecord>();
	readonly #listeners = new Set<() => void>();
	#ackBridgeSeq = 0;
	#transitionQueue: Promise<void> = Promise.resolve();

	private constructor(wal: DurableWalStore) {
		this.#wal = wal;
	}

	static restore(wal: DurableWalStore): OutgoingFrameJournal {
		const journal = new OutgoingFrameJournal(wal);
		for (const record of wal.records()) journal.#apply(record);
		return journal;
	}

	acknowledgedSequence(): number {
		return this.#ackBridgeSeq;
	}

	pending(after = this.#ackBridgeSeq): OutgoingBridgeFrame[] {
		return [...this.#frames.values()]
			.filter((frame) => frame.bridgeSeq > after)
			.sort((left, right) => left.bridgeSeq - right.bridgeSeq)
			.map(({ payloadHash: _, ...frame }) => structuredClone(frame));
	}

	subscribe(listener: () => void): () => void {
		this.#listeners.add(listener);
		return () => this.#listeners.delete(listener);
	}

	async appendTerminal(
		serverSeq: number,
		commandId: string,
		outcome: TerminalOutcome,
	): Promise<OutgoingTerminalFrame> {
		if (!Number.isSafeInteger(serverSeq) || serverSeq < 1)
			throw new TypeError("serverSeq must be a positive safe integer");
		assertOpaqueRef(commandId, "commandId");
		validateOutcome(outcome);
		return this.#transition(async () => {
			for (const existing of this.#frames.values()) {
				if (existing.type !== "terminal" || existing.commandId !== commandId)
					continue;
				if (
					existing.serverSeq !== serverSeq ||
					existing.payloadHash !== hash(outcome)
				) {
					throw new Error("terminal frame cannot be replaced");
				}
				const { payloadHash: _, ...frame } = existing;
				return structuredClone(frame);
			}
			const frame: FrameRecord = {
				type: "terminal",
				bridgeSeq: this.#frames.size + 1,
				serverSeq,
				commandId,
				outcome,
				payloadHash: hash(outcome),
			};
			const record = await this.#wal.append("bridge.frame", frame);
			this.#apply(record);
			this.#notify();
			const { payloadHash: _, ...result } = frame;
			return structuredClone(result);
		});
	}

	async appendEvent(event: ProviderEvent): Promise<OutgoingEventFrame> {
		validateEvent(event);
		return this.#transition(async () => {
			const frame: FrameRecord = {
				type: "event",
				bridgeSeq: this.#frames.size + 1,
				event: structuredClone(event),
				payloadHash: hash(event),
			};
			const record = await this.#wal.append("bridge.frame", frame);
			this.#apply(record);
			this.#notify();
			const { payloadHash: _, ...result } = frame;
			return structuredClone(result);
		});
	}

	async acknowledge(through: number): Promise<void> {
		if (!Number.isSafeInteger(through) || through < 0)
			throw new TypeError("ackBridgeSeq is invalid");
		await this.#transition(async () => {
			if (through <= this.#ackBridgeSeq) return;
			const highest = this.#frames.size;
			if (through > highest)
				throw new Error("ackBridgeSeq exceeds the durable outgoing cursor");
			for (let sequence = this.#ackBridgeSeq + 1; sequence <= through; sequence += 1) {
				if (!this.#frames.has(sequence))
					throw new Error(`outgoing frame gap at ${sequence}`);
			}
			const record = await this.#wal.append("bridge.ack", {
				ackBridgeSeq: through,
			} satisfies AckRecord);
			this.#apply(record);
		});
	}

	async #transition<T>(operation: () => Promise<T>): Promise<T> {
		const result = this.#transitionQueue.then(operation);
		this.#transitionQueue = result.then(
			() => undefined,
			() => undefined,
		);
		return result;
	}

	#apply(record: WalRecord): void {
		if (record.kind === "bridge.frame") {
			const frame = parseFrame(record.payload);
			const expected = this.#frames.size + 1;
			if (frame.bridgeSeq !== expected || this.#frames.has(frame.bridgeSeq))
				throw new Error(`outgoing frame sequence mismatch at ${frame.bridgeSeq}`);
			this.#frames.set(frame.bridgeSeq, frame);
			return;
		}
		if (record.kind === "bridge.ack") {
			const ack = parseAck(record.payload);
			if (ack.ackBridgeSeq < this.#ackBridgeSeq || !this.#frames.has(ack.ackBridgeSeq))
				throw new Error("outgoing acknowledgment is invalid");
			this.#ackBridgeSeq = ack.ackBridgeSeq;
		}
	}

	#notify(): void {
		for (const listener of this.#listeners) listener();
	}
}

function parseFrame(value: unknown): FrameRecord {
	const frame = object(value, "outgoing frame");
	if (!Number.isSafeInteger(frame.bridgeSeq) || Number(frame.bridgeSeq) < 1)
		throw new TypeError("outgoing bridgeSeq is invalid");
	if (frame.type === "event") {
		validateEvent(frame.event);
		if (
			typeof frame.payloadHash !== "string" ||
			frame.payloadHash !== hash(frame.event)
		)
			throw new Error("outgoing frame hash is invalid");
		return {
			type: "event",
			bridgeSeq: Number(frame.bridgeSeq),
			event: frame.event,
			payloadHash: frame.payloadHash,
		};
	}
	if (frame.type !== "terminal")
		throw new TypeError("outgoing frame type is invalid");
	if (!Number.isSafeInteger(frame.serverSeq) || Number(frame.serverSeq) < 1)
		throw new TypeError("outgoing serverSeq is invalid");
	assertOpaqueRef(frame.commandId, "outgoing commandId");
	validateOutcome(frame.outcome);
	if (typeof frame.payloadHash !== "string" || frame.payloadHash !== hash(frame.outcome))
		throw new Error("outgoing frame hash is invalid");
	return {
		type: "terminal", bridgeSeq: Number(frame.bridgeSeq), serverSeq: Number(frame.serverSeq),
		commandId: frame.commandId, outcome: frame.outcome, payloadHash: frame.payloadHash,
	};
}

function validateEvent(value: unknown): asserts value is ProviderEvent {
	const event = object(value, "provider event");
	if (
		typeof event.kind !== "string" ||
		event.kind.length === 0 ||
		event.kind.length > 256 ||
		typeof event.occurredAt !== "string" ||
		Number.isNaN(Date.parse(event.occurredAt))
	)
		throw new TypeError("provider event metadata is invalid");
	for (const key of [
		"sourceThreadRef",
		"sourceTurnRef",
		"sourceItemRef",
		"sourceRequestRef",
	]) {
		if (event[key] !== undefined) assertOpaqueRef(event[key], `event.${key}`);
	}
	object(event.payload, "provider event payload");
}

function parseAck(value: unknown): AckRecord {
	const ack = object(value, "outgoing acknowledgment");
	if (!Number.isSafeInteger(ack.ackBridgeSeq) || Number(ack.ackBridgeSeq) < 1)
		throw new TypeError("outgoing ackBridgeSeq is invalid");
	return { ackBridgeSeq: Number(ack.ackBridgeSeq) };
}

function validateOutcome(value: unknown): asserts value is TerminalOutcome {
	const outcome = object(value, "terminal outcome");
	if (outcome.kind === "result") {
		object(outcome.result, "terminal result");
		return;
	}
	if (outcome.kind === "error") {
		const error = object(outcome.error, "terminal error");
		if (typeof error.code !== "string" || typeof error.message !== "string")
			throw new TypeError("terminal error is invalid");
		return;
	}
	throw new TypeError("terminal outcome kind is invalid");
}

function object(value: unknown, field: string): Record<string, unknown> {
	if (typeof value !== "object" || value === null || Array.isArray(value))
		throw new TypeError(`${field} must be an object`);
	return value as Record<string, unknown>;
}

function hash(value: unknown): string {
	return createHash("sha256").update(JSON.stringify(value)).digest("hex");
}
