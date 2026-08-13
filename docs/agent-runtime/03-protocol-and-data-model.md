# 协议与数据模型

## 1. Contract 层

| 层 | 参与方 | 事实源 | 允许的数据 |
| --- | --- | --- | --- |
| Provider | Bridge 与 Codex app-server | pinned generated TypeScript schema | canonical cwd、provider ID、raw request ID，仅本机 |
| Bridge | Bridge 与 Agent Gateway | WSS v1 envelope + private Bridge durable WAL store | device-scoped durable frames 与 typed command payload |
| Web | Browser 与 Agent HTTP API | Go DTO/Swagger -> `@deeix/api-contract` | opaque public IDs、typed intent、AgentEvent projection |

新增 HTTP DTO、handler annotation 和 Swagger response 后执行 `pnpm api:generate`，再执行 `pnpm api:check`。Generated Swagger and TypeScript files remain generated artifacts.

## 2. HTTP route groups

### 2.1 Device enrollment and Bridge credential

```text
POST   /api/v1/agent/devices/enrollments
GET    /api/v1/agent/devices
GET    /api/v1/agent/devices/:device_id
PATCH  /api/v1/agent/devices/:device_id
DELETE /api/v1/agent/devices/:device_id

POST   /api/v1/agent/bridge/enroll
POST   /api/v1/agent/bridge/token-challenges
POST   /api/v1/agent/bridge/tokens
GET    /api/v1/agent/bridge/connect
GET    /api/v1/agent/bridge/artifacts/:artifact_id/content
```

Browser enrollment creates a one-use code. Bridge exchanges code plus device public key, obtains a challenge, signs it locally, receives a short ordinary `connection` token, then upgrades WSS. Credential endpoints use device-specific rate limits and device-key verification rather than browser session authentication. These secret-returning mutations use the shared claim-first idempotency pipeline and return restart-safe reconstructed bearer values as defined in the credential section. During `revoking`, ordinary/new-work token issuance remains blocked; the same `token-challenges` plus `tokens` endpoints may issue only a typed `settlement` token after a fresh device-key-signed challenge when delivery-started work remains.

### 2.2 Devices, profiles, and account

```text
GET    /api/v1/agent/devices/:device_id/profiles
GET    /api/v1/agent/profiles/:profile_id
GET    /api/v1/agent/profiles/:profile_id/capabilities
GET    /api/v1/agent/profiles/:profile_id/account
GET    /api/v1/agent/profiles/:profile_id/rate-limits
```

First release projects only existing local account/status/rate-limit state needed to validate auth; it creates no local-auth `AgentCommand`. Profile lookup always verifies its device belongs to the caller.

### 2.3 Workspaces and files

```text
GET    /api/v1/agent/devices/:device_id/workspaces
POST   /api/v1/agent/workspaces/:workspace_id/artifacts
GET    /api/v1/agent/devices/:device_id/workspaces/:workspace_id/resources/:resource
POST   /api/v1/agent/devices/:device_id/workspaces/:workspace_id/resources/:resource/refresh
```

Browser first uses the existing `/files` upload. Artifact registration accepts that opaque `fileId`, validates ownership and image/audio metadata, then returns `artifactId`. Turn input carries only `artifactRef`; requests never contain a local absolute path.

### 2.4 Thread lifecycle

```text
GET    /api/v1/agent/threads?device_id=&workspace_id=&profile_id=&status=&query=&cursor=
POST   /api/v1/agent/threads
GET    /api/v1/agent/threads/:thread_id
GET    /api/v1/agent/threads/:thread_id/snapshot
PATCH  /api/v1/agent/threads/:thread_id
PATCH  /api/v1/agent/threads/:thread_id/name
PATCH  /api/v1/agent/threads/:thread_id/provider-metadata
POST   /api/v1/agent/threads/:thread_id/fork
POST   /api/v1/agent/threads/:thread_id/archive
POST   /api/v1/agent/threads/:thread_id/unarchive
DELETE /api/v1/agent/threads/:thread_id
GET    /api/v1/agent/threads/:thread_id/turns?cursor=
GET    /api/v1/agent/threads/:thread_id/items
GET    /api/v1/agent/threads/:thread_id/events?after_seq=N
GET    /api/v1/agent/threads/:thread_id/notifications
GET    /api/v1/agent/threads/:thread_id/interactions?status=
```

`POST /threads` accepts device/profile/workspace public IDs, typed thread-start settings and optional initial input. It creates only a `thread.create` command: that command carries no input. When input is present, the transaction stores it in a provisional AgentTurn `awaiting_thread`; after the Bridge `thread/start` result/event binds `sourceThreadRef`, projector creates the one follow-on `turn.start` command. `PATCH /threads/:thread_id` persists only DEEIX labels, share policy and `is_pinned` directly in cloud metadata in the same idempotent transaction; pin has no app-server call. The separate idempotent `PATCH /threads/:thread_id/name` accepts only bounded normalized `name`, resolves `sourceThreadRef`, and queues `thread.rename`; Bridge resolves raw thread ID/cwd for `thread/name/set`, and provider result/event updates title. The separate idempotent `PATCH /threads/:thread_id/provider-metadata` validates only generated pinned `ThreadMetadataPatch.gitInfo`: optional nullable `sha`, `branch`, and `originUrl`; it resolves `sourceThreadRef` and queues `thread.metadata.update`, rejecting arbitrary JSON and DEEIX labels/share/pin fields. Provider result/event updates the Git projection. `fork`, archive, unarchive and delete queue typed lifecycle commands. UI layout preference belongs to user settings. Browser event replay is always thread-scoped.

当前后端允许一个 User 配对多个 Device；每个 Device 独立维护凭据、Runtime Profile、Workspace、WSS 游标和命令序列。同一 Device 只保留一个活跃 WSS，新连接接管旧连接；不同 Device 可同时在线。AgentThread 创建后永久绑定一个 `device_id + runtime_profile_id + workspace_id`，元数据接口不接受修改这三个字段。命令提交后，单实例 Full 服务按 User 唤醒其在线连接，但每条连接只按自身内部 Device ID 拉取连续命令，因此多个网关之间不会串发。唤醒只降低延迟，PostgreSQL `agent_commands` 和设备游标仍是恢复事实源。

`GET /threads/:thread_id/snapshot` 在 PostgreSQL `REPEATABLE READ READ ONLY` 事务中读取 Thread、Turn、Item、Interaction 和 `snapshotSeq`。调用方安装快照后仅回放 `thread_seq > snapshotSeq` 的事件。`PATCH /threads/:thread_id/provider-metadata` 生成 `thread.metadata.update`，Bridge 严格保留 Git 字段的 omit/null/value 语义并调用 Codex `thread/metadata/update`；只有成功 terminal result 才更新云端 Git 投影。

`GET /threads/:thread_id/notifications` 在校验线程所有权后提供用户级 SSE wake。连接成功立即发送一次 `wake`，后续 provider frame、terminal result、云端元数据或命令提交只发送空 wake；调用方收到后仍查询数据库快照/事件接口。15 秒注释心跳只维持连接，不推进任何事件 cursor。

`thread/section/move` and `threadSection/*` are separately locked extensions with no first-release control.

### 2.5 Turns, reviews, and interaction

```text
POST   /api/v1/agent/threads/:thread_id/turns
POST   /api/v1/agent/turns/:turn_id/steer
POST   /api/v1/agent/turns/:turn_id/interrupt
POST   /api/v1/agent/threads/:thread_id/reviews
POST   /api/v1/agent/threads/:thread_id/compact
POST   /api/v1/agent/interactions/:interaction_id/respond
```

All mutation routes require `Idempotency-Key`. `agent_idempotency_records` is the HTTP idempotency fact: same user/operation/key/request hash returns the first committed response; same user/operation/key with a different request hash returns conflict. Payloads are bounded by DTO validation before command creation.

Serialized command payloads carry public IDs, applicable `sourceThreadRef`, `sourceTurnRef` and `sourceRequestRef`, typed input, and allowlisted `expiresAt`/deadline where expiry matters only. `thread.create` has neither user input nor `sourceThreadRef` until Bridge result/event binding; it carries only typed pinned-schema thread-start settings. Lifecycle, typed `thread.metadata.update`, and turn-start commands carry `sourceThreadRef`; metadata patch contains only generated writable `gitInfo` fields: optional nullable `sha`, `branch`, and `originUrl`. Steer/interrupt carry both thread and turn refs; interaction response carries thread/request refs and carries a turn ref only for turn scope. `item/tool/requestUserInput` is a server-initiated typed request with source-request correlation, projected as AgentInteraction and answered through `interaction.respond`. `tool/requestUserInput` is absent from this stable lock; any separately locked experimental form does not share its payload or correlation. The local resolver resolves metadata's raw thread ID and canonical cwd through durable source mappings before it forms `ProviderCommand`; cloud command payloads never contain raw provider IDs.

`thread/section/move` and `threadSection/*` remain separately locked extensions with no first-release control.

Every HTTP mutation uses this order:

1. Authenticate the user, bind DTO, perform pure structural/size/enum validation, normalize the accepted request, and calculate `request_hash`.
2. Start a transaction and claim/read `(user_id, operation, idempotency_key)`. A committed matching-hash record returns its saved response immediately, before ownership, capability, current-state or CAS validation. A differing hash returns conflict.
3. Only a newly inserted record continues with ownership/capability/current-state validation, aggregate mutation/CAS, command/audit creation, response persistence, and commit.
4. Concurrent requests lock/read the same record and observe that committed response after the first transaction finishes.

State checks that may change after the first call, such as a pending interaction becoming responding, are exclusively in the new-record branch. Labels, share policy, lifecycle actions and uploads use the same order; first-release runtime resource surfaces are read-only.

### 2.6 Runtime resources, configuration, share and audit

```text
GET    /api/v1/agent/profiles/:profile_id/models
GET    /api/v1/agent/profiles/:profile_id/permission-profiles
GET    /api/v1/agent/profiles/:profile_id/collaboration-modes
GET    /api/v1/agent/workspaces/:workspace_id/skills
GET    /api/v1/agent/workspaces/:workspace_id/hooks
GET    /api/v1/agent/profiles/:profile_id/plugins
GET    /api/v1/agent/profiles/:profile_id/marketplaces
GET    /api/v1/agent/profiles/:profile_id/apps
GET    /api/v1/agent/profiles/:profile_id/mcp
GET    /api/v1/agent/profiles/:profile_id/config
POST   /api/v1/agent/threads/:thread_id/share
DELETE /api/v1/agent/threads/:thread_id/share/:share_id
GET    /api/v1/agent/threads/:thread_id/audit?cursor=
```

First release projects read-only skills/hooks/plugins/apps/MCP/config diagnostics only. It creates no AgentCommand for local auth, config, skill-root/config, MCP OAuth/credential, plugin, or reload mutation; those stable-schema methods are manifest/policy-disabled. Preview/under-development capabilities return a disabled descriptor until product support and mapper fixtures exist. Shares are curated snapshots of redacted projections, never a live device connection.

## 3. WSS and credentials

```text
GET wss://HOST/api/v1/agent/bridge/connect
Sec-WebSocket-Protocol: deeix.bridge.v1, deeix.auth.<connection-token>
```

Enrollment code, challenge nonce and `connection` token are deterministic Base64URL bearers derived with Go standard-library HMAC-SHA-256 over purpose, issuance public ID, User/device credential version and expiry. Database rows store only SHA-256 `token_hash` and immutable issuance fields. Connection tokens are short-lived and one-use. Upgrade consumes the token, selects `deeix.bridge.v1` and never places token text in URL logs. Device revocation rotates the credential version so all earlier challenges and connection tokens fail validation.

Revocation increments the device credential version and blocks further challenge, connection and command issuance. No second connection-token class exists.

Derivation key ring retains an old key until every issuance under that version has expired and its idempotency replay window has elapsed. Key material is server secret/KMS configuration only; database data contains key version, hash and immutable issuance fields.

```ts
type BridgeEnvelope = {
  version: 1;
  type: "hello" | "welcome" | "command" | "event" | "command.result" | "command.error" |
    "interaction.request" | "interaction.resolved" | "snapshot" | "ack" | "ping" | "pong";
  deviceId: string;
  id: string;
  bridgeSeq?: number;
  serverSeq?: number;
  ackBridgeSeq?: number;
  ackServerSeq?: number;
  connectionEpoch?: string;
  payload: unknown;
};

type ArtifactGrant = {
  artifactRef: string;
  fileName: string;
  mimeType: string;
  sizeBytes: number;
  sha256: string;
  expiresAt: string;
  grant: string;
};

type BridgeCommandEnvelope = Omit<BridgeEnvelope, "type" | "serverSeq" | "payload"> & {
  type: "command";
  serverSeq: number;
  commandId: string;
  command: AgentCommand;
  artifacts: ArtifactGrant[];
};
```

`hello`, `welcome`, `ack`, `ping`, and `pong` are control frames. Every durable Bridge-to-server frame has `bridgeSeq`; every command frame has server-allocated `serverSeq`. `artifacts` is always explicit, including an empty array. Its grants are five-minute HMAC values bound to artifact, command, User, device, workspace and expiry. Bridge validates the frame, persists only the command, verifies each attachment, writes `command.receipt-ready`, then advances the contiguous ACK cursor. Grants never enter command payload, WAL, browser response or terminal cache.

`hello.ackServerSeq` is Bridge's contiguous durable receipt cursor for cloud-to-Bridge commands. After credential, epoch, range and contiguity validation against allocated command rows, the server may advance cloud `last_acked_server_seq` and mark covered commands acked. `hello.ackBridgeSeq` is only Bridge's last observed cloud acknowledgement for diagnostics/reconciliation; it never advances cloud `last_acked_bridge_seq`. `welcome` returns cloud `last_acked_bridge_seq` as the authoritative upstream cursor, and Bridge resends WAL frames strictly above that value. Thus a stale or higher Bridge-reported `ackBridgeSeq` cannot suppress resend or cause cloud cursor advancement; only the contiguous staged-frame projection transaction advances `last_acked_bridge_seq`.

## 4. Durable sequencing

### 4.0 PostgreSQL transaction rules

Agent projections and commands use PostgreSQL with pgvector as the only server Gorm database. Write transactions use row locks, upserts, unique constraints and partial indexes for device `server_seq`, thread `thread_seq`, bridge-frame dedupe/projection and HTTP idempotency claim/replay. They never perform socket, provider, object or other network I/O. PostgreSQL integration tests cover crash rollback with no duplicate sequence, command or event. Redis is required for cache/wake behavior; it never replaces database replay as the authoritative history.

### 4.1 Upstream Bridge to server

Bridge allocates monotonically increasing `bridge_seq` in its private durable WAL store and writes the full durable envelope before attempting WSS send. This embedded local crash/reconnect store is not a server deployment database. Gateway first inserts/deduplicates the frame as staged inbox evidence in `agent_events` with unique `(device_id, bridge_seq)`; receipt/inbox storage is distinct from projection. `last_acked_bridge_seq` means the largest contiguous applied/projected cursor, never the largest merely persisted cursor. If `bridgeSeq > last_acked_bridge_seq + 1`, the PostgreSQL transaction commits the frame as staged and returns only the current contiguous ack: it does not project the frame or allocate `thread_seq`. When the next contiguous frame is present, `ApplyBridgeFrame` applies it and drains a bounded contiguous staged run in `bridge_seq` order, atomically per frame or bounded PostgreSQL transaction; each applied frame updates projection, any `thread_seq`, and the contiguous cursor. Staged/applied duplicates are no-ops. A post-commit Redis wake/worker may continue a bounded drain, and database replay remains authoritative.

`agent_events` contains both durable inbox evidence and browser event projection. A frame with a resolved thread receives `thread_seq`; device/profile/command frames have NULL `thread_id` and NULL `thread_seq`. Browser replay filters to its thread and non-NULL sequence, so device activity has no accidental placement in a thread timeline.

### 4.2 Downstream server to Bridge

Command creation serializes the `agent_devices.next_server_seq` read-update and `agent_commands` insert with the PostgreSQL device row lock. It commits the command with its aggregate projection. `next_server_seq` starts at 1 and always represents the next value available for allocation.

Command dispatcher selects rows above `last_acked_server_seq`, marks delivery before WSS write and resends the same command ID/sequence after ambiguous disconnect. Bridge WAL states are `received`, `receipt-ready`, `invocation_started` and `terminal_cached`. Attachment failure leaves the command received but not receipt-ready, so neither current ACK nor restart hello can skip it. A cached terminal result is re-emitted without a second provider call. A restored unsafe invocation becomes `outcome_unknown`; read-only resource refresh and explicitly idempotent lifecycle operations may replay. ACK and command completion remain independent timestamps.

### 4.3 Local command recovery

On Bridge/app-server startup, Bridge initializes provider, ingests/reconciles provider state/events and source mappings, then drains incoming commands strictly by `server_seq`. Tombstone has no provider call; `terminal_cached` re-emits; `received` with no invocation marker executes. `invocation_started` with no cached terminal never blindly executes. A single typed per-operation recovery registry either proves provider postcondition/source binding and synthesizes/caches the normalized result, proves non-application and permits one execution, or records/emits normalized terminal `outcome_unknown` and requires inspection/reconciliation without automatic replay; safe read-only methods may be classified retryable. It persists every recovered result/error before its durable up-frame. JSON-RPC is not assumed to provide universal exactly-once semantics.

`thread.create` and the follow-on initial `turn.start` reconcile provider thread/turn state and source mappings before this recovery decision. A recovered binding flows through the existing cloud conditional transition and partial unique for the provisional `awaiting_thread` turn, so it cannot create another initial `turn.start`; ambiguous acceptance never creates another provider thread or turn.

### 4.4 Browser thread order

When `ApplyBridgeFrame` projects a thread-visible event, it locks that AgentThread, increments `last_thread_seq`, and writes `agent_events.thread_seq`. `AgentEvent.seq` is exactly this value. The unique `(thread_id, thread_seq)` applies only where both columns are present. `thread_seq` has no relation to either wire cursor.

```ts
export type AgentEvent = {
  eventId: string;
  seq: number; // thread_seq
  kind: string;
  threadId: string;
  deviceId: string;
  profileId?: string;
  workspaceId?: string;
  turnId?: string;
  itemId?: string;
  interactionId?: string;
  occurredAt: string;
  payload: unknown;
};
```

## 5. Persistent model

All public IDs are immutable opaque strings. Cloud source refs are opaque text values. Raw provider IDs use text types only in Local Bridge durable mappings. JSON values carry schema version and are encrypted/redacted where they can contain user content. Every ownership foreign key below is `user_id` and references the existing `identity_users.id`; Browser DTOs expose the owner's existing `identity_users.public_id` and never accept a caller-provided user ID.

### 5.1 `agent_devices`

```text
id, public_id, user_id, name, platform_json, status, device_public_key,
credential_version, bridge_version, protocol_version,
next_server_seq, last_acked_server_seq, last_acked_bridge_seq,
last_seen_at, revoked_at, created_at, updated_at
```

Unique `public_id`; indexes `(user_id, status, last_seen_at)`. `next_server_seq` and `last_acked_server_seq` are delivery cursors. `last_acked_bridge_seq` is the largest contiguous Bridge frame cursor already applied/projected, not an inbox-receipt cursor; all are updated under the configured database transaction rules.

### 5.2 `agent_device_credentials`

```text
id, public_id, user_id, device_id NULL, kind, token_hash, derivation_key_version,
credential_version, settlement_upper_server_seq NULL, expires_at, consumed_at, created_at
```

Unique `public_id` and `token_hash`; index `(device_id, kind, expires_at)`. `kind` is enrollment, challenge, ordinary `connection`, or `settlement`. Enrollment has NULL `device_id`; Bridge exchange creates/binds the device and then consumes enrollment. Challenge and connection require `device_id`. `settlement` requires `revoking` device status, fresh device-key challenge, current credential version, remaining delivery-started work, and non-NULL immutable `settlement_upper_server_seq`; final revoke/credential rotation invalidates it. Expired/consumed hashes are retention-cleaned after audit window.

### 5.3 `agent_runtime_profiles`

```text
id, public_id, device_id, provider, display_name, status, adapter_version,
runtime_version, provider_protocol_version, schema_hash, capabilities_json,
auth_state_json, last_error_json, last_synced_at, created_at, updated_at
```

Unique `(device_id, provider, display_name)`; indexes `(device_id, status)` and `(schema_hash)`. `auth_state_json` holds mode/status only.

### 5.4 `agent_workspaces`

```text
id, public_id, device_id, local_ref, name, display_path, git_json, status,
discovered_from, metadata_json, last_seen_at, created_at, updated_at, deleted_at
```

Unique `(device_id, local_ref)` and `public_id`; index `(device_id, status)`. `local_ref` is Bridge-generated. Canonical path stays local; `display_path` is optional redacted presentation text.

### 5.5 `agent_threads`

```text
id, public_id, user_id, device_id, runtime_profile_id, workspace_id,
source_thread_ref NULL, source_kind, title, preview, labels_json,
metadata_json, status, is_pinned, archived_at, forked_from_thread_id,
git_json, last_turn_id, last_thread_seq, last_synced_at,
created_at, updated_at, deleted_at
```

Partial unique `(runtime_profile_id, source_thread_ref)` where `source_thread_ref` is non-NULL, plus unique `public_id`; indexes `(user_id, device_id, workspace_id, updated_at)` and `(workspace_id, status)`. This schema has no ConversationProject relation. Thread create begins with NULL source ref; Bridge result/event backfills its high-entropy stable source ref atomically. In that same transaction, a locked provisional `awaiting_thread` turn may receive its sole initial `turn.start` command; duplicate result/event sees the existing linked command and creates none.

`source_thread_ref`, `source_turn_ref`, `source_item_ref` and `source_request_ref` are high-entropy stable opaque values issued by Bridge. They are not derivable from a raw provider ID. Cloud tables, API DTOs and command aggregates carry public IDs/source refs only; Local Bridge uses its durable mapping to resolve the corresponding raw IDs for `ProviderCommand`.

`title` is the provider `thread/name` projection and changes through durable native rename command/result processing. `preview` is derived projection text, not an independently edited title source.

### 5.6 `agent_turns` and `agent_items`

```text
agent_turns(id, public_id, thread_id, source_turn_ref NULL, ordinal, status,
  input_encrypted_json NULL, input_summary, settings_snapshot_json,
  error_code, error_json, usage_json,
  started_at, completed_at, cancel_requested_at, created_at, updated_at)

agent_items(id, public_id, turn_id, source_item_ref, ordinal, type, status,
  phase, text, payload_encrypted_json, display_json, started_at, completed_at,
  created_at, updated_at)
```

Partial unique `(thread_id, source_turn_ref)` where `source_turn_ref` is non-NULL, plus unique `(thread_id, ordinal)`, `(turn_id, source_item_ref)`, `(turn_id, ordinal)`; indexes on status/timestamps. `awaiting_thread` is an initial-input-only provisional turn. Its locked conditional transition to `queued`, together with `agent_commands` partial unique `(turn_id) WHERE operation='turn.start' AND initial_thread_create=true`, prevents a second initial command across duplicate Bridge events or restart without a turn-to-command foreign key. Turn create begins with NULL source ref; Bridge result/event backfills it atomically. Item source refs are present when created from Bridge source mapping. `display_json` is redacted share/admin projection.

### 5.7 `agent_interactions`

```text
id, public_id, user_id, thread_id, turn_id NULL, runtime_profile_id, source_request_ref,
scope, kind, request_encrypted_json, display_json, response_encrypted_json, status,
terminal_code NULL, expires_at, responded_at, resolved_at, provider_cleared_at, terminal_at NULL,
created_at, updated_at
```

Unique `(runtime_profile_id, source_request_ref)` and `public_id`; indexes `(thread_id, status, expires_at)` and `(user_id, status)`. `scope` is `turn` or `thread`: turn scope requires non-NULL `turn_id` related to `thread_id`; thread scope requires NULL `turn_id` and is used by no-turn MCP elicitation. Thread/profile/source request ref are always present. `pending` may expire. A `responding` interaction may expire atomically with its response command only while that command has not begun delivery; after delivery starts, deadline enforcement belongs to Bridge and the interaction remains `responding` until provider resolved/cleared or linked Bridge terminal result/recovery writes resolved, failed, or expired as appropriate. Thread soft deletion retains interaction/audit data for configured history window, then retention worker redacts encrypted request/response before physical purge.

### 5.8 `agent_events`

```text
id, public_id, device_id, bridge_seq, projection_state, thread_id NULL, thread_seq NULL,
runtime_profile_id NULL, turn_id NULL, item_id NULL, interaction_id NULL,
command_id NULL, kind, provider_method, payload_encrypted_json, display_json,
occurred_at, received_at, created_at
```

Unique `(device_id, bridge_seq)`; `projection_state` is `staged` or `applied`; partial unique `(thread_id, thread_seq)` where both fields are non-NULL; indexes `(thread_id, thread_seq)`, `(device_id, received_at)`, `(command_id)`. A staged frame has no thread projection/`thread_seq`; device/profile frames that are applied remain durable rows with NULL thread fields. Retention keeps browser replay rows for product history and compacts/redacts old payloads according to policy.

### 5.9 `agent_commands`

```text
id, public_id, user_id, device_id, runtime_profile_id NULL, workspace_id NULL,
thread_id NULL, turn_id NULL, interaction_id NULL, idempotency_record_id NULL,
operation, initial_thread_create boolean NOT NULL default false, server_seq, payload_json, status, attempt_count, delivery_started_at NULL, dispatched_at, last_sent_at,
acked_at, completed_at, result_json, error_code, error_json, expires_at,
created_at, updated_at
```

Unique `(device_id, server_seq)` and guarded initial-turn command creation prevent duplicate provider work. `payload_json` contains only typed input and public/source refs; it never contains raw provider IDs, local paths or artifact grants. `acked_at` records Bridge receipt readiness and is independent from provider completion. Terminal replay with the same hash is a no-op; conflicting terminal data is rejected.

### 5.10 `agent_idempotency_records`

```text
id, public_id, user_id, operation, idempotency_key, request_hash,
response_status, response_skeleton_json, secret_kind NULL, secret_ref NULL, command_id NULL,
created_at, expires_at
```

Unique `(user_id, operation, idempotency_key)`. `request_hash` is calculated from normalized validated request content. Enrollment/challenge/connection bearer reconstruction uses its credential row and verifies the stored token hash. Every Agent mutation claims the idempotency record, validates aggregate state, creates the command and stores the result in one transaction.

### 5.11 `agent_artifacts`

```text
id, public_id, user_id, workspace_id, file_object_id, file_name,
mime_type, size_bytes, sha256, status, created_at, updated_at
```

Unique `(workspace_id, file_object_id)` makes registration idempotent. Only active image/audio file objects are accepted. Command creation verifies every `artifactRef` against User and workspace. Content delivery additionally binds the artifact to the concrete command/device before issuing a short-lived HMAC grant.

### 5.12 `agent_cleanup_jobs`

```text
id, dedupe_key, cleanup_kind, object_ref, temp_ref NULL, status, attempt_count,
next_attempt_at, last_error_code NULL, last_error_at NULL, lease_token NULL,
lease_owner NULL, leased_at NULL, leased_until NULL, completed_at NULL, created_at, updated_at
```

`agent_cleanup_jobs` has no foreign key to a user or Agent aggregate. Unique `dedupe_key` prevents duplicate deletion work for an object/temp ref. The account-delete transaction revokes access and inserts eligible jobs before removing artifact/ticket rows. PostgreSQL claims bounded due rows in a transaction with `FOR UPDATE SKIP LOCKED`, sets unique `lease_token`/owner/deadline, commits before object deletion, then CAS-completes or backoffs by lease token. Transactions never perform network/object I/O. Failure CAS increments attempts and backoff; lease timeout is reclaimable; object delete is idempotent. Retention removes completed jobs after the audit window.

## 6. Transactions and state machines

### 6.1 `StartAgentThread`

1. Authenticate, complete pure DTO validation, normalize request and calculate `request_hash`.
2. Under transaction, claim/read idempotency record. Matching committed record returns its saved response; differing hash returns conflict.
3. New record validates User/device/profile/workspace ownership and ready capability, inserts provisional AgentThread `queued`, and, when browser supplied initial input, inserts one AgentTurn `awaiting_thread` with encrypted input/settings. It allocates only `thread.create` `server_seq`, inserts its command/audit/idempotent response, and does not serialize input into that command.
4. Commit, publish Hub wake-up, return thread plus command status. On the first Bridge `thread/start` result/event, projector locks the thread and provisional turn, binds `source_thread_ref`, conditionally changes `awaiting_thread -> queued`, allocates the next device `server_seq`, inserts exactly one `turn.start(initial_thread_create=true)` with stored input, and commits the projection together. Recovery first reconciles provider thread/turn state and source mappings; a recovered binding takes this same conditional-transition/partial-unique path. Duplicate result/event or restart creates none, and ambiguous acceptance is `outcome_unknown` rather than another provider thread or turn. A terminal thread-create failure, cancellation or expiry terminalizes its awaiting turn with matching normalized code/time.

### 6.2 `StartAgentTurn`

1. Authenticate, complete pure DTO validation, normalize request and calculate `request_hash`.
2. Transactionally claim/read idempotency record. Matching committed record returns saved response before thread status, ownership or active-turn validation; differing hash returns conflict.
3. New record validates thread status, User ownership, matching workspace/profile, capability and no conflicting active turn; it inserts AgentTurn `queued`, updates thread summary, allocates command sequence, inserts linked typed command, audit and saved response.
4. Commit then notify dispatcher. Provider event makes turn `in_progress`; terminal provider event decides completed/interrupted/failed.

### 6.3 command dispatcher

Business state flow is `queued -> dispatched -> completed|failed_terminal`. The first delivery attempt is recorded before WSS write; ambiguous disconnect resends the same command ID and sequence. `acked_at` is receipt readiness, not provider completion. A terminal frame settles the matching command once and projects its typed result into Thread/Turn/Resource state.

### 6.4 `ApplyBridgeFrame`

1. Validate connection epoch, device identity, payload limit and required `bridge_seq`.
2. Start transaction; insert/deduplicate `agent_events` inbox evidence by `(device_id, bridge_seq)` as `staged`. Duplicate of staged or applied data returns the current contiguous applied cursor.
3. If the received frame is not `last_acked_bridge_seq + 1`, commit it staged and send only the current contiguous ack; do not project it or allocate `thread_seq`.
4. If the next contiguous frame exists, apply it and bounded-drain following staged frames in `bridge_seq` order. For each frame, apply command result/error, device/profile snapshot, interaction or provider event; for a thread-visible event lock thread, allocate `thread_seq`, and update turn/item/interaction projection in the same transaction; mark frame applied and advance `last_acked_bridge_seq`.
5. Commit, publish notifier/post-commit drain wake, then send `ackBridgeSeq` equal to the largest contiguous applied cursor.

### 6.5 `RespondInteraction`

1. Authenticate, complete pure response-shape/size/enum validation, normalize response and calculate `request_hash`.
2. In a transaction claim/read idempotency record. Matching committed record returns the first response even when the interaction is already `responding`; differing hash returns conflict.
3. New record validates User/thread/device/profile relation and `status=pending` with unexpired deadline. Turn scope also validates the supplied turn relation; thread scope validates thread/profile/request ID and permits no turn. It then CAS updates `pending -> responding`, allocates `server_seq`, inserts linked `interaction.respond` command, audit and saved response.
4. Commit and dispatch. Provider resolved/cleared event may close a pending or responding interaction, but a late provider event never regresses a terminal row. Expiry worker expires `pending`; for `responding`, it atomically expires command/interaction and emits a tombstone only before first delivery attempt. After delivery starts it leaves the interaction `responding`; Bridge checks the command deadline before invocation/recovery and settles resolved, failed, or expired through its terminal frame. Cloud cancellation/revoke follows the same pre-delivery boundary; retryable pre-write delivery failure keeps it responding.

### 6.6 Browser replay, restart and revoke

With a mounted reducer, Browser opens the Redis notifier subscription first and replays `thread_seq > lastAppliedSeq` strictly from its existing cursor; it never replaces that cursor from a thread header. On cold load, cursor gap or compaction recovery, Browser subscribes first, fetches one atomic aggregate snapshot of thread plus included turns/items/interactions and `snapshotSeq` from one PostgreSQL read snapshot, replaces reducer state, sets `lastAppliedSeq=snapshotSeq`, then replays events `> snapshotSeq`. Notifier payload is wake-up only, so every wake queries again. The HTTP snapshot DTO carries `snapshotSeq`; tests commit events between subscribe, snapshot and replay. API restart restores commands from PostgreSQL; Bridge reconnect resends WAL frames and receives commands above contiguous ack. Provider restart reconciles current thread/turn state before marking a missing active turn interrupted. Device revoke uses `revoking` settlement: it blocks new commands and ordinary credential issuance, terminalizes only never-delivered work and pending interactions, and keeps an authenticated bounded settlement-only connection for delivery-started work until Bridge result/recovery settles it. It then finalizes `revoked`, rotates credential and closes connection; offline unsettled devices remain `revoking`. `DeleteAccountHard` first writes an independent `sub2_revocation_cleanup` record containing deletion-specific encrypted refresh-token material/ref, immutable `not_after`, and no User/account foreign key. `not_after` is calculated from conservative receipt time plus startup-mandatory pinned-Sub2 `sub2_refresh_token_max_ttl` and safety margin. The same transaction revokes DEEIX local access/session, accepts no late frame, and deletes DEEIX User/key/device/content aggregates without removing that retry material. After commit, only the cleanup worker may call Sub2 `POST /auth/logout`; its response is audit evidence only, so cleanup remains `pending` and retains the token/ref until `not_after` passes, then securely erases it and marks `completed`. It never calls `/auth/revoke-all-sessions`, does not invalidate unrelated Sub2 client sessions, does not delete the Sub2 account, and does not claim to cancel local provider activity. Late provider frames remain non-regressing.

Deletion status is operation-scoped: a User-independent barrier links all current, pending and late candidate cleanup rows for the stable Sub2 account; only after every member's immutable `not_after` and the operation `deletion_not_after` pass does the receipt report `completed`.

Workspace deletion and retained sensitive account mutations first consume a short-lived, single-use `user_session_step_up_grant` bound to the current User session, exact purpose and current account/token version in the same transaction. Cross-session, wrong-purpose, expired, replayed, session-revoked or token-rotated grants fail; unsupported factors have no fallback mutation path.

Protected Browser route authentication is User-session cookie only: middleware validates the hashed opaque session, expiry/revocation, active User and local role before handler; legacy Browser Bearer and caller-provided role claims fail. Bridge and deletion-receipt verifiers remain separate.

### 6.7 Aggregate state machines

```text
device: enrolled -> connecting -> online -> degraded -> offline
                                      |                    |
                                      `-----> revoking <----'
                                                |
                                                `-> revoked

profile: starting -> ready | schema_mismatch | auth_required | error | stopped

thread: queued -> idle -> active -> waiting_interaction -> idle
                   |       |                     |
                   |       +-> archived          +-> system_error
                   `-> deleted

turn: awaiting_thread -> queued -> dispatched -> in_progress -> completed
                                  |     |
                                  |     +-> interrupted | failed
                                  `-> waiting_interaction -> in_progress

interaction: pending -> responding -> resolved | provider_cleared
                 |            |
                 +-> expired  +-> cancelled | expired | failed
                 `-> cancelled
```

Each state change comes from a validated command result, provider frame, retention worker, or revoke transaction. Browser mutations request transitions; they do not write final provider state directly.
