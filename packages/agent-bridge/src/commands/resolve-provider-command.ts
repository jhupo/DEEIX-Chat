import { randomUUID } from "node:crypto";
import type { WorkspaceRegistry } from "../config/workspace-registry.js";
import type { DurableWalStore, WalRecord } from "../wal/durable-wal-store.js";
import type {
	AgentCommand,
	InteractionResponse,
	ProfileResource,
	ReviewTarget,
	ThreadSettings,
	ThreadGitInfoPatch,
	WorkspaceResource,
} from "../protocol/agent-command.js";
import { assertOpaqueRef } from "../protocol/agent-command.js";

type LocalCommand = { commandId: string; profileRef: string };
type LocalWorkspaceCommand = LocalCommand & { canonicalCwd: string };
type LocalThreadCommand = LocalWorkspaceCommand & { providerThreadId: string };
type LocalTurnCommand = LocalThreadCommand & { providerTurnId: string };
export type ProviderInput =
	| { kind: "text"; text: string }
	| { kind: "local-image"; path: string }
	| { kind: "local-audio"; path: string };

export type ProviderCommand =
	| (LocalWorkspaceCommand & {
			kind: "thread.create";
			settings: ThreadSettings;
	  })
	| (LocalThreadCommand & {
			kind: "thread.lifecycle";
			action: "resume" | "fork" | "archive" | "unarchive" | "delete";
	  })
	| (LocalThreadCommand & { kind: "thread.rename"; name: string })
	| (LocalThreadCommand & { kind: "thread.metadata.update"; gitInfo: ThreadGitInfoPatch })
	| (LocalThreadCommand & { kind: "thread.compact" })
	| (LocalThreadCommand & { kind: "review.start"; target: ReviewTarget })
	| (LocalThreadCommand & {
			kind: "turn.start";
			input: ProviderInput[];
			settings: ThreadSettings;
	  })
	| (LocalTurnCommand & { kind: "turn.steer"; input: ProviderInput[] })
	| (LocalTurnCommand & { kind: "turn.interrupt" })
	| (LocalThreadCommand & {
			kind: "interaction.respond";
			scope: "thread";
			providerRequestId: string;
			response: InteractionResponse;
	  })
	| (LocalTurnCommand & {
			kind: "interaction.respond";
			scope: "turn";
			providerRequestId: string;
			response: InteractionResponse;
	  })
	| (LocalCommand & {
			kind: "resource.refresh";
			resource: { scope: "profile"; name: ProfileResource };
	  })
	| (LocalWorkspaceCommand & {
			kind: "resource.refresh";
			resource: { scope: "workspace"; name: WorkspaceResource };
	  });

type SourceKind = "thread" | "turn" | "item" | "request";

export class SourceRefRegistry {
	readonly #values = new Map<string, string>();
	readonly #references = new Map<string, string>();
	readonly #wal?: DurableWalStore;
	#queue: Promise<void> = Promise.resolve();

	constructor(wal?: DurableWalStore) {
		this.#wal = wal;
	}

	static restore(wal: DurableWalStore): SourceRefRegistry {
		const registry = new SourceRefRegistry(wal);
		for (const record of wal.records("source.map")) registry.#apply(record);
		return registry;
	}

	async register(
		profileId: string,
		kind: SourceKind,
		sourceRef: string,
		providerId: string,
	): Promise<void> {
		assertOpaqueRef(profileId, "profileId");
		assertOpaqueRef(sourceRef, "sourceRef");
		validateProviderId(providerId);
		await this.#transition(async () => {
			const mapping = { profileId, kind, sourceRef, providerId };
			this.#assertMapping(mapping);
			if (this.#values.has(sourceKey(profileId, kind, sourceRef))) return;
			const record = this.#wal
				? await this.#wal.append("source.map", mapping)
				: { version: 1 as const, sequence: 0, kind: "source.map", payload: mapping };
			this.#apply(record);
		});
	}

	async publish(
		profileId: string,
		kind: SourceKind,
		providerId: string,
	): Promise<string> {
		assertOpaqueRef(profileId, "profileId");
		validateProviderId(providerId);
		let result = "";
		await this.#transition(async () => {
			const key = providerKey(profileId, kind, providerId);
			const existing = this.#references.get(key);
			if (existing !== undefined) {
				result = existing;
				return;
			}
			const mapping = {
				profileId,
				kind,
				sourceRef: `${kind}_${randomUUID()}`,
				providerId,
			};
			const record = this.#wal
				? await this.#wal.append("source.map", mapping)
				: { version: 1 as const, sequence: 0, kind: "source.map", payload: mapping };
			this.#apply(record);
			result = mapping.sourceRef;
		});
		return result;
	}

	resolve(profileId: string, kind: SourceKind, sourceRef: string): string {
		const value = this.#values.get(sourceKey(profileId, kind, sourceRef));
		if (value === undefined)
			throw new Error(
				`${kind} source reference is not registered: ${sourceRef}`,
			);
		return value;
	}

	async #transition(operation: () => Promise<void>): Promise<void> {
		const result = this.#queue.then(operation);
		this.#queue = result.then(
			() => undefined,
			() => undefined,
		);
		await result;
	}

	#apply(record: WalRecord): void {
		if (record.kind !== "source.map") return;
		const mapping = parseMapping(record.payload);
		this.#assertMapping(mapping);
		this.#values.set(
			sourceKey(mapping.profileId, mapping.kind, mapping.sourceRef),
			mapping.providerId,
		);
		this.#references.set(
			providerKey(mapping.profileId, mapping.kind, mapping.providerId),
			mapping.sourceRef,
		);
	}

	#assertMapping(mapping: SourceMapping): void {
		const source = this.#values.get(
			sourceKey(mapping.profileId, mapping.kind, mapping.sourceRef),
		);
		if (source !== undefined && source !== mapping.providerId)
			throw new Error(`source reference cannot be rebound: ${mapping.sourceRef}`);
		const reference = this.#references.get(
			providerKey(mapping.profileId, mapping.kind, mapping.providerId),
		);
		if (reference !== undefined && reference !== mapping.sourceRef)
			throw new Error(`provider ID already has a source reference: ${mapping.providerId}`);
	}
}

type SourceMapping = {
	profileId: string;
	kind: SourceKind;
	sourceRef: string;
	providerId: string;
};

export async function resolveProviderCommand(
	commandId: string,
	command: AgentCommand,
	workspaces: WorkspaceRegistry,
	sources: SourceRefRegistry,
	artifacts: ReadonlyMap<string, { path: string; mimeType: string }> = new Map(),
): Promise<ProviderCommand> {
	assertOpaqueRef(commandId, "commandId");
	const base = { commandId, profileRef: command.profileId };
	if (isProfileResourceCommand(command)) {
		return { ...base, kind: command.kind, resource: command.resource };
	}

	const canonicalCwd = await workspaces.resolvePath(command.workspaceId, ".");
	const workspace = { ...base, canonicalCwd };
	if (command.kind === "thread.create")
		return { ...workspace, kind: command.kind, settings: command.settings };
	if (command.kind === "resource.refresh") {
		return { ...workspace, kind: command.kind, resource: command.resource };
	}

	const providerThreadId = sources.resolve(
		command.profileId,
		"thread",
		command.sourceThreadRef,
	);
	const thread = { ...workspace, providerThreadId };
	switch (command.kind) {
		case "thread.lifecycle":
			return { ...thread, kind: command.kind, action: command.action };
		case "thread.rename":
			return { ...thread, kind: command.kind, name: command.name };
		case "thread.metadata.update":
			return { ...thread, kind: command.kind, gitInfo: command.gitInfo };
		case "thread.compact":
			return { ...thread, kind: command.kind };
		case "review.start":
			return { ...thread, kind: command.kind, target: command.target };
		case "turn.start":
			return {
				...thread,
				kind: command.kind,
				input: providerInput(command.input, artifacts),
				settings: command.settings,
			};
		case "turn.steer":
			return {
				...thread,
				...turn(command, sources),
				kind: command.kind,
				input: providerInput(command.input, artifacts),
			};
		case "turn.interrupt":
			return { ...thread, ...turn(command, sources), kind: command.kind };
		case "interaction.respond": {
			const providerRequestId = sources.resolve(
				command.profileId,
				"request",
				command.sourceRequestRef,
			);
			return command.scope === "turn"
				? {
						...thread,
						...turn(command, sources),
						kind: command.kind,
						scope: command.scope,
						providerRequestId,
						response: command.response,
					}
				: {
						...thread,
						kind: command.kind,
						scope: command.scope,
						providerRequestId,
						response: command.response,
					};
		}
	}
}

function providerInput(
	input: Extract<AgentCommand, { kind: "turn.start" | "turn.steer" }>["input"],
	artifacts: ReadonlyMap<string, { path: string; mimeType: string }>,
): ProviderInput[] {
	return input.map((item) => {
		if (item.kind === "text") return item;
		const artifact = artifacts.get(item.artifactRef);
		if (!artifact) throw new Error(`artifact is not prepared: ${item.artifactRef}`);
		if (artifact.mimeType.startsWith("image/"))
			return { kind: "local-image", path: artifact.path };
		if (artifact.mimeType.startsWith("audio/"))
			return { kind: "local-audio", path: artifact.path };
		throw new Error(`artifact MIME is unsupported: ${artifact.mimeType}`);
	});
}

function isProfileResourceCommand(
	command: AgentCommand,
): command is Extract<AgentCommand, { kind: "resource.refresh" }> & {
	resource: { scope: "profile"; name: ProfileResource };
} {
	return (
		command.kind === "resource.refresh" && command.resource.scope === "profile"
	);
}

function turn(
	command:
		| Extract<AgentCommand, { kind: "turn.steer" | "turn.interrupt" }>
		| Extract<AgentCommand, { kind: "interaction.respond"; scope: "turn" }>,
	sources: SourceRefRegistry,
): { providerTurnId: string } {
	return {
		providerTurnId: sources.resolve(
			command.profileId,
			"turn",
			command.sourceTurnRef,
		),
	};
}

function sourceKey(
	profileId: string,
	kind: SourceKind,
	sourceRef: string,
): string {
	return `${profileId}\0${kind}\0${sourceRef}`;
}

function providerKey(
	profileId: string,
	kind: SourceKind,
	providerId: string,
): string {
	return `${profileId}\0${kind}\0${providerId}`;
}

function validateProviderId(providerId: unknown): asserts providerId is string {
	if (
		typeof providerId !== "string" ||
		providerId.length === 0 ||
		providerId.length > 4096 ||
		providerId.includes("\0")
	)
		throw new TypeError("providerId is invalid");
}

function parseMapping(value: unknown): SourceMapping {
	if (typeof value !== "object" || value === null || Array.isArray(value))
		throw new TypeError("source mapping is invalid");
	const profileId = Reflect.get(value, "profileId");
	const kind = Reflect.get(value, "kind");
	const sourceRef = Reflect.get(value, "sourceRef");
	const providerId = Reflect.get(value, "providerId");
	assertOpaqueRef(profileId, "source profileId");
	assertOpaqueRef(sourceRef, "source sourceRef");
	if (!["thread", "turn", "item", "request"].includes(kind))
		throw new TypeError("source kind is invalid");
	validateProviderId(providerId);
	return {
		profileId,
		kind: kind as SourceKind,
		sourceRef,
		providerId,
	};
}
