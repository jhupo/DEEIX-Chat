import type { ProviderCommand } from "../commands/resolve-provider-command.js";
import type {
	AgentInput,
	ProfileResource,
	ThreadSettings,
	WorkspaceResource,
} from "../protocol/agent-command.js";

export type ProviderInteractionKind =
	| "command_approval"
	| "file_approval"
	| "user_input"
	| "permission"
	| "mcp_elicitation"
	| "dynamic_tool";

export type ProviderManifest = {
	provider: string;
	runtimeVersion: string;
	protocolVersion: string;
	schemaHash: string;
	commands: readonly ProviderCommand["kind"][];
	resources: {
		profile: readonly ProfileResource[];
		workspace: readonly WorkspaceResource[];
	};
	inputKinds: readonly AgentInput["kind"][];
	threadSettings: {
		model: boolean;
		reasoningEffort: readonly NonNullable<ThreadSettings["reasoningEffort"]>[];
		approvalPolicy: readonly NonNullable<ThreadSettings["approvalPolicy"]>[];
		sandboxPolicy: readonly NonNullable<ThreadSettings["sandboxPolicy"]>[];
	};
	interactionKinds: readonly ProviderInteractionKind[];
};

export type ProviderEvent = {
	kind: string;
	sourceThreadRef?: string;
	sourceTurnRef?: string;
	sourceItemRef?: string;
	sourceRequestRef?: string;
	occurredAt: string;
	payload: Record<string, unknown>;
};

export type ProviderResult =
	| { kind: "accepted" }
	| { kind: "thread-created"; sourceThreadRef: string }
	| { kind: "thread-forked"; sourceThreadRef: string }
	| { kind: "turn-started"; sourceTurnRef: string }
	| { kind: "resource"; resource: string; data: unknown };

export interface ProviderAdapter {
	readonly kind: string;
	start(
		onEvent: (event: ProviderEvent) => Promise<void>,
		signal: AbortSignal,
	): Promise<ProviderManifest>;
	proveRuntimeAuth(challenge: string, signal: AbortSignal): Promise<string>;
	execute(
		command: ProviderCommand,
		signal: AbortSignal,
	): Promise<ProviderResult>;
	capabilities(): ProviderManifest;
	close(): Promise<void>;
}
