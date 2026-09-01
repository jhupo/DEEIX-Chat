# Codex app-server 接入

> 当前实现与实测状态以 [10-app-server-validation-and-gap-plan.md](./10-app-server-validation-and-gap-plan.md) 为准。本文件保留锁定 schema、方法边界和设计背景；表格未标为“当前实现”的内容属于后续设计。

> 本文的 method matrix 与 schema pin 仍是 Bridge 实现依据；Web/API 拓扑以 [统一会话架构](./01-architecture.md) 和 [当前协议](./03-protocol-and-data-model.md) 为准。

## 1. 接入规则与 schema pin

The native `deeix-agent` launches the user-installed `codex app-server` with stdio JSONL. The provider protocol is JSON-RPC 2.0 without a header; each line is one object. WebSocket and Unix transports exist upstream, but the Agent uses stdio because it binds process lifecycle locally and keeps app-server off the network. Upstream WebSocket transport is not the DEEIX transport.

The production adapter is pinned to official OpenAI Codex `rust-v0.151.0`, commit `78c290807ce710180111df227df3b7a4fe845452` (2026-08-29). Its reproducible authority is [codex-app-server-v0.151.0.lock.json](./codex-app-server-v0.151.0.lock.json): stable/non-experimental `codex app-server generate-ts --out ...` and `generate-json-schema --out ...`, release asset digest, generated artifact hashes, exhaustive union members and dispositions.

The runtime accepts official Codex CLI versions in the reviewed `0.151.x` line, starting at `0.151.0`. The Agent always advertises the reviewed `0.151.0/stable` protocol and exact locked schema hash; Cloud rejects runtimes outside the supported range and any other protocol/hash pair. Startup also probes `thread/list` with `sortKey: "recency_at"` before enrollment.

Adapter startup follows this procedure:

1. Keep the stable lock as the reviewed protocol evidence and update the native method registry with it.
2. Run the native Agent tests and build all three target executables.
3. Start app-server, send `initialize`, verify runtime output, then send `initialized`.
4. Publish manifest `{ runtimeVersion, protocolVersion, schemaHash, capabilities }`; the server validates every advertised capability.

The lock file and its exact generated schema are implementation authority. The tables below are a human capability summary, not an exhaustive registry. A name outside this stable lock is not advertised or dispatched. Main-branch/experimental drift stays disabled until a separate `--experimental` lock has generated artifacts, hashes, exhaustive dispositions and review.

### JSONL frame and process lifecycle

The local RPC reader is line-oriented but bounded: incoming app-server lines are capped at 64 MiB and outgoing requests at 4 MiB. Thread history is read through bounded turn and item pages, while the larger incoming limit still accommodates provider resource and lifecycle responses. The reader rejects a line that exceeds the cap without allocating an unbounded buffer. A frame overflow, malformed JSON, EOF, or a dead stdio pipe closes the RPC client and terminates the associated app-server process. The Agent runtime supervisor then cancels the old runtime scope and restarts a fresh app-server with jittered backoff; it does not keep a dead RPC process behind a reconnecting WSS socket.

The WSS bridge keeps the same 64 MiB bounded frame budget for thread history terminal results. History projection does not take a tail window: it preserves all user, assistant, and reasoning messages in the app-server snapshot, subject to the explicit 4096-message/48 MiB server-side validation budget. The Web API still reads messages in pages, so large conversations remain bounded in the browser without losing older history at import time. Existing projections from an earlier history shape are rehydrated once through the history projection version marker.

The authenticated WSS heartbeat atomically renews the runtime lease and presence on the existing connection. A healthy Agent does not reconnect on a timer; reconnects are reserved for heartbeat timeout, network failure, service restart, device revocation, or validation failure. Workspace discovery is initially served from the persisted device configuration, then refreshed through `thread/list` and sent as an authenticated `workspaces.sync` frame. This keeps project discovery and history reads on the app-server contract while allowing a newly registered local folder to appear without reinstalling the Agent.

Official references: [app-server documentation](https://developers.openai.com/codex/app-server/), [app-server source](https://github.com/openai/codex/tree/main/codex-rs/app-server), and [Codex repository](https://github.com/openai/codex).

## 2. Local adapter boundary

下面的类型片段是边界设计草图，不是编译后的当前合同。当前合同直接查看 `backend/internal/agentclient/protocol.go`、`codex.go` 和 `gateway.go`。

```ts
import type { ThreadMetadataUpdateParams as CodexThreadMetadataUpdateParams } from "./generated/codex-app-server-v0.151.0/v2";

type AgentResource =
  | { scope: "profile"; resource: ProfileResource }
  | { scope: "workspace"; resource: WorkspaceResource };

type ProfileResourceCommand<K extends "resource.refresh" | "resource.mutate"> =
  { kind: K; deviceId: string; profileId: string; resource: Extract<AgentResource, { scope: "profile" }> };

type WorkspaceResourceCommand<K extends "resource.refresh" | "resource.mutate"> =
  { kind: K; deviceId: string; profileId: string; workspaceId: string; resource: Extract<AgentResource, { scope: "workspace" }> };

type TurnInteractionCommand = {
  kind: "interaction.respond";
  scope: "turn";
  deviceId: string;
  profileId: string;
  workspaceId: string;
  threadId: string;
  sourceThreadRef: string;
  turnId: string;
  sourceTurnRef: string;
  interactionId: string;
  sourceRequestRef: string;
  response: InteractionResponse;
};

type ThreadInteractionCommand = {
  kind: "interaction.respond";
  scope: "thread";
  deviceId: string;
  profileId: string;
  workspaceId: string;
  threadId: string;
  sourceThreadRef: string;
  interactionId: string;
  sourceRequestRef: string;
  response: InteractionResponse;
};

type ThreadMetadataPatch = Pick<
  CodexThreadMetadataUpdateParams,
  "gitInfo"
>;

export type AgentCommand =
  | { kind: "thread.create"; deviceId: string; profileId: string; workspaceId: string; settings: ThreadStartSettings }
  | { kind: "thread.lifecycle"; deviceId: string; profileId: string; workspaceId: string; threadId: string; sourceThreadRef: string; action: "resume" | "fork" | "archive" | "unarchive" | "delete" }
  | { kind: "thread.rename"; deviceId: string; profileId: string; workspaceId: string; threadId: string; sourceThreadRef: string; name: string }
  | { kind: "thread.metadata.update"; deviceId: string; profileId: string; workspaceId: string; threadId: string; sourceThreadRef: string; patch: ThreadMetadataPatch }
  | { kind: "thread.compact"; deviceId: string; profileId: string; workspaceId: string; threadId: string; sourceThreadRef: string }
  | { kind: "thread.read"; deviceId: string; profileId: string; workspaceId: string; threadId: string; sourceThreadRef: string }
  | { kind: "review.start"; deviceId: string; profileId: string; workspaceId: string; threadId: string; sourceThreadRef: string; target: ReviewTarget }
  | { kind: "turn.start"; deviceId: string; profileId: string; workspaceId: string; threadId: string; sourceThreadRef: string; input: AgentInput[]; settings: TurnSettings }
  | { kind: "turn.steer"; deviceId: string; profileId: string; workspaceId: string; threadId: string; sourceThreadRef: string; turnId: string; sourceTurnRef: string; input: AgentInput[] }
  | { kind: "turn.interrupt"; deviceId: string; profileId: string; workspaceId: string; threadId: string; sourceThreadRef: string; turnId: string; sourceTurnRef: string }
  | TurnInteractionCommand
  | ThreadInteractionCommand
  | ProfileResourceCommand<"resource.refresh">
  | WorkspaceResourceCommand<"resource.refresh">
  | (ProfileResourceCommand<"resource.mutate"> & { mutation: ResourceMutation })
  | (WorkspaceResourceCommand<"resource.mutate"> & { mutation: ResourceMutation });

type ProviderKind = ProviderRegistry["kind"];
type ProviderAgentCommand = AgentCommand;
type CloudRefs = "deviceId" | "profileId" | "workspaceId" | "threadId" | "turnId" | "interactionId" | "sourceThreadRef" | "sourceTurnRef" | "sourceRequestRef";

type ResolvedFields = {
  "thread.create": { canonicalCwd: string };
  "thread.lifecycle": { providerThreadId: string; canonicalCwd: string };
  "thread.rename": { providerThreadId: string; canonicalCwd: string };
  "thread.metadata.update": { providerThreadId: string; canonicalCwd: string };
  "thread.compact": { providerThreadId: string; canonicalCwd: string };
  "review.start": { providerThreadId: string; canonicalCwd: string };
  "turn.start": { providerThreadId: string; canonicalCwd: string };
  "turn.steer": { providerThreadId: string; providerTurnId: string; canonicalCwd: string };
  "turn.interrupt": { providerThreadId: string; providerTurnId: string; canonicalCwd: string };
  "interaction.respond": { providerThreadId: string; providerRequestId: string; canonicalCwd: string };
  "resource.refresh": {};
  "resource.mutate": {};
};

type WorkspaceResolvedFields<C extends ProviderAgentCommand> =
  C extends { resource: { scope: "workspace" } } ? { canonicalCwd: string } : { canonicalCwd?: never };

type InteractionResolvedFields<C extends ProviderAgentCommand> =
  C extends { kind: "interaction.respond"; scope: "turn" }
    ? { providerTurnId: string }
    : C extends { kind: "interaction.respond"; scope: "thread" }
      ? { providerTurnId?: never }
      : {};

type ResolvedCommandMember<C extends ProviderAgentCommand> =
  C extends ProviderAgentCommand
    ? Omit<C, CloudRefs | "kind"> &
        ResolvedFields[C["kind"]] &
        WorkspaceResolvedFields<C> & {
          kind: C["kind"];
          commandId: string;
          provider: ProviderKind;
          profileRef: string;
        }
        & InteractionResolvedFields<C>
    : never;

type ResolvedCommand<K extends ProviderAgentCommand["kind"]> =
  ResolvedCommandMember<Extract<AgentCommand, { kind: K }>>;

export type ProviderCommand = {
  [K in ProviderAgentCommand["kind"]]: ResolvedCommand<K>;
}[ProviderAgentCommand["kind"]];

export interface ProviderAdapter {
  readonly kind: string;
  start(onEvent: (event: ProviderEvent) => Promise<void>, signal: AbortSignal): Promise<ProviderManifest>;
  execute(command: ProviderCommand, signal: AbortSignal): Promise<ProviderResult>;
  capabilities(): ProviderManifest;
  close(): Promise<void>;
}
```

Cloud `AgentCommand` has opaque public/source refs and allowlisted discriminants only. `thread.create` has no user input and no `sourceThreadRef` until its Bridge result/event binds it; Browser initial input appears solely in the follow-on `turn.start` command. Thread lifecycle/name/compact/review and turn start carry `sourceThreadRef`; steer/interrupt also carry `sourceTurnRef`; interaction response carries `sourceRequestRef`. Commands contain no canonical cwd, raw provider ID, raw RPC method, credential or secret. An attachment is an opaque `artifactRef`; Cloud validates its User/workspace binding, the WSS envelope adds a short-lived command-bound grant, and Bridge maps the downloaded image/audio to `localImage`/`localAudio`. Skill and App inputs carry only `resourceRef`; Bridge resolves its local mapping to a skill path or `app://id`. Adapter receives a `kind`-narrowed union with no `unknown` payload. Bridge de-duplicates by command ID/result cache.

`thread/section/move` and `threadSection/*` remain separately locked extensions with no first-release control.

当前 `ProviderManifest` 是 Bridge 与 Cloud 共用的唯一能力声明，包含 provider、runtime/protocol version、schema hash、command kinds、profile/workspace resources、input kinds、thread settings 取值以及六类 interaction kinds。Bridge 在 runtime proof 帧中提交该结构；Cloud 严格校验后保存到 `AgentRuntimeProfile.manifest_json`，并通过 profile API 返回同一快照。Bridge 执行命令前仍使用同一 manifest 重新校验 command kind。

```json
{
  "provider": "codex",
  "runtimeVersion": "0.151.0",
  "protocolVersion": "0.151.0/stable",
  "schemaHash": "<64-char sha256>",
  "commands": ["thread.create", "turn.start", "interaction.respond"],
  "resources": { "profile": ["models"], "workspace": ["sessions"] },
  "inputKinds": ["text", "artifact", "skill", "app-mention"],
  "threadSettings": {
    "model": true,
    "reasoningEffort": ["low", "medium", "high", "xhigh"],
    "approvalPolicy": ["untrusted", "on-request", "never"],
    "sandboxPolicy": ["read-only", "workspace-write"]
  },
  "interactionKinds": ["command_approval", "file_approval", "user_input", "permission", "mcp_elicitation", "dynamic_tool"]
}
```

## 3. Method matrix

| Group | Codex methods / notifications | Stability and current-schema rule | Bridge mapping | Web behavior and terminal semantics |
| --- | --- | --- | --- | --- |
| Initialize/lifecycle | `initialize`, `initialized`, runtime notices/errors | Mandatory gate; generated initialize result determines manifest | Spawn stdio process, initialize, persist profile snapshot and lifecycle frames | Profile diagnostics show startup/auth/schema state; command dispatch starts after ready |
| Thread catalog | `thread/list`, `thread/read`, `thread/start`, `thread/resume`, `thread/fork`, `thread/name/set`, `thread/archive`, `thread/unarchive`, `thread/delete` | Stable only when generated type contains the method | `PATCH /threads/:thread_id/name` idempotently queues typed `thread.rename`; Bridge resolves raw ID/cwd and calls `thread/name/set` | Thread list, open, create, resume, fork, rename, archive, delete; provider result/event projects title |
| Thread metadata | `thread/metadata/update` with pinned-schema `gitInfo` only | Mapped stable method; generated schema defines the `ThreadMetadataPatch` allowlist: optional nullable `sha`, `branch`, `originUrl` | `PATCH /threads/:thread_id/provider-metadata` queues typed `thread.metadata.update`, then resolves raw thread ID/cwd | Git projection changes after provider result/event; pin, labels and share remain cloud metadata |
| Thread goal | `thread/goal/set`, `thread/goal/get`, `thread/goal/clear` are stable-lock extensions; `thread/goal/updated` and `thread/goal/cleared` notifications are mapped | First production exposes no goal read/mutation request control and creates no AgentCommand or Agent API dispatch for these extension requests | Project mapped goal notifications as redacted read-only diagnostics | Read-only diagnostic panel reflects `thread/goal/updated`/`thread/goal/cleared`; DEEIX labels remain cloud metadata. A later promotion changes lock disposition, adds typed command/API mapper and fixtures, and passes generated-lock CI together |
| Turn | `turn/start`, `turn/steer`, `turn/interrupt`, `thread/compact/start`, `review/start` | Stable/beta follows generated method metadata | Map to AgentTurn and local request correlation | Composer supports start/steer/interrupt; compact/review appear only when capability exists; `turn/completed` is turn terminal |
| Plan/diff/usage | `turn/plan/updated`, `turn/diff/updated`, usage notifications | Use current generated notification union | Normalize plan/diff/usage projection | Plan/diff panels update incrementally; their item lists can be empty and never replace item journal |
| Item/notifications | Stable item notifications include started/delta/completed and `item/fileChange/patchUpdated` (mapped); `item/commandExecution/terminalInteraction` is an extension notification | Lock disposition and generated notification type are authoritative | Upsert mapped AgentItem projections; retain extension payload redacted | Completed supplies item terminal form; extension remains inspectable without promoting a control |
| Server requests | Stable request union includes command/file/permission approvals, `item/tool/requestUserInput`, MCP elicitation and typed tool calls; `attestation/generate` is disabled | Generated request/response types and maturity metadata in the lock are authoritative | Store local request ID/source request ref in Bridge WAL, project AgentInteraction, and map typed response through `interaction.respond` | Drawer presents mapped typed response; disabled request has no control; browser never sees raw RPC ID |
| Models/modes | Stable `model/list`, `modelProvider/capabilities/read`, `permissionProfile/list`; unpinned experimental candidate `collaborationMode/list` | Stable entries follow the lock. `collaborationMode/list` is disabled until a separate generated, hashed and exhaustive experimental lock exists | Snapshot profile resources and capability descriptors | Model and permission selectors are disabled while stale/offline; collaboration has no control before its experimental lock |
| Skills/Hooks | `skills/list`, `skills/changed`, `hooks/list` | 当前实现只读 list 与显式 Skill input；extra roots/config write 为 extension | Query using canonical cwd; publish opaque resource ref，路径留在 Bridge | `/` 选择 Skill；启停尚未接入 |
| Plugins/marketplaces | `plugin/list` | 当前只调度 pinned `plugin/list`；官方仍标记 under development | 投影只读列表 | read/install/uninstall/share 尚未接入 |
| Apps | 分页 `app/list` | 当前调度 list 与显式 mention input | Profile resource snapshot 只含 opaque ref，App ID 留在 Bridge | `@` 选择 App mention；`app/installed`、`app/read` 尚未接入 |
| MCP | `mcpServerStatus/list` 与 startup notification | 当前只调度 status/list | 投影脱敏状态、工具与资源目录 | resource read、tool call、OAuth 和 reload 尚未接入 |
| Config | `config/read`、writes、requirements | 当前 Agent 协议未暴露；真实进程只探测了 `config/read` | 无 Cloud config snapshot | 后续只增加脱敏 summary，secret 与本机 path 留在 Bridge |
| Filesystem/command/process | fs、`command/exec`、process candidates | 当前 Agent 协议未暴露；真实进程只探测了 metadata 和隔离 command | 文件附件走 artifact grant | 集成终端需单独的 workspace-relative command 设计 |
| Account/auth | `account/read` | 使用官方 account API 投影 auth mode/status；API-key 模式下由 Agent 以配置用户身份只读 `codexHome/auth.json`，原始 key 仅在本机生成 HMAC proof | Bridge 只发送 proof 与脱敏诊断，不发送原始 key 或 auth 文件 | 订阅余额继续由 Sub2API 提供；Codex account 只作 runtime 诊断 |
| External config/environment | Stable-lock external config members; `environment/info` is an unpinned experimental candidate | Lock disposition governs. `environment/info` is disabled until a separate generated, hashed and exhaustive experimental lock exists | Diagnostic/resource events with field allowlist | Setup diagnostics and guided import status; no environment secret export |
| Preview extensions | Stable `experimentalFeature/list`, `experimentalFeature/enablement/set`, `windowsSandbox/setupStart` and `feedback/upload` members | Pinned stable members with disabled disposition; they have no dispatch until promoted with a typed mapper and fixtures | Dedicated typed extension mapper, audit each mutation | No control while disabled; failure does not affect normal thread execution |

### 3.1 Stable lock coverage

[codex-app-server-v0.151.0.lock.json](./codex-app-server-v0.151.0.lock.json) is the exhaustive authority for the four generated stable unions. A method enters execution only when the lock, manifest and local policy all declare it.

`thread/section/move` and `threadSection/*` remain separately locked extensions with no first-release control.

| Union | Members | Dispositions | Labeled examples |
| --- | ---: | --- | --- |
| `ClientRequest` | 98 | mapped 26, extension 53, disabled 19 | mapped 与实际 dispatch registry 严格相等 |
| `ClientNotification` | 1 | mapped 1 | `initialized` mapped |
| `ServerRequest` | 10 | mapped 6, extension 1, disabled 3 | `attestation/generate` disabled |
| `ServerNotification` | 79 | mapped 41, extension 23, disabled 15 | `item/fileChange/patchUpdated` mapped; `item/commandExecution/terminalInteraction` extension |

`mapped` ClientRequest 必须同时出现在实际 dispatch registry 并具有验收；`extension` 保留在锁中但没有 AgentCommand；`disabled` 没有 dispatch。ServerRequest 与 notification 的 disposition 由完整 policy object 校验。

`collaborationMode/list`, `environment/info`, process client requests, backgroundTerminal client requests, `currentTime/read`, and `tool/requestUserInput` are unpinned experimental candidates. They are disabled until a separate generated, hashed and exhaustive experimental lock exists; they do not follow or extend this stable `v0.151.0` schema. `thread/turns/list` is mapped for bounded history hydration with `itemsView: "full"`. Although the generated stable union contains `thread/items/list`, the released `0.151.0` runtime returns JSON-RPC `-32601`, so the audited lock disables it.

## 4. Native actions and Conversation ownership

Native Codex thread actions只在 pinned schema 存在等价 method 时由 Bridge Adapter 实现。底层能力不直接形成 Web Thread API；产品动作必须先进入 Conversation 用例，再由强类型 Gateway command 调用对应 method。`AgentThread` 只保存 provider source ref、运行状态和必要投影，标题、置顶、标签、分享与项目归属都由 Conversation 持有。

`thread/section/move` and `threadSection/*` remain separately locked extensions with no first-release control.

Conversation-owned metadata includes title, pin, labels, share policy, project association and product audit references; it不进入 provider command。Device/Profile/Workspace 是 Conversation 的 immutable execution binding。

At initialize, the initial stable profile does not set `experimentalApi=true`; it advertises only implemented stable capabilities with a generated-schema mapper and acceptance fixture, such as `optOutNotificationMethods` and `mcpServerOpenaiFormElicitation` when implemented. `experimentalApi` may be enabled only after a separate experimental generated, hashed and exhaustive lock and its fixtures exist. `requestAttestation` remains absent from the advertised manifest until its disabled `attestation/generate` server-request handler is promoted with policy and fixtures. The manifest ties each declared capability to the exact generated schema version and local policy before Web exposes a control.

## 5. Mapper and terminal rules

1. Method registry covers every notification in the pinned generated union.
2. Typed mapper handles lifecycle, thread, turn, item, interaction, plan, diff, usage, artifact and error records.
3. Registry routes a recognized-but-not-yet-promoted notification to redacted `provider.extension` with method, provider timestamp and bounded payload.
4. Provider server request creates one `AgentInteraction`; Local Bridge WAL first persists `(profile_ref, source_kind, source_ref) -> raw provider ID`, then projects the source request ref and prevents duplicate response delivery. `mcpServer/elicitation/request` may create a thread-scoped interaction with no `source_turn_ref`; its Local Bridge resolution likewise has no raw provider turn ID.
5. `item.completed` is item terminal source; `turn.completed` is turn terminal source. A process close with no provider terminal invokes reconciliation, then projects `interrupted` only after current thread/turn read.
6. Result and error frames contain command ID, idempotency key and typed normalized code. Bridge result cache returns the preserved result for a repeat command instead of re-executing app-server.

## 6. Codex test matrix

当前测试分两层：确定性 fixture 覆盖六类 ServerRequest、notification disposition、source ref、WAL、重放与输入校验；真实 pinned 进程覆盖初始化、认证证明、10 种资源、完整线程生命周期、turn start/steer/interrupt、compact、review、历史投影和隔离 command probe。完整命令、结果与剩余环境依赖见 [10-app-server-validation-and-gap-plan.md](./10-app-server-validation-and-gap-plan.md)。
