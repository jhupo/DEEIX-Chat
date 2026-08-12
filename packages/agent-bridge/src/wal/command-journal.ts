import { createHash } from "node:crypto";
import type { AgentCommand } from "../protocol/agent-command.js";
import {
	assertOpaqueRef,
	parseAgentCommand,
} from "../protocol/agent-command.js";
import type { DurableWalStore, WalRecord } from "./durable-wal-store.js";

export type TerminalOutcome =
	| { kind: "result"; result: Record<string, unknown> }
	| { kind: "error"; error: { code: string; message: string } };

export type JournalCommandState =
	| {
			state: "received";
			serverSeq: number;
			commandId: string;
			command: AgentCommand;
			commandHash: string;
	  }
	| {
			state: "invocation_started";
			serverSeq: number;
			commandId: string;
			command: AgentCommand;
			commandHash: string;
			recovery: Record<string, unknown>;
	  }
	| {
			state: "terminal_cached";
			serverSeq: number;
			commandId: string;
			command: AgentCommand;
			commandHash: string;
			outcome: TerminalOutcome;
	  };

type ReceivedRecord = {
	serverSeq: number;
	commandId: string;
	command: AgentCommand;
	commandHash: string;
};

type StartedRecord = { commandId: string; recovery: Record<string, unknown> };
type TerminalRecord = { commandId: string; outcome: TerminalOutcome };

export class CommandJournal {
	readonly #wal: DurableWalStore;
	readonly #states = new Map<string, JournalCommandState>();
	readonly #serverSequences = new Map<number, string>();
	#transitionQueue: Promise<void> = Promise.resolve();

	private constructor(wal: DurableWalStore) {
		this.#wal = wal;
	}

	static restore(wal: DurableWalStore): CommandJournal {
		const journal = new CommandJournal(wal);
		for (const record of wal.records()) journal.#apply(record);
		return journal;
	}

	get(commandId: string): JournalCommandState | undefined {
		assertOpaqueRef(commandId, "commandId");
		const state = this.#states.get(commandId);
		return state === undefined ? undefined : structuredClone(state);
	}

	contiguousReceipt(after = 0): number {
		let sequence = after + 1;
		while (this.#serverSequences.has(sequence)) sequence += 1;
		return sequence - 1;
	}

	pendingRecovery(): JournalCommandState[] {
		return [...this.#states.values()]
			.filter((state) => state.state === "invocation_started")
			.sort((left, right) => left.serverSeq - right.serverSeq)
			.map((state) => structuredClone(state));
	}

	async receive(
		serverSeq: number,
		commandId: string,
		value: unknown,
	): Promise<JournalCommandState> {
		if (!Number.isSafeInteger(serverSeq) || serverSeq < 1)
			throw new TypeError("serverSeq must be a positive safe integer");
		assertOpaqueRef(commandId, "commandId");
		const command = parseAgentCommand(value);
		const commandHash = hash(command);
		return this.#transition(async () => {
			const existing = this.#states.get(commandId);
			if (existing) {
				if (
					existing.serverSeq !== serverSeq ||
					existing.commandHash !== commandHash
				)
					throw new Error("duplicate commandId has different content");
				return structuredClone(existing);
			}
			const sequenceOwner = this.#serverSequences.get(serverSeq);
			if (sequenceOwner !== undefined && sequenceOwner !== commandId)
				throw new Error("serverSeq is already assigned to another command");
			const payload: ReceivedRecord = {
				serverSeq,
				commandId,
				command,
				commandHash,
			};
			const record = await this.#wal.append("command.received", payload);
			this.#apply(record);
			return structuredClone(this.#required(commandId));
		});
	}

	async start(
		commandId: string,
		recovery: Record<string, unknown>,
	): Promise<JournalCommandState> {
		assertOpaqueRef(commandId, "commandId");
		assertSerializableObject(recovery, "recovery");
		return this.#transition(async () => {
			const state = this.#required(commandId);
			if (
				state.state === "terminal_cached" ||
				state.state === "invocation_started"
			)
				return structuredClone(state);
			const record = await this.#wal.append("command.invocation-started", {
				commandId,
				recovery,
			} satisfies StartedRecord);
			this.#apply(record);
			return structuredClone(this.#required(commandId));
		});
	}

	async complete(
		commandId: string,
		outcome: TerminalOutcome,
	): Promise<JournalCommandState> {
		assertOpaqueRef(commandId, "commandId");
		validateOutcome(outcome);
		return this.#transition(async () => {
			const state = this.#required(commandId);
			if (state.state === "terminal_cached") {
				if (hash(state.outcome) !== hash(outcome))
					throw new Error("terminal outcome cannot be replaced");
				return structuredClone(state);
			}
			const record = await this.#wal.append("command.terminal", {
				commandId,
				outcome,
			} satisfies TerminalRecord);
			this.#apply(record);
			return structuredClone(this.#required(commandId));
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

	#required(commandId: string): JournalCommandState {
		const state = this.#states.get(commandId);
		if (!state) throw new Error(`command is not received: ${commandId}`);
		return state;
	}

	#apply(record: WalRecord): void {
		switch (record.kind) {
			case "command.received": {
				const payload = receivedRecord(record.payload);
				if (this.#states.has(payload.commandId))
					throw new Error(
						`duplicate command.received record: ${payload.commandId}`,
					);
				const owner = this.#serverSequences.get(payload.serverSeq);
				if (owner !== undefined)
					throw new Error(`duplicate serverSeq in WAL: ${payload.serverSeq}`);
				this.#serverSequences.set(payload.serverSeq, payload.commandId);
				this.#states.set(payload.commandId, { state: "received", ...payload });
				return;
			}
			case "command.invocation-started": {
				const payload = startedRecord(record.payload);
				const state = this.#required(payload.commandId);
				if (state.state !== "received")
					throw new Error(
						`invalid invocation transition: ${payload.commandId}`,
					);
				this.#states.set(payload.commandId, {
					...state,
					state: "invocation_started",
					recovery: payload.recovery,
				});
				return;
			}
			case "command.terminal": {
				const payload = terminalRecord(record.payload);
				const state = this.#required(payload.commandId);
				if (state.state === "terminal_cached")
					throw new Error(`duplicate terminal record: ${payload.commandId}`);
				this.#states.set(payload.commandId, {
					state: "terminal_cached",
					serverSeq: state.serverSeq,
					commandId: state.commandId,
					command: state.command,
					commandHash: state.commandHash,
					outcome: payload.outcome,
				});
			}
		}
	}
}

function receivedRecord(value: unknown): ReceivedRecord {
	const payload = object(value, "received record");
	if (!Number.isSafeInteger(payload.serverSeq) || Number(payload.serverSeq) < 1)
		throw new TypeError("received serverSeq is invalid");
	assertOpaqueRef(payload.commandId, "received commandId");
	const command = parseAgentCommand(payload.command);
	const commandHash = string(payload.commandHash, "received commandHash");
	if (commandHash !== hash(command))
		throw new Error("received command hash is invalid");
	return {
		serverSeq: Number(payload.serverSeq),
		commandId: payload.commandId,
		command,
		commandHash,
	};
}

function startedRecord(value: unknown): StartedRecord {
	const payload = object(value, "started record");
	assertOpaqueRef(payload.commandId, "started commandId");
	const recovery = object(payload.recovery, "started recovery");
	assertSerializableObject(recovery, "started recovery");
	return { commandId: payload.commandId, recovery };
}

function terminalRecord(value: unknown): TerminalRecord {
	const payload = object(value, "terminal record");
	assertOpaqueRef(payload.commandId, "terminal commandId");
	validateOutcome(payload.outcome);
	return { commandId: payload.commandId, outcome: payload.outcome };
}

function validateOutcome(value: unknown): asserts value is TerminalOutcome {
	const outcome = object(value, "terminal outcome");
	if (outcome.kind === "result") {
		assertSerializableObject(outcome.result, "terminal result");
		return;
	}
	if (outcome.kind === "error") {
		const error = object(outcome.error, "terminal error");
		string(error.code, "terminal error code");
		string(error.message, "terminal error message");
		return;
	}
	throw new TypeError("terminal outcome kind is invalid");
}

function assertSerializableObject(
	value: unknown,
	field: string,
): asserts value is Record<string, unknown> {
	object(value, field);
	try {
		JSON.stringify(value);
	} catch {
		throw new TypeError(`${field} is not JSON serializable`);
	}
}

function object(value: unknown, field: string): Record<string, unknown> {
	if (typeof value !== "object" || value === null || Array.isArray(value))
		throw new TypeError(`${field} must be an object`);
	return value as Record<string, unknown>;
}

function string(value: unknown, field: string): string {
	if (typeof value !== "string" || value.length === 0 || value.length > 4096)
		throw new TypeError(`${field} is invalid`);
	return value;
}

function hash(value: unknown): string {
	return createHash("sha256").update(JSON.stringify(value)).digest("hex");
}
