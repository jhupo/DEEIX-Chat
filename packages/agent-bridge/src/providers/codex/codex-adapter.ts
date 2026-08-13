import type { InitializeParams } from "../../../generated/codex-app-server-v0.147.0/ts/InitializeParams.js";
import { isAbsolute } from "node:path";
import { createHmac, randomUUID } from "node:crypto";
import type { InitializeResponse } from "../../../generated/codex-app-server-v0.147.0/ts/InitializeResponse.js";
import type { GetAuthStatusResponse } from "../../../generated/codex-app-server-v0.147.0/ts/GetAuthStatusResponse.js";
import type { AppsListResponse } from "../../../generated/codex-app-server-v0.147.0/ts/v2/AppsListResponse.js";
import type { ListMcpServerStatusResponse } from "../../../generated/codex-app-server-v0.147.0/ts/v2/ListMcpServerStatusResponse.js";
import type { ModelListResponse } from "../../../generated/codex-app-server-v0.147.0/ts/v2/ModelListResponse.js";
import type { ModelProviderCapabilitiesReadResponse } from "../../../generated/codex-app-server-v0.147.0/ts/v2/ModelProviderCapabilitiesReadResponse.js";
import type { PermissionProfileListResponse } from "../../../generated/codex-app-server-v0.147.0/ts/v2/PermissionProfileListResponse.js";
import type { PluginListResponse } from "../../../generated/codex-app-server-v0.147.0/ts/v2/PluginListResponse.js";
import type { ReviewStartResponse } from "../../../generated/codex-app-server-v0.147.0/ts/v2/ReviewStartResponse.js";
import type { SkillsListResponse } from "../../../generated/codex-app-server-v0.147.0/ts/v2/SkillsListResponse.js";
import type { HooksListResponse } from "../../../generated/codex-app-server-v0.147.0/ts/v2/HooksListResponse.js";
import type { ThreadForkResponse } from "../../../generated/codex-app-server-v0.147.0/ts/v2/ThreadForkResponse.js";
import type { ThreadListResponse } from "../../../generated/codex-app-server-v0.147.0/ts/v2/ThreadListResponse.js";
import type { ThreadReadResponse } from "../../../generated/codex-app-server-v0.147.0/ts/v2/ThreadReadResponse.js";
import type { ThreadResumeResponse } from "../../../generated/codex-app-server-v0.147.0/ts/v2/ThreadResumeResponse.js";
import type { ThreadStartResponse } from "../../../generated/codex-app-server-v0.147.0/ts/v2/ThreadStartResponse.js";
import type { Turn } from "../../../generated/codex-app-server-v0.147.0/ts/v2/Turn.js";
import type { TurnStartResponse } from "../../../generated/codex-app-server-v0.147.0/ts/v2/TurnStartResponse.js";
import type { UserInput } from "../../../generated/codex-app-server-v0.147.0/ts/v2/UserInput.js";
import type {
	ProviderCommand,
	SourceRefRegistry,
} from "../../commands/resolve-provider-command.js";
import type { InteractionResponse } from "../../protocol/agent-command.js";
import type { ProviderInput } from "../../commands/resolve-provider-command.js";
import type {
	ProviderAdapter,
	ProviderEvent,
	ProviderManifest,
	ProviderResult,
} from "../provider-adapter.js";
import type {
	JsonLineRpcClient,
	RpcNotification,
	RpcServerRequest,
} from "./rpc-client.js";
import {
	CODEX_SERVER_NOTIFICATIONS,
	CODEX_SERVER_REQUESTS,
	codexMethodDisposition,
} from "./codex-method-policy.js";

const SCHEMA_HASH = "f72b2caa3cbfa4298de9e85c62dda6dfbaf2266ffeb916fed30615ca69ff8c74";
const COMMANDS: ProviderManifest["commands"] = [
	"thread.create",
	"thread.lifecycle",
	"thread.rename",
	"thread.metadata.update",
	"thread.compact",
	"review.start",
	"turn.start",
	"turn.steer",
	"turn.interrupt",
	"interaction.respond",
	"resource.refresh",
];

type PendingInteraction = {
	method: string;
	params: Record<string, unknown>;
	answerKeys: Map<string, string>;
	resolve: (response: unknown) => void;
	reject: (error: Error) => void;
};

export class CodexAdapter implements ProviderAdapter {
	readonly kind = "codex";
	readonly #profileId: string;
	readonly #rpc: JsonLineRpcClient;
	readonly #sources: SourceRefRegistry;
	readonly #runtimeVersion: string;
	readonly #closeProcess?: () => Promise<void>;
	readonly #pending = new Map<string, PendingInteraction>();
	#onEvent?: (event: ProviderEvent) => Promise<void>;
	#started = false;

	constructor(options: {
		profileId: string;
		rpc: JsonLineRpcClient;
		sources: SourceRefRegistry;
		runtimeVersion?: string;
		closeProcess?: () => Promise<void>;
	}) {
		this.#profileId = options.profileId;
		this.#rpc = options.rpc;
		this.#sources = options.sources;
		this.#runtimeVersion = options.runtimeVersion ?? "0.147.0";
		this.#closeProcess = options.closeProcess;
		this.#rpc.setHandlers({
			onNotification: (notification) => this.#notification(notification),
			onServerRequest: (request) => this.#serverRequest(request),
		});
	}

	async start(
		onEvent: (event: ProviderEvent) => Promise<void>,
		signal: AbortSignal,
	): Promise<ProviderManifest> {
		if (this.#started) throw new Error("Codex adapter is already started");
		this.#onEvent = onEvent;
		const params: InitializeParams = {
			clientInfo: {
				name: "deeix-agent-bridge",
				title: "DEEIX Agent Bridge",
				version: this.#runtimeVersion,
			},
			capabilities: {
				experimentalApi: false,
				requestAttestation: false,
				mcpServerOpenaiFormElicitation: true,
			},
		};
		await this.#rpc.request<InitializeResponse>("initialize", params, signal);
		await this.#rpc.notify("initialized");
		this.#started = true;
		return this.capabilities();
	}

	capabilities(): ProviderManifest {
		return {
			provider: this.kind,
			runtimeVersion: this.#runtimeVersion,
			protocolVersion: "0.147.0/stable",
			schemaHash: SCHEMA_HASH,
			commands: COMMANDS,
		};
	}

	async proveRuntimeAuth(challenge: string, signal: AbortSignal): Promise<string> {
		if (!this.#started) throw new Error("Codex adapter is not started");
		if (
			typeof challenge !== "string" || challenge.length > 1024 ||
			!(challenge.startsWith("deeix-runtime-auth-proof-v1\n") && challenge.split("\n").length === 7) &&
			!(challenge.startsWith("deeix-device-enrollment-v1\n") && challenge.split("\n").length === 6)
		) {
			throw new TypeError("runtime authentication challenge is invalid");
		}
		const auth = await this.#rpc.request<GetAuthStatusResponse>(
			"getAuthStatus",
			{ includeToken: true, refreshToken: false },
			signal,
		);
		if (auth.authMethod !== "apikey" || typeof auth.authToken !== "string" || auth.authToken.trim() === "")
			throw new Error("Codex must be signed in with an API key");
		return createHmac("sha256", auth.authToken).update(challenge, "utf8").digest("base64url");
	}

	async execute(
		command: ProviderCommand,
		signal: AbortSignal,
	): Promise<ProviderResult> {
		if (!this.#started) throw new Error("Codex adapter is not started");
		switch (command.kind) {
			case "thread.create": {
				const response = await this.#rpc.request<ThreadStartResponse>(
					"thread/start",
					{
						cwd: command.canonicalCwd,
						...threadSettings(command.settings),
						threadSource: "deeix-web",
					},
					signal,
				);
				return {
					kind: "thread-created",
					sourceThreadRef: await this.#sources.publish(
						this.#profileId,
						"thread",
						response.thread.id,
					),
				};
			}
			case "thread.lifecycle":
				return this.#threadLifecycle(command, signal);
			case "thread.rename":
				await this.#rpc.request("thread/name/set", {
					threadId: command.providerThreadId,
					name: command.name,
				}, signal);
				return { kind: "accepted" };
			case "thread.metadata.update":
				await this.#rpc.request("thread/metadata/update", {
					threadId: command.providerThreadId,
					gitInfo: command.gitInfo,
				}, signal);
				return { kind: "accepted" };
			case "thread.compact":
				await this.#rpc.request("thread/compact/start", {
					threadId: command.providerThreadId,
				}, signal);
				return { kind: "accepted" };
			case "review.start": {
				const response = await this.#rpc.request<ReviewStartResponse>(
					"review/start",
					{
						threadId: command.providerThreadId,
						target: reviewTarget(command.target),
						delivery: "inline",
					},
					signal,
				);
				return {
					kind: "turn-started",
					sourceTurnRef: await this.#sources.publish(
						this.#profileId,
						"turn",
						response.turn.id,
					),
				};
			}
			case "turn.start": {
				await this.#rpc.request<ThreadResumeResponse>(
					"thread/resume",
					{ threadId: command.providerThreadId, cwd: command.canonicalCwd },
					signal,
				);
				const response = await this.#rpc.request<TurnStartResponse>(
					"turn/start",
					{
						threadId: command.providerThreadId,
						input: userInput(command.input),
						cwd: command.canonicalCwd,
						...turnSettings(command.settings, command.canonicalCwd),
					},
					signal,
				);
				return {
					kind: "turn-started",
					sourceTurnRef: await this.#sources.publish(
						this.#profileId,
						"turn",
						response.turn.id,
					),
				};
			}
			case "turn.steer":
				await this.#rpc.request("turn/steer", {
					threadId: command.providerThreadId,
					expectedTurnId: command.providerTurnId,
					input: userInput(command.input),
				}, signal);
				return { kind: "accepted" };
			case "turn.interrupt":
				await this.#rpc.request("turn/interrupt", {
					threadId: command.providerThreadId,
					turnId: command.providerTurnId,
				}, signal);
				return { kind: "accepted" };
			case "interaction.respond":
				this.#respond(command.providerRequestId, command.response);
				return { kind: "accepted" };
			case "resource.refresh":
				return this.#resource(command, signal);
		}
	}

	async close(): Promise<void> {
		for (const pending of this.#pending.values())
			pending.reject(new Error("Codex adapter closed"));
		this.#pending.clear();
		this.#rpc.close();
		await this.#closeProcess?.();
	}

	async #threadLifecycle(
		command: Extract<ProviderCommand, { kind: "thread.lifecycle" }>,
		signal: AbortSignal,
	): Promise<ProviderResult> {
		if (command.action === "fork") {
			const response = await this.#rpc.request<ThreadForkResponse>(
				"thread/fork",
				{
					threadId: command.providerThreadId,
					cwd: command.canonicalCwd,
					threadSource: "deeix-web",
				},
				signal,
			);
			return {
				kind: "thread-forked",
				sourceThreadRef: await this.#sources.publish(
					this.#profileId,
					"thread",
					response.thread.id,
				),
			};
		}
		const method = {
			resume: "thread/resume",
			archive: "thread/archive",
			unarchive: "thread/unarchive",
			delete: "thread/delete",
		}[command.action];
		await this.#rpc.request(method, {
			threadId: command.providerThreadId,
			...(command.action === "resume" ? { cwd: command.canonicalCwd } : {}),
		}, signal);
		return { kind: "accepted" };
	}

	async #resource(
		command: Extract<ProviderCommand, { kind: "resource.refresh" }>,
		signal: AbortSignal,
	): Promise<ProviderResult> {
		const name = command.resource.name;
		const cwd = "canonicalCwd" in command ? command.canonicalCwd : undefined;
		let data: unknown;
		switch (name) {
			case "models":
				data = await this.#rpc.request<ModelListResponse>("model/list", {}, signal);
				break;
			case "model-capabilities":
				data = await this.#rpc.request<ModelProviderCapabilitiesReadResponse>(
					"modelProvider/capabilities/read",
					{},
					signal,
				);
				break;
			case "permission-profiles":
				data = await this.#rpc.request<PermissionProfileListResponse>(
					"permissionProfile/list",
					{},
					signal,
				);
				break;
			case "apps":
				data = await this.#rpc.request<AppsListResponse>("app/list", {}, signal);
				break;
			case "mcp":
				data = await this.#rpc.request<ListMcpServerStatusResponse>(
					"mcpServerStatus/list",
					{ detail: "full" },
					signal,
				);
				break;
			case "plugins":
				data = await this.#rpc.request<PluginListResponse>(
					"plugin/list",
					{ forceRefetch: true },
					signal,
				);
				break;
			case "auth-status": {
				const auth = await this.#rpc.request<GetAuthStatusResponse>(
					"getAuthStatus",
					{ includeToken: false, refreshToken: false },
					signal,
				);
				data = {
					authMethod: auth.authMethod,
					requiresOpenaiAuth: auth.requiresOpenaiAuth,
				};
				break;
			}
			case "sessions": {
				if (!cwd) throw new TypeError("workspace resource requires cwd");
				const threads = await this.#rpc.request<ThreadListResponse>(
					"thread/list",
					{ cwd, limit: 30, archived: false },
					signal,
				);
				data = {
					data: await Promise.all(threads.data.map(async (thread) => {
						let turns: Turn[] = [];
						try {
							const detail = await this.#rpc.request<ThreadReadResponse>(
								"thread/read",
								{ threadId: thread.id, includeTurns: true },
								signal,
							);
							turns = detail.thread.turns;
						} catch {
							if (signal.aborted) throw signal.reason;
						}
						return {
							sourceThreadRef: await this.#sources.publish(this.#profileId, "thread", thread.id),
							preview: thread.preview,
							name: thread.name,
							modelProvider: thread.modelProvider,
							createdAt: thread.createdAt,
							updatedAt: thread.updatedAt,
							recencyAt: thread.recencyAt,
							status: "active",
							messages: projectSessionMessages(turns),
						};
					})),
				};
				break;
			}
			case "skills":
				if (!cwd) throw new TypeError("workspace resource requires cwd");
				data = await this.#rpc.request<SkillsListResponse>(
					"skills/list",
					{ cwds: [cwd], forceReload: true },
					signal,
				);
				break;
			case "hooks":
				if (!cwd) throw new TypeError("workspace resource requires cwd");
				data = await this.#rpc.request<HooksListResponse>(
					"hooks/list",
					{ cwds: [cwd] },
					signal,
				);
				break;
		}
		return {
			kind: "resource",
			resource: name,
			data: name === "sessions" ? data : projectResourceData(data),
		};
	}

	async #notification(notification: RpcNotification): Promise<void> {
		if (!this.#onEvent) return;
		const disposition = codexMethodDisposition(
			CODEX_SERVER_NOTIFICATIONS,
			notification.method,
		);
		if (disposition === "disabled") return;
		const params = record(notification.params);
		const threadId = identity(params, "threadId", "thread");
		const turnId = identity(params, "turnId", "turn");
		const itemId = identity(params, "itemId", "item");
		const requestId = stringOrNumber(params.requestId);
		const sourceThreadRef = threadId
			? await this.#sources.publish(this.#profileId, "thread", threadId)
			: undefined;
		const sourceTurnRef = turnId
			? await this.#sources.publish(this.#profileId, "turn", turnId)
			: undefined;
		const sourceItemRef = itemId
			? await this.#sources.publish(this.#profileId, "item", itemId)
			: undefined;
		const sourceRequestRef = requestId
			? await this.#sources.publish(this.#profileId, "request", rpcId(requestId))
			: undefined;
		await this.#onEvent({
			kind: disposition === "mapped" ? notification.method : "provider.extension",
			...(sourceThreadRef ? { sourceThreadRef } : {}),
			...(sourceTurnRef ? { sourceTurnRef } : {}),
			...(sourceItemRef ? { sourceItemRef } : {}),
			...(sourceRequestRef ? { sourceRequestRef } : {}),
			occurredAt: new Date().toISOString(),
			payload: disposition === "mapped"
				? sanitizeRecord(params)
				: { method: notification.method, data: sanitizeRecord(params) },
		});
	}

	async #serverRequest(request: RpcServerRequest): Promise<unknown> {
		if (!this.#onEvent) throw new Error("Codex adapter has no event consumer");
		if (
			codexMethodDisposition(CODEX_SERVER_REQUESTS, request.method) !== "mapped"
		)
			throw new Error(`Codex server request is disabled: ${request.method}`);
		const providerRequestId = rpcId(request.id);
		const sourceRequestRef = await this.#sources.publish(
			this.#profileId,
			"request",
			providerRequestId,
		);
		const params = record(request.params);
		const projected = projectServerRequest(request.method, params);
		const threadId = identity(params, "threadId", "thread");
		const turnId = identity(params, "turnId", "turn");
		const sourceThreadRef = threadId
			? await this.#sources.publish(this.#profileId, "thread", threadId)
			: undefined;
		const sourceTurnRef = turnId
			? await this.#sources.publish(this.#profileId, "turn", turnId)
			: undefined;
		const result = new Promise<unknown>((resolve, reject) => {
			this.#pending.set(providerRequestId, {
				method: request.method,
				params,
				answerKeys: projected.answerKeys,
				resolve,
				reject,
			});
		});
		try {
			await this.#onEvent({
				kind: "interaction.requested",
				...(sourceThreadRef ? { sourceThreadRef } : {}),
				...(sourceTurnRef ? { sourceTurnRef } : {}),
				sourceRequestRef,
				occurredAt: new Date().toISOString(),
				payload: { method: request.method, request: projected.request },
			});
		} catch (error) {
			this.#pending.delete(providerRequestId);
			throw error;
		}
		return result;
	}

	#respond(providerRequestId: string, response: InteractionResponse): void {
		const pending = this.#pending.get(providerRequestId);
		if (!pending) throw new Error("Codex interaction is no longer pending");
		const mapped = interactionResponse(pending, response);
		this.#pending.delete(providerRequestId);
		pending.resolve(mapped);
	}
}

type ProjectedSessionMessage = {
	role: "user" | "assistant";
	content: string;
	reasoningContent?: string;
	createdAt?: number;
};

function projectSessionMessages(turns: Turn[]): ProjectedSessionMessage[] {
	const messages: ProjectedSessionMessage[] = [];
	for (const turn of turns) {
		let reasoning = "";
		for (const item of turn.items) {
			if (item.type === "userMessage") {
				const content = item.content
					.filter((input) => input.type === "text")
					.map((input) => input.text.trim())
					.filter(Boolean)
					.join("\n");
				if (content) messages.push({ role: "user", content, createdAt: turn.startedAt ?? undefined });
				continue;
			}
			if (item.type === "reasoning") {
				reasoning = [...item.summary, ...item.content].map((part) => part.trim()).filter(Boolean).join("\n");
				continue;
			}
			if (item.type === "agentMessage" && item.text.trim()) {
				messages.push({
					role: "assistant",
					content: item.text.trim(),
					...(reasoning ? { reasoningContent: reasoning } : {}),
					createdAt: turn.completedAt ?? turn.startedAt ?? undefined,
				});
				reasoning = "";
			}
		}
	}

	const selected: ProjectedSessionMessage[] = [];
	let size = 0;
	for (let index = messages.length - 1; index >= 0; index -= 1) {
		const message = messages[index]!;
		const nextSize = Buffer.byteLength(JSON.stringify(message), "utf8");
		if (selected.length > 0 && size + nextSize > 32 * 1024) break;
		selected.unshift(message);
		size += nextSize;
	}
	return selected;
}

function threadSettings(settings: ProviderCommand extends never ? never : {
	model?: string;
	reasoningEffort?: string;
	approvalPolicy?: "untrusted" | "on-request" | "never";
	sandboxPolicy?: "read-only" | "workspace-write";
}): Record<string, unknown> {
	return {
		...(settings.model ? { model: settings.model } : {}),
		...(settings.approvalPolicy ? { approvalPolicy: settings.approvalPolicy } : {}),
		...(settings.sandboxPolicy ? { sandbox: settings.sandboxPolicy } : {}),
	};
}

function turnSettings(
	settings: Parameters<typeof threadSettings>[0],
	cwd: string,
): Record<string, unknown> {
	return {
		...(settings.model ? { model: settings.model } : {}),
		...(settings.reasoningEffort ? { effort: settings.reasoningEffort } : {}),
		...(settings.approvalPolicy ? { approvalPolicy: settings.approvalPolicy } : {}),
		...(settings.sandboxPolicy === "read-only"
			? { sandboxPolicy: { type: "readOnly", networkAccess: false } }
			: settings.sandboxPolicy === "workspace-write"
				? {
						sandboxPolicy: {
							type: "workspaceWrite",
							writableRoots: [cwd],
							networkAccess: false,
							excludeTmpdirEnvVar: false,
							excludeSlashTmp: false,
						},
					}
				: {}),
	};
}

function userInput(input: ProviderInput[]): UserInput[] {
	return input.map((item) => {
		if (item.kind === "text")
			return { type: "text", text: item.text, text_elements: [] };
		return item.kind === "local-image"
			? { type: "localImage", path: item.path }
			: { type: "localAudio", path: item.path };
	});
}

function reviewTarget(
	target: Extract<ProviderCommand, { kind: "review.start" }>["target"],
): Record<string, unknown> {
	if (target.kind === "working-tree") return { type: "uncommittedChanges" };
	if (target.kind === "base-branch")
		return { type: "baseBranch", branch: target.branch };
	return { type: "commit", sha: target.sha, title: null };
}

function interactionResponse(
	pending: Pick<PendingInteraction, "method" | "params" | "answerKeys">,
	response: InteractionResponse,
): unknown {
	const { method, params } = pending;
	if (
		method === "item/commandExecution/requestApproval" ||
		method === "item/fileChange/requestApproval"
	) {
		if (response.kind !== "approval") throw new TypeError("approval response required");
		return { decision: response.decision };
	}
	if (method === "item/tool/requestUserInput") {
		if (response.kind !== "user-input") throw new TypeError("user-input response required");
		return {
			answers: Object.fromEntries(
				Object.entries(response.answers).map(([key, value]) => {
					const providerKey = pending.answerKeys.get(key);
					if (!providerKey) throw new TypeError(`unknown questionRef: ${key}`);
					return [providerKey, { answers: [value] }];
				}),
			),
		};
	}
	if (method === "mcpServer/elicitation/request") {
		if (response.kind !== "mcp-elicitation")
			throw new TypeError("mcp-elicitation response required");
		return {
			action: response.decision,
			content: response.decision === "accept" ? (response.content ?? {}) : null,
			_meta: null,
		};
	}
	if (method === "item/permissions/requestApproval") {
		if (response.kind !== "permission") throw new TypeError("permission response required");
		if (response.decision === "decline") return { permissions: {}, scope: "turn" };
		const requested = record(params.permissions);
		return {
			permissions: {
				...(requested.network == null ? {} : { network: requested.network }),
				...(requested.fileSystem == null ? {} : { fileSystem: requested.fileSystem }),
			},
			scope: response.scope ?? "turn",
		};
	}
	if (method === "item/tool/call") {
		if (response.kind !== "dynamic-tool")
			throw new TypeError("dynamic-tool response required");
		return {
			success: response.success,
			contentItems: response.content.map((item) =>
				item.kind === "text"
					? { type: "inputText", text: item.text }
					: item.kind === "image"
						? { type: "inputImage", imageUrl: item.url }
						: { type: "inputAudio", audioUrl: item.url },
			),
		};
	}
	throw new Error(`Codex server request is disabled: ${method}`);
}

function projectServerRequest(
	method: string,
	params: Record<string, unknown>,
): { request: Record<string, unknown>; answerKeys: Map<string, string> } {
	const answerKeys = new Map<string, string>();
	const request = sanitizeRecord(params);
	if (method !== "item/tool/requestUserInput") return { request, answerKeys };
	const questions = Array.isArray(params.questions) ? params.questions : [];
	request.questions = questions.map((value) => {
		const question = record(value);
		if (typeof question.id !== "string" || question.id.length === 0)
			throw new TypeError("Codex user-input question id is invalid");
		const questionRef = `question_${randomUUID()}`;
		answerKeys.set(questionRef, question.id);
		return { ...sanitizeRecord(question), questionRef };
	});
	return { request, answerKeys };
}

function identity(
	params: Record<string, unknown>,
	direct: string,
	nested: string,
): string | undefined {
	if (typeof params[direct] === "string") return params[direct];
	const value = record(params[nested]);
	return typeof value.id === "string" ? value.id : undefined;
}

function rpcId(value: string | number): string {
	return `${typeof value === "number" ? "n" : "s"}:${value}`;
}

function stringOrNumber(value: unknown): string | number | undefined {
	return typeof value === "string" || typeof value === "number" ? value : undefined;
}

function record(value: unknown): Record<string, unknown> {
	return typeof value === "object" && value !== null && !Array.isArray(value)
		? (value as Record<string, unknown>)
		: {};
}

function sanitize(value: unknown, key = ""): unknown {
	if (/token|secret|authorization/i.test(key)) return undefined;
	if (
		typeof value === "string" &&
		(/^(cwd|codexHome|instructionSources?|writableRoots?)$/i.test(key) ||
			containsLocalPath(value))
	)
		return undefined;
	if (Array.isArray(value)) return value.map((item) => sanitize(item));
	if (typeof value !== "object" || value === null) return value;
	const source = value as Record<string, unknown>;
	const identityObject =
		("sessionId" in source && "cwd" in source) ||
		("items" in source && "status" in source) ||
		("type" in source && "id" in source);
	return Object.fromEntries(
		Object.entries(source).flatMap(([name, item]) => {
			if (
				name === "id" ||
				/Id$/.test(name) ||
				/token|secret|authorization/i.test(name)
			)
				return [];
			const sanitized = sanitize(item, name);
			return sanitized === undefined ? [] : [[name, sanitized]];
		}),
	);
}

function sanitizeRecord(value: Record<string, unknown>): Record<string, unknown> {
	return sanitize(value) as Record<string, unknown>;
}

function projectResourceData(value: unknown, key = ""): unknown {
	if (
		/token|secret|authorization|password|credential/i.test(key) ||
		/^(cwd|path|codexHome|home|command|args|env|environment|instructionSources?|writableRoots?)$/i.test(key)
	)
		return undefined;
	if (typeof value === "string" && containsLocalPath(value)) return undefined;
	if (Array.isArray(value))
		return value.flatMap((item) => {
			const projected = projectResourceData(item);
			return projected === undefined ? [] : [projected];
		});
	if (typeof value !== "object" || value === null) return value;
	return Object.fromEntries(
		Object.entries(value as Record<string, unknown>).flatMap(([name, item]) => {
			const projected = projectResourceData(item, name);
			return projected === undefined ? [] : [[name, projected]];
		}),
	);
}

function containsLocalPath(value: string): boolean {
	return (
		isAbsolute(value) ||
		/^file:/i.test(value) ||
		/(?:^|\s)(?:[A-Za-z]:[\\/]|\\\\|\/[A-Za-z0-9._-]+[\\/])/.test(value)
	);
}
