import type { WorkspaceRegistry } from "../config/workspace-registry.js";
import type { AgentCommand } from "../protocol/agent-command.js";
import type { ProviderAdapter } from "../providers/provider-adapter.js";
import type { ProviderRegistry } from "../providers/provider-registry.js";
import type {
	CommandJournal,
	JournalCommandState,
	TerminalOutcome,
} from "../wal/command-journal.js";
import {
	type ProviderCommand,
	resolveProviderCommand,
	type SourceRefRegistry,
} from "./resolve-provider-command.js";

export class GatewayCommandExecutor {
	readonly #journal: CommandJournal;
	readonly #workspaces: WorkspaceRegistry;
	readonly #sources: SourceRefRegistry;
	readonly #providers: ProviderRegistry;
	#queue: Promise<void> = Promise.resolve();
	#interactionQueue: Promise<void> = Promise.resolve();
	readonly #active = new Map<string, Promise<TerminalOutcome>>();

	constructor(
		journal: CommandJournal,
		workspaces: WorkspaceRegistry,
		sources: SourceRefRegistry,
		providers: ProviderRegistry,
	) {
		this.#journal = journal;
		this.#workspaces = workspaces;
		this.#sources = sources;
		this.#providers = providers;
	}

	async dispatch(
		serverSeq: number,
		commandId: string,
		value: unknown,
		signal: AbortSignal,
		onReceived?: (ackServerSeq: number) => Promise<void>,
	): Promise<TerminalOutcome> {
		let active = this.#active.get(commandId);
		const state = await this.#journal.receive(serverSeq, commandId, value);
		if (onReceived) await onReceived(this.#journal.contiguousReceipt());
		active ??= this.#active.get(commandId);
		if (active) return active;
		if (state.state === "terminal_cached") return state.outcome;
		const operation = () => this.#execute(state, signal);
		const result =
			state.command.kind === "interaction.respond"
				? this.#serializeInteraction(operation)
				: this.#serialize(operation);
		this.#active.set(commandId, result);
		try {
			return await result;
		} finally {
			if (this.#active.get(commandId) === result) this.#active.delete(commandId);
		}
	}

	async #execute(
		state: JournalCommandState,
		signal: AbortSignal,
	): Promise<TerminalOutcome> {
		const commandId = state.commandId;
		if (state.state === "terminal_cached") return state.outcome;
		if (state.state === "invocation_started")
			return this.#cacheUnknownOutcome(state);
		const command = state.command;
		if (command.kind === "transfer.execute") {
			return this.#cacheError(
				commandId,
				"transport_command_required",
				"transfer.execute must use the Gateway transport",
			);
		}

		let providerCommand: ProviderCommand;
		let adapter: ProviderAdapter;
		try {
			providerCommand = await resolveProviderCommand(
				commandId,
				command,
				this.#workspaces,
				this.#sources,
			);
			adapter = this.#providers.get(command.profileId);
			const manifest = adapter.capabilities();
			if (adapter.kind !== manifest.provider) {
				return this.#cacheError(
					commandId,
					"provider_manifest_mismatch",
					"Adapter kind does not match its manifest",
				);
			}
			if (!manifest.commands.includes(providerCommand.kind)) {
				return this.#cacheError(
					commandId,
					"unsupported_command",
					`Runtime does not support ${providerCommand.kind}`,
				);
			}
		} catch (error) {
			return this.#cacheError(
				commandId,
				"command_resolution_failed",
				errorMessage(error),
			);
		}

		await this.#journal.start(commandId, recoveryMarker(command));
		try {
			const result = await adapter.execute(providerCommand, signal);
			const terminal = await this.#journal.complete(commandId, {
				kind: "result",
				result,
			});
			return terminalOutcome(terminal);
		} catch (error) {
			return this.#cacheError(
				commandId,
				"provider_error",
				errorMessage(error),
			);
		}
	}

	async #cacheUnknownOutcome(
		state: Extract<JournalCommandState, { state: "invocation_started" }>,
	): Promise<TerminalOutcome> {
		return this.#cacheError(
			state.commandId,
			"outcome_unknown",
			`Previous ${state.command.kind} invocation has no cached terminal result`,
		);
	}

	async #cacheError(
		commandId: string,
		code: string,
		message: string,
	): Promise<TerminalOutcome> {
		const terminal = await this.#journal.complete(commandId, {
			kind: "error",
			error: { code, message },
		});
		return terminalOutcome(terminal);
	}

	async #serialize<T>(operation: () => Promise<T>): Promise<T> {
		const result = this.#queue.then(operation);
		this.#queue = result.then(
			() => undefined,
			() => undefined,
		);
		return result;
	}

	async #serializeInteraction<T>(operation: () => Promise<T>): Promise<T> {
		const result = this.#interactionQueue.then(operation);
		this.#interactionQueue = result.then(
			() => undefined,
			() => undefined,
		);
		return result;
	}
}

function recoveryMarker(command: AgentCommand): Record<string, unknown> {
	const marker: Record<string, unknown> = { commandKind: command.kind };
	if ("profileId" in command) marker.profileId = command.profileId;
	if ("sourceThreadRef" in command && command.sourceThreadRef !== undefined)
		marker.sourceThreadRef = command.sourceThreadRef;
	if ("sourceTurnRef" in command && command.sourceTurnRef !== undefined)
		marker.sourceTurnRef = command.sourceTurnRef;
	if (command.kind === "turn.start") marker.inputHashRequired = true;
	return marker;
}

function terminalOutcome(state: JournalCommandState): TerminalOutcome {
	if (state.state !== "terminal_cached")
		throw new Error("command did not reach a terminal state");
	return state.outcome;
}

function errorMessage(error: unknown): string {
	const message =
		error instanceof Error ? error.message : "Provider execution failed";
	return [...message]
		.map((character) => (character.charCodeAt(0) < 32 ? " " : character))
		.join("")
		.slice(0, 1024);
}
