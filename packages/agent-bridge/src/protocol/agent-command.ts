export type OpaqueRef = string;

export type AgentInput =
	| { kind: "text"; text: string }
	| { kind: "artifact"; artifactRef: OpaqueRef };

export type ThreadSettings = {
	model?: string;
	reasoningEffort?: "low" | "medium" | "high" | "xhigh";
	approvalPolicy?: "untrusted" | "on-request" | "never";
	sandboxPolicy?: "read-only" | "workspace-write";
};

export type TurnSettings = ThreadSettings;

export type InteractionResponse =
	| { kind: "approval"; decision: "accept" | "decline" }
	| { kind: "user-input"; answers: Record<string, string> }
	| {
			kind: "permission";
			decision: "accept" | "decline";
			scope?: "turn" | "session";
	  }
	| {
			kind: "dynamic-tool";
			success: boolean;
			content: Array<
				| { kind: "text"; text: string }
				| { kind: "image"; url: string }
				| { kind: "audio"; url: string }
			>;
	  }
	| {
			kind: "mcp-elicitation";
			decision: "accept" | "decline";
			content?: Record<string, string>;
	  };

export type ReviewTarget =
	| { kind: "working-tree" }
	| { kind: "base-branch"; branch: string }
	| { kind: "commit"; sha: string };

export type ProfileResource =
	| "models"
	| "permission-profiles"
	| "apps"
	| "mcp"
	| "plugins"
	| "auth-status";
export type WorkspaceResource = "sessions" | "skills" | "hooks";

type CommandTarget = {
	deviceId: OpaqueRef;
	profileId: OpaqueRef;
};

type WorkspaceTarget = CommandTarget & {
	workspaceId: OpaqueRef;
};

type ThreadTarget = WorkspaceTarget & {
	threadId: OpaqueRef;
	sourceThreadRef: OpaqueRef;
};

type TurnTarget = ThreadTarget & {
	turnId: OpaqueRef;
	sourceTurnRef: OpaqueRef;
};

export type TransferCommand = {
	kind: "transfer.execute";
	deviceId: OpaqueRef;
	workspaceId: OpaqueRef;
	threadId?: OpaqueRef;
	sourceThreadRef?: OpaqueRef;
	transferTicketRef: OpaqueRef;
};

export type AgentCommand =
	| (WorkspaceTarget & { kind: "thread.create"; settings: ThreadSettings })
	| (ThreadTarget & {
			kind: "thread.lifecycle";
			action: "resume" | "fork" | "archive" | "unarchive" | "delete";
	  })
	| (ThreadTarget & { kind: "thread.rename"; name: string })
	| (ThreadTarget & { kind: "thread.compact" })
	| (ThreadTarget & { kind: "review.start"; target: ReviewTarget })
	| (ThreadTarget & {
			kind: "turn.start";
			input: AgentInput[];
			settings: TurnSettings;
	  })
	| (TurnTarget & { kind: "turn.steer"; input: AgentInput[] })
	| (TurnTarget & { kind: "turn.interrupt" })
	| (TurnTarget & {
			kind: "interaction.respond";
			scope: "turn";
			interactionId: OpaqueRef;
			sourceRequestRef: OpaqueRef;
			response: InteractionResponse;
	  })
	| (ThreadTarget & {
			kind: "interaction.respond";
			scope: "thread";
			interactionId: OpaqueRef;
			sourceRequestRef: OpaqueRef;
			response: InteractionResponse;
	  })
	| (CommandTarget & {
			kind: "resource.refresh";
			resource: { scope: "profile"; name: ProfileResource };
	  })
	| (WorkspaceTarget & {
			kind: "resource.refresh";
			resource: { scope: "workspace"; name: WorkspaceResource };
	  })
	| TransferCommand;

const REF_PATTERN = /^[A-Za-z0-9][A-Za-z0-9._:-]{0,255}$/;
const MAX_TEXT_BYTES = 1024 * 1024;

export function assertOpaqueRef(
	value: unknown,
	field: string,
): asserts value is OpaqueRef {
	if (typeof value !== "string" || !REF_PATTERN.test(value)) {
		throw new TypeError(`${field} must be an opaque reference`);
	}
}

export function parseAgentCommand(value: unknown): AgentCommand {
	const command = object(value, "command");
	const kind = string(command.kind, "command.kind");

	switch (kind) {
		case "thread.create":
			exact(command, [
				"kind",
				"deviceId",
				"profileId",
				"workspaceId",
				"settings",
			]);
			return {
				kind,
				...workspace(command),
				settings: settings(command.settings, "command.settings"),
			};
		case "thread.lifecycle": {
			exact(command, ["kind", ...threadKeys(), "action"]);
			const action = oneOf(command.action, "command.action", [
				"resume",
				"fork",
				"archive",
				"unarchive",
				"delete",
			] as const);
			return { kind, ...thread(command), action };
		}
		case "thread.rename":
			exact(command, ["kind", ...threadKeys(), "name"]);
			return {
				kind,
				...thread(command),
				name: boundedString(command.name, "command.name", 256),
			};
		case "thread.compact":
			exact(command, ["kind", ...threadKeys()]);
			return { kind, ...thread(command) };
		case "review.start":
			exact(command, ["kind", ...threadKeys(), "target"]);
			return { kind, ...thread(command), target: reviewTarget(command.target) };
		case "turn.start":
			exact(command, ["kind", ...threadKeys(), "input", "settings"]);
			return {
				kind,
				...thread(command),
				input: inputs(command.input),
				settings: settings(command.settings, "command.settings"),
			};
		case "turn.steer":
			exact(command, ["kind", ...turnKeys(), "input"]);
			return { kind, ...turn(command), input: inputs(command.input) };
		case "turn.interrupt":
			exact(command, ["kind", ...turnKeys()]);
			return { kind, ...turn(command) };
		case "interaction.respond":
			return interaction(command);
		case "resource.refresh":
			return resourceRefresh(command);
		case "transfer.execute":
			return transfer(command);
		default:
			throw new TypeError(`unsupported command kind: ${kind}`);
	}
}

function interaction(command: Record<string, unknown>): AgentCommand {
	const scope = oneOf(command.scope, "command.scope", [
		"thread",
		"turn",
	] as const);
	const keys = [
		"kind",
		...threadKeys(),
		"scope",
		"interactionId",
		"sourceRequestRef",
		"response",
	];
	if (scope === "turn") keys.push("turnId", "sourceTurnRef");
	exact(command, keys);
	const common = {
		kind: "interaction.respond" as const,
		...thread(command),
		interactionId: ref(command.interactionId, "command.interactionId"),
		sourceRequestRef: ref(command.sourceRequestRef, "command.sourceRequestRef"),
		response: interactionResponse(command.response),
	};
	return scope === "turn"
		? { ...common, scope, ...turnOnly(command) }
		: { ...common, scope };
}

function resourceRefresh(command: Record<string, unknown>): AgentCommand {
	const resource = object(command.resource, "command.resource");
	const scope = oneOf(resource.scope, "command.resource.scope", [
		"profile",
		"workspace",
	] as const);
	exact(resource, ["scope", "name"]);
	if (scope === "profile") {
		exact(command, ["kind", "deviceId", "profileId", "resource"]);
		return {
			kind: "resource.refresh",
			...target(command),
			resource: {
				scope,
				name: oneOf(resource.name, "command.resource.name", [
					"models",
					"permission-profiles",
					"apps",
					"mcp",
					"plugins",
					"auth-status",
				] as const),
			},
		};
	}
	exact(command, ["kind", "deviceId", "profileId", "workspaceId", "resource"]);
	return {
		kind: "resource.refresh",
		...workspace(command),
		resource: {
			scope,
			name: oneOf(resource.name, "command.resource.name", [
				"sessions",
				"skills",
				"hooks",
			] as const),
		},
	};
}

function transfer(command: Record<string, unknown>): TransferCommand {
	const allowed = ["kind", "deviceId", "workspaceId", "transferTicketRef"];
	if (command.threadId !== undefined || command.sourceThreadRef !== undefined) {
		allowed.push("threadId", "sourceThreadRef");
		if (
			command.threadId === undefined ||
			command.sourceThreadRef === undefined
		) {
			throw new TypeError(
				"command.threadId and command.sourceThreadRef must be provided together",
			);
		}
	}
	exact(command, allowed);
	const result: TransferCommand = {
		kind: "transfer.execute",
		deviceId: ref(command.deviceId, "command.deviceId"),
		workspaceId: ref(command.workspaceId, "command.workspaceId"),
		transferTicketRef: ref(
			command.transferTicketRef,
			"command.transferTicketRef",
		),
	};
	if (command.threadId !== undefined) {
		result.threadId = ref(command.threadId, "command.threadId");
		result.sourceThreadRef = ref(
			command.sourceThreadRef,
			"command.sourceThreadRef",
		);
	}
	return result;
}

function interactionResponse(value: unknown): InteractionResponse {
	const response = object(value, "command.response");
	const kind = oneOf(response.kind, "command.response.kind", [
		"approval",
		"user-input",
		"permission",
		"dynamic-tool",
		"mcp-elicitation",
	] as const);
	if (kind === "approval") {
		exact(response, ["kind", "decision"]);
		return {
			kind,
			decision: oneOf(response.decision, "command.response.decision", [
				"accept",
				"decline",
			] as const),
		};
	}
	if (kind === "user-input") {
		exact(response, ["kind", "answers"]);
		return {
			kind,
			answers: stringRecord(response.answers, "command.response.answers"),
		};
	}
	if (kind === "permission") {
		exact(response, ["kind", "decision", "scope"], true);
		const decision = oneOf(response.decision, "command.response.decision", [
			"accept",
			"decline",
		] as const);
		if (response.scope === undefined) return { kind, decision };
		return {
			kind,
			decision,
			scope: oneOf(response.scope, "command.response.scope", [
				"turn",
				"session",
			] as const),
		};
	}
	if (kind === "dynamic-tool") {
		exact(response, ["kind", "success", "content"]);
		if (typeof response.success !== "boolean")
			throw new TypeError("command.response.success must be a boolean");
		if (!Array.isArray(response.content) || response.content.length > 64)
			throw new TypeError("command.response.content is invalid");
		return {
			kind,
			success: response.success,
			content: response.content.map((item, index) => {
				const content = object(item, `command.response.content[${index}]`);
				const contentKind = oneOf(
					content.kind,
					`command.response.content[${index}].kind`,
					["text", "image", "audio"] as const,
				);
				exact(content, ["kind", contentKind === "text" ? "text" : "url"]);
				return contentKind === "text"
					? {
							kind: contentKind,
							text: boundedString(
								content.text,
								`command.response.content[${index}].text`,
								MAX_TEXT_BYTES,
							),
						}
					: {
							kind: contentKind,
							url: boundedString(
								content.url,
								`command.response.content[${index}].url`,
								16 * 1024,
							),
						};
			}),
		};
	}
	const keys = ["kind", "decision"];
	if (response.content !== undefined) keys.push("content");
	exact(response, keys);
	const decision = oneOf(response.decision, "command.response.decision", [
		"accept",
		"decline",
	] as const);
	if (decision === "decline" && response.content !== undefined) {
		throw new TypeError("declined MCP elicitation must not include content");
	}
	return response.content === undefined
		? { kind, decision }
		: {
				kind,
				decision,
				content: stringRecord(response.content, "command.response.content"),
			};
}

function reviewTarget(value: unknown): ReviewTarget {
	const target = object(value, "command.target");
	const kind = oneOf(target.kind, "command.target.kind", [
		"working-tree",
		"base-branch",
		"commit",
	] as const);
	if (kind === "working-tree") {
		exact(target, ["kind"]);
		return { kind };
	}
	if (kind === "base-branch") {
		exact(target, ["kind", "branch"]);
		return {
			kind,
			branch: boundedString(target.branch, "command.target.branch", 256),
		};
	}
	exact(target, ["kind", "sha"]);
	const sha = string(target.sha, "command.target.sha");
	if (!/^[0-9a-fA-F]{7,64}$/.test(sha))
		throw new TypeError("command.target.sha is invalid");
	return { kind, sha };
}

function inputs(value: unknown): AgentInput[] {
	if (!Array.isArray(value) || value.length === 0 || value.length > 64) {
		throw new TypeError("command.input must contain 1 to 64 items");
	}
	let textBytes = 0;
	return value.map((item, index) => {
		const input = object(item, `command.input[${index}]`);
		const kind = oneOf(input.kind, `command.input[${index}].kind`, [
			"text",
			"artifact",
		] as const);
		if (kind === "text") {
			exact(input, ["kind", "text"]);
			const text = boundedString(
				input.text,
				`command.input[${index}].text`,
				MAX_TEXT_BYTES,
			);
			textBytes += Buffer.byteLength(text);
			if (textBytes > MAX_TEXT_BYTES)
				throw new TypeError("command.input text exceeds 1 MiB");
			return { kind, text };
		}
		exact(input, ["kind", "artifactRef"]);
		return {
			kind,
			artifactRef: ref(
				input.artifactRef,
				`command.input[${index}].artifactRef`,
			),
		};
	});
}

function settings(value: unknown, field: string): ThreadSettings {
	const source = object(value, field);
	exact(
		source,
		["model", "reasoningEffort", "approvalPolicy", "sandboxPolicy"],
		true,
	);
	const result: ThreadSettings = {};
	if (source.model !== undefined)
		result.model = boundedString(source.model, `${field}.model`, 256);
	if (source.reasoningEffort !== undefined)
		result.reasoningEffort = oneOf(
			source.reasoningEffort,
			`${field}.reasoningEffort`,
			["low", "medium", "high", "xhigh"] as const,
		);
	if (source.approvalPolicy !== undefined)
		result.approvalPolicy = oneOf(
			source.approvalPolicy,
			`${field}.approvalPolicy`,
			["untrusted", "on-request", "never"] as const,
		);
	if (source.sandboxPolicy !== undefined)
		result.sandboxPolicy = oneOf(
			source.sandboxPolicy,
			`${field}.sandboxPolicy`,
			["read-only", "workspace-write"] as const,
		);
	return result;
}

function target(value: Record<string, unknown>): CommandTarget {
	return {
		deviceId: ref(value.deviceId, "command.deviceId"),
		profileId: ref(value.profileId, "command.profileId"),
	};
}

function workspace(value: Record<string, unknown>): WorkspaceTarget {
	return {
		...target(value),
		workspaceId: ref(value.workspaceId, "command.workspaceId"),
	};
}

function thread(value: Record<string, unknown>): ThreadTarget {
	return {
		...workspace(value),
		threadId: ref(value.threadId, "command.threadId"),
		sourceThreadRef: ref(value.sourceThreadRef, "command.sourceThreadRef"),
	};
}

function turnOnly(
	value: Record<string, unknown>,
): Pick<TurnTarget, "turnId" | "sourceTurnRef"> {
	return {
		turnId: ref(value.turnId, "command.turnId"),
		sourceTurnRef: ref(value.sourceTurnRef, "command.sourceTurnRef"),
	};
}

function turn(value: Record<string, unknown>): TurnTarget {
	return { ...thread(value), ...turnOnly(value) };
}

function threadKeys(): string[] {
	return [
		"deviceId",
		"profileId",
		"workspaceId",
		"threadId",
		"sourceThreadRef",
	];
}

function turnKeys(): string[] {
	return [...threadKeys(), "turnId", "sourceTurnRef"];
}

function ref(value: unknown, field: string): string {
	assertOpaqueRef(value, field);
	return value;
}

function object(value: unknown, field: string): Record<string, unknown> {
	if (typeof value !== "object" || value === null || Array.isArray(value))
		throw new TypeError(`${field} must be an object`);
	return value as Record<string, unknown>;
}

function string(value: unknown, field: string): string {
	if (typeof value !== "string")
		throw new TypeError(`${field} must be a string`);
	return value;
}

function boundedString(
	value: unknown,
	field: string,
	maxBytes: number,
): string {
	const result = string(value, field);
	if (
		result.length === 0 ||
		Buffer.byteLength(result) > maxBytes ||
		[...result].some(
			(character) =>
				character.charCodeAt(0) < 32 &&
				character !== "\n" &&
				character !== "\r" &&
				character !== "\t",
		)
	) {
		throw new TypeError(
			`${field} is empty, too large, or contains control characters`,
		);
	}
	return result;
}

function oneOf<const T extends readonly string[]>(
	value: unknown,
	field: string,
	allowed: T,
): T[number] {
	if (typeof value !== "string" || !allowed.includes(value))
		throw new TypeError(`${field} is invalid`);
	return value as T[number];
}

function exact(
	value: Record<string, unknown>,
	allowed: string[],
	optional = false,
): void {
	const names = new Set(allowed);
	for (const key of Object.keys(value)) {
		if (!names.has(key)) throw new TypeError(`unexpected field: ${key}`);
	}
	if (!optional) {
		for (const key of allowed) {
			if (!(key in value)) throw new TypeError(`missing field: ${key}`);
		}
	}
}

function stringRecord(value: unknown, field: string): Record<string, string> {
	const source = object(value, field);
	const entries = Object.entries(source);
	if (entries.length > 64) throw new TypeError(`${field} has too many entries`);
	const result: Record<string, string> = {};
	for (const [key, item] of entries) {
		if (!REF_PATTERN.test(key))
			throw new TypeError(`${field} contains an invalid key`);
		result[key] = boundedString(item, `${field}.${key}`, 16 * 1024);
	}
	return result;
}
