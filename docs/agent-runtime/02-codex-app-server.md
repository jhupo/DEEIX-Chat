# Codex app-server 接入

## 1. 接入规则与 schema pin

Bridge launches `codex app-server` with stdio JSONL. The provider protocol is JSON-RPC 2.0 without a header; each line is one object. WebSocket and Unix transports exist upstream, but Bridge uses stdio because it binds process lifecycle locally and keeps app-server off the network. Upstream WebSocket transport is experimental and is not the Bridge transport.

The initial production adapter is pinned to official OpenAI Codex `rust-v0.147.0`, commit `be6e8eac029b183056b7e4402879f15d2c85f61b` (2026-08-07). Its reproducible authority is [codex-app-server-v0.147.0.lock.json](./codex-app-server-v0.147.0.lock.json): stable/non-experimental `codex app-server generate-ts --out ...` and `generate-json-schema --out ...`, release asset digest, generated artifact hashes, exhaustive union members and dispositions. Adapter startup follows this procedure:

1. Download and verify the lock's release asset, then install the pinned Codex dependency in the Bridge workspace.
2. Run the lock's stable generator commands without `--experimental`; commit generated source, schema hash and lock together.
3. Run the lock checker, then compile adapter method registry and fixtures against that output.
4. Start app-server, send `initialize`, verify protocol/runtime/schema compatibility, then send `initialized`.
5. Publish manifest `{ runtimeVersion, protocolVersion, schemaHash, capabilities }`; a mismatch produces `schema_mismatch`, which permits diagnostics and blocks command dispatch.

The lock file and its exact generated schema are implementation authority. The tables below are a human capability summary, not an exhaustive registry. A name outside this stable lock is not advertised or dispatched. Main-branch/experimental drift stays disabled until a separate `--experimental` lock has generated artifacts, hashes, exhaustive dispositions and review.

Official references: [app-server documentation](https://developers.openai.com/codex/app-server/), [app-server source](https://github.com/openai/codex/tree/main/codex-rs/app-server), and [Codex repository](https://github.com/openai/codex).

## 2. Local adapter boundary

```ts
import type { ThreadMetadataUpdateParams as CodexThreadMetadataUpdateParams } from "./generated/codex-app-server-v0.147.0/v2";

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

type TransferCommand = {
  kind: "transfer.execute";
  deviceId: string;
  workspaceId: string;
  threadId?: string;
  sourceThreadRef?: string;
  transferTicketRef: string;
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
  | { kind: "review.start"; deviceId: string; profileId: string; workspaceId: string; threadId: string; sourceThreadRef: string; target: ReviewTarget }
  | { kind: "turn.start"; deviceId: string; profileId: string; workspaceId: string; threadId: string; sourceThreadRef: string; input: AgentInput[]; settings: TurnSettings }
  | { kind: "turn.steer"; deviceId: string; profileId: string; workspaceId: string; threadId: string; sourceThreadRef: string; turnId: string; sourceTurnRef: string; input: AgentInput[] }
  | { kind: "turn.interrupt"; deviceId: string; profileId: string; workspaceId: string; threadId: string; sourceThreadRef: string; turnId: string; sourceTurnRef: string }
  | TurnInteractionCommand
  | ThreadInteractionCommand
  | TransferCommand
  | ProfileResourceCommand<"resource.refresh">
  | WorkspaceResourceCommand<"resource.refresh">
  | (ProfileResourceCommand<"resource.mutate"> & { mutation: ResourceMutation })
  | (WorkspaceResourceCommand<"resource.mutate"> & { mutation: ResourceMutation });

type ProviderKind = ProviderRegistry["kind"];
type ProviderAgentCommand = Exclude<AgentCommand, TransferCommand>;
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

Cloud `AgentCommand` has opaque public/source refs and allowlisted discriminants only. `thread.create` has no user input and no `sourceThreadRef` until its Bridge result/event binds it; its settings are only the typed `thread/start` settings accepted by the pinned schema. Browser initial input is stored on an optional provisional turn and appears solely in the follow-on `turn.start` command. Thread lifecycle/name/metadata/compact/review and turn start carry `sourceThreadRef`; `thread.metadata.update.patch` is only generated `thread/metadata/update.gitInfo`, whose only nested fields are optional nullable `sha`, `branch`, and `originUrl`; it rejects every other field, arbitrary JSON, and DEEIX labels/share/pin. Steer/interrupt also carry `sourceTurnRef`; interaction response carries `sourceThreadRef` and `sourceRequestRef`, plus `sourceTurnRef` only for turn scope. It has no canonical cwd, raw provider ID, raw RPC method, arbitrary JSON, credential or secret. Resolver validates every top-level and nested ref, then resolves source refs through the Local Bridge durable mapping `(profile_ref, source_kind, source_ref) -> raw provider ID` before constructing `ProviderCommand`; `thread.metadata.update` resolves only local raw thread ID plus canonical cwd. The `providerThreadId`, `providerTurnId` and `providerRequestId` fields exist only in this local adapter boundary. `transfer.execute` is a Bridge transport command, not a `ProviderAdapter` command: its persisted command payload has only `transferTicketRef` and allowlisted device/workspace/thread public/source refs. The distributive `ResolvedCommandMember` preserves `AgentResource.scope`: profile-scoped resources reject `canonicalCwd`, while workspace-scoped resources require it. `InteractionResolvedFields` preserves interaction scope: both forms require provider thread/request/cwd, turn scope requires provider turn, and thread scope rejects it. `mcpServer/elicitation/request` can be thread-scoped with no `source_turn_ref`; at the Local Bridge boundary that path likewise has no raw `providerTurnId`. Adapter receives a `kind`-narrowed union with no `unknown` payload. Bridge de-duplicates by command ID/result cache.

`thread/section/move` and `threadSection/*` remain separately locked extensions with no first-release control.

`ProviderManifest` declares availability as `stable`, `beta`, `experimental`, `under_development`, `disabled`, or `schema_mismatch`. Web controls are capability-gated and the server/Bridge revalidate before dispatch. `under_development` methods are visible as diagnostics only in the first production client.

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
| Skills/Hooks | `skills/list`, `skills/extraRoots/set`, `skills/config/write`, `skills/changed`, `hooks/list` | Stable schema may contain mutation methods, but first-release manifest/policy disables `skills/extraRoots/set` and `skills/config/write` | Query using canonical cwd; publish read-only resource snapshot/invalidation; create no mutation command | Workspace resources page lists skills/hooks and refreshes diagnostics; no root/config editing control |
| Plugins/marketplaces | marketplace add/remove/upgrade; plugin list/read/install/uninstall; `plugin/skill/read` | Plugin lifecycle is under development and excluded from first production mutation UI | Read snapshots may be projected; mutation mapper remains feature-flagged by manifest | Read-only diagnostics in first release; no raw marketplace path or arbitrary install request from browser |
| Apps | `app/installed`, `app/list`, `app/read` | Stable/beta based on generated schema | Profile resource snapshot and app change event | Installed Apps page reads profile-local state; item events link to safe app ref |
| MCP | MCP server status/list, resource/tool operations, OAuth, reload, startup notification | Stable schema may contain OAuth/config credential mutation, but first-release manifest/policy disables every MCP OAuth and credential/config mutation; reload is also disabled in favor of read-only state | Project redacted server/resource/tool state and startup diagnostics; create no OAuth/config command | MCP page is read-only diagnostics; secret values stay local and no OAuth start/reload control is rendered |
| Config | `config/read`, `config/value/write`, `config/batchWrite`, `configRequirements/read` | Stable schema may contain writes, but first-release manifest/policy disables `config/value/write` and `config/batchWrite` | Map allowlisted redacted config summary/requirements; create no write command | Config page is diagnostic/read-only; no secret/plaintext or editing control |
| Filesystem/command/process | Stable-lock filesystem and command-execution members; process client requests are unpinned experimental candidates | Stable entries follow the lock. Process client requests are disabled until a separate generated, hashed and exhaustive experimental lock exists | Resolve file refs/cwd locally, validate root, normalize outputs | Files panel uses signed file refs; command UI follows sandbox policy; process UI has no control before its experimental lock |
| Account/auth | `account/read`, `account/login/start`, `account/login/completed`, `account/login/cancel`, `account/logout`, `account/updated`, `account/chatgptAuthTokens/refresh`, `account/rateLimits/read`, `account/rateLimits/updated`, `account/usage/read`, `account/workspaceMessages/read` | Stable schema may contain mutation methods, but first-release manifest/policy disables `account/login/start`, `account/login/cancel`, `account/logout`, and `account/chatgptAuthTokens/refresh` | Project read-only auth mode/status/rate-limit/usage/workspace-message state, omitting tokens; create no auth command | Account diagnostics refresh projection only; no local-auth start/logout/token-refresh control |
| External config/environment | Stable-lock external config members; `environment/info` is an unpinned experimental candidate | Lock disposition governs. `environment/info` is disabled until a separate generated, hashed and exhaustive experimental lock exists | Diagnostic/resource events with field allowlist | Setup diagnostics and guided import status; no environment secret export |
| Preview extensions | Stable `experimentalFeature/list`, `experimentalFeature/enablement/set`, `windowsSandbox/setupStart` and `feedback/upload` members | Pinned stable members with disabled disposition; they have no dispatch until promoted with a typed mapper and fixtures | Dedicated typed extension mapper, audit each mutation | No control while disabled; failure does not affect normal thread execution |

### 3.1 Stable lock coverage

[codex-app-server-v0.147.0.lock.json](./codex-app-server-v0.147.0.lock.json) is the exhaustive authority for the four generated stable unions. A method enters execution only when the lock, manifest and local policy all declare it.

`thread/section/move` and `threadSection/*` remain separately locked extensions with no first-release control.

| Union | Members | Dispositions | Labeled examples |
| --- | ---: | --- | --- |
| `ClientRequest` | 98 | mapped 53, extension 26, disabled 19 | Full member set and dispositions are in the lock |
| `ClientNotification` | 1 | mapped 1 | `initialized` mapped |
| `ServerRequest` | 10 | mapped 6, extension 1, disabled 3 | `attestation/generate` disabled |
| `ServerNotification` | 72 | mapped 42, extension 19, disabled 11 | `item/fileChange/patchUpdated` mapped; `item/commandExecution/terminalInteraction` extension |

`mapped` members require a typed mapper and acceptance fixture. `extension` members remain redacted and inspectable without becoming controls; `disabled` members have no dispatch.

`collaborationMode/list`, `environment/info`, process client requests, `thread/turns/list`, `thread/items/list`, backgroundTerminal client requests, `currentTime/read`, and `tool/requestUserInput` are unpinned experimental candidates. They are disabled until a separate generated, hashed and exhaustive experimental lock exists; they do not follow or extend this stable `v0.147.0` schema.

## 4. Native actions and DEEIX metadata

Native Codex thread actions map only where the pinned schema has an equivalent method. `AgentThread.title` is the provider `thread/name` projection. The separate idempotent `PATCH /api/v1/agent/threads/:thread_id/name` accepts only bounded normalized `name`, resolves `sourceThreadRef`, and queues typed `thread.rename`; Bridge resolves raw ID/cwd and calls `thread/name/set`, whose provider result/event updates title. `PATCH /api/v1/agent/threads/:thread_id/provider-metadata` validates generated `gitInfo` only, with optional nullable `sha`, `branch`, and `originUrl`, claims idempotency, and queues typed `thread.metadata.update`; provider result/event updates Git projection. `PATCH /api/v1/agent/threads/:thread_id` remains DEEIX cloud metadata only: `preview` is derived, layout preference belongs in user settings, and pin/labels/share never enter either provider command. Archive/unarchive has a native call where upstream supports it, plus a projection state transition; the server waits for provider result before treating native archive as final.

`thread/section/move` and `threadSection/*` remain separately locked extensions with no first-release control.

`is_pinned` is DEEIX-owned cloud metadata, updated locally in the browser mutation's idempotent transaction with no app-server call. Other DEEIX-owned metadata includes labels, share policy, device/workspace association and product-specific audit references.

At initialize, the initial stable profile does not set `experimentalApi=true`; it advertises only implemented stable capabilities with a generated-schema mapper and acceptance fixture, such as `optOutNotificationMethods` and `mcpServerOpenaiFormElicitation` when implemented. `experimentalApi` may be enabled only after a separate experimental generated, hashed and exhaustive lock and its fixtures exist. `requestAttestation` remains absent from the advertised manifest until its disabled `attestation/generate` server-request handler is promoted with policy and fixtures. The manifest ties each declared capability to the exact generated schema version and local policy before Web exposes a control.

## 5. Mapper and terminal rules

1. Method registry covers every notification in the pinned generated union.
2. Typed mapper handles lifecycle, thread, turn, item, interaction, plan, diff, usage, artifact and error records.
3. Registry routes a recognized-but-not-yet-promoted notification to redacted `provider.extension` with method, provider timestamp and bounded payload.
4. Provider server request creates one `AgentInteraction`; Local Bridge WAL first persists `(profile_ref, source_kind, source_ref) -> raw provider ID`, then projects the source request ref and prevents duplicate response delivery. `mcpServer/elicitation/request` may create a thread-scoped interaction with no `source_turn_ref`; its Local Bridge resolution likewise has no raw provider turn ID.
5. `item.completed` is item terminal source; `turn.completed` is turn terminal source. A process close with no provider terminal invokes reconciliation, then projects `interrupted` only after current thread/turn read.
6. Result and error frames contain command ID, idempotency key and typed normalized code. Bridge result cache returns the preserved result for a repeat command instead of re-executing app-server.

## 6. Codex test matrix

Bridge fixtures use recorded, redacted JSONL samples for initialize, thread/turn lifecycle, every interaction kind, no-turn MCP elicitation from request through response to `serverRequest/resolved`, server-initiated `item/tool/requestUserInput` through sourceRequestRef/raw local request correlation and typed `interaction.respond`, item delta ordering, terminal events, command retry and schema mismatch. `tool/requestUserInput` is absent from this stable pin; a separately locked experimental fixture, if introduced, is never decoded as that server request. Each fixture asserts source-ref/raw-ID mapping, WAL write-before-send, thread projection order, Interaction CAS result and browser-visible event shape. A pinned-schema upgrade regenerates these fixtures before capability changes reach Web.
