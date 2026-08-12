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
POST   /api/v1/agent/bridge/transfers/:transfer_ticket_ref/claim
PUT    /api/v1/agent/bridge/transfers/:transfer_ticket_ref/data
GET    /api/v1/agent/bridge/transfers/:transfer_ticket_ref/data
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
POST   /api/v1/agent/devices/:device_id/workspaces/pick
PATCH  /api/v1/agent/workspaces/:workspace_id
DELETE /api/v1/agent/workspaces/:workspace_id
GET    /api/v1/agent/workspaces/:workspace_id/files?parent_ref=&query=&cursor=
GET    /api/v1/agent/workspaces/:workspace_id/resources
POST   /api/v1/agent/workspaces/:workspace_id/uploads
GET    /api/v1/agent/artifacts/:artifact_id/download
```

Picker is a queued Bridge command. Files use signed opaque `file_ref`; requests never include a local absolute path. Upload and artifact download endpoints issue durable `agent_transfer_tickets` rows after user/workspace ownership checks and return the ticket public ref and state, not a transfer bearer.

### 2.4 Thread lifecycle

```text
GET    /api/v1/agent/threads?device_id=&workspace_id=&profile_id=&status=&query=&cursor=
POST   /api/v1/agent/threads
GET    /api/v1/agent/threads/:thread_id
PATCH  /api/v1/agent/threads/:thread_id
PATCH  /api/v1/agent/threads/:thread_id/name
PATCH  /api/v1/agent/threads/:thread_id/provider-metadata
POST   /api/v1/agent/threads/:thread_id/fork
POST   /api/v1/agent/threads/:thread_id/archive
POST   /api/v1/agent/threads/:thread_id/unarchive
DELETE /api/v1/agent/threads/:thread_id
GET    /api/v1/agent/threads/:thread_id/turns?cursor=
GET    /api/v1/agent/threads/:thread_id/events?after_seq=N
GET    /api/v1/agent/threads/:thread_id/interactions?status=
```

`POST /threads` accepts device/profile/workspace public IDs, typed thread-start settings and optional initial input. It creates only a `thread.create` command: that command carries no input. When input is present, the transaction stores it in a provisional AgentTurn `awaiting_thread`; after the Bridge `thread/start` result/event binds `sourceThreadRef`, projector creates the one follow-on `turn.start` command. `PATCH /threads/:thread_id` persists only DEEIX labels, share policy and `is_pinned` directly in cloud metadata in the same idempotent transaction; pin has no app-server call. The separate idempotent `PATCH /threads/:thread_id/name` accepts only bounded normalized `name`, resolves `sourceThreadRef`, and queues `thread.rename`; Bridge resolves raw thread ID/cwd for `thread/name/set`, and provider result/event updates title. The separate idempotent `PATCH /threads/:thread_id/provider-metadata` validates only generated pinned `ThreadMetadataPatch.gitInfo`: optional nullable `sha`, `branch`, and `originUrl`; it resolves `sourceThreadRef` and queues `thread.metadata.update`, rejecting arbitrary JSON and DEEIX labels/share/pin fields. Provider result/event updates the Git projection. `fork`, archive, unarchive and delete queue typed lifecycle commands. UI layout preference belongs to user settings. Browser event replay is always thread-scoped.

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

Enrollment code, challenge nonce and ordinary `connection` token are issued as deterministic Base64URL bearers using Go standard-library HMAC-SHA-256 over active derivation key version, purpose/domain separator, immutable random issuance `public_id`, `user_id`/device/credential version, expiry and normalized issuance fields. Database rows store only SHA-256 `token_hash`, `derivation_key_version` and immutable issuance fields; raw bearer, seed and key material are absent. Ordinary connection tokens remain short-lived and one-use. A `settlement` token is the only second kind: while device is `revoking` and delivery-started work remains, a fresh device-key-signed challenge may issue a short-lived one-use bearer derived/hashed with purpose/domain `bridge-settlement`, device, current credential version, immutable issuance ID/expiry and immutable `settlement_upper_server_seq` equal to the highest already delivery-started command eligible at issuance. Consumption is one conditional update: `consumed_at IS NULL AND expires_at > now`. Upgrade validates the device key/token, marks the connection credential consumed, selects `deeix.bridge.v1`, and creates ordinary or settlement-scoped in-memory epoch. Final revoke/credential rotation invalidates settlement tokens. This subprotocol transport works with Node 24 native `WebSocket` without placing token text in URL logs.

A settlement-scoped epoch permits only hello/welcome, resend/ack for already delivery-started command rows at or below `settlement_upper_server_seq`, result/error/recovery/provider frames tied to that settlement set, and existing transfer receipt/data required to finish those commands. It rejects new User-scoped commands, resource refresh/mutation, new transfer claims, unrelated provider frames and every command above the bound. A repeat disconnect may obtain another one-use settlement token through a fresh signed challenge while unsettled work remains; ordinary credential issuance stays blocked.

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

type CommandDelivery =
  | { kind: "execute"; command: Exclude<AgentCommand, { kind: "transfer.execute" }> }
  | { kind: "execute"; command: Extract<AgentCommand, { kind: "transfer.execute" }>; transferBearer: string }
  | { kind: "tombstone"; commandId: string; reason: "cancelled" | "expired" | "failed_terminal" };

type BridgeCommandEnvelope = Omit<BridgeEnvelope, "type" | "serverSeq" | "payload"> & {
  type: "command";
  serverSeq: number;
  payload: CommandDelivery;
};
```

`hello`, `welcome`, `ack`, `ping`, and `pong` are control frames. They carry cursors but no durable `server_seq`. Every non-control Bridge-to-server frame has a `bridgeSeq`, including device/profile snapshots, command result/error, interaction notifications and provider events. Every command frame has a `serverSeq` allocated by the server. For `transfer.execute`, the dispatcher rebuilds the deterministic HMAC bearer from immutable ticket fields, verifies its SHA-256 against `token_hash`, and attaches it only as `transferBearer` in this ephemeral WSS execute envelope. Bridge validates that envelope and writes a sanitized incoming command record containing `serverSeq`, command ID and ticket/public/source refs, never the bearer. The persisted command payload, browser response and cloud response/body cache contain the ticket ref and allowlisted public/source refs only.

`hello.ackServerSeq` is Bridge's contiguous durable receipt cursor for cloud-to-Bridge commands. After credential, epoch, range and contiguity validation against allocated command rows, the server may advance cloud `last_acked_server_seq` and mark covered commands acked. `hello.ackBridgeSeq` is only Bridge's last observed cloud acknowledgement for diagnostics/reconciliation; it never advances cloud `last_acked_bridge_seq`. `welcome` returns cloud `last_acked_bridge_seq` as the authoritative upstream cursor, and Bridge resends WAL frames strictly above that value. Thus a stale or higher Bridge-reported `ackBridgeSeq` cannot suppress resend or cause cloud cursor advancement; only the contiguous staged-frame projection transaction advances `last_acked_bridge_seq`.

## 4. Durable sequencing

### 4.0 PostgreSQL transaction rules

Agent projections and commands use PostgreSQL with pgvector as the only server Gorm database. Write transactions use row locks, upserts, unique constraints and partial indexes for device `server_seq`, thread `thread_seq`, bridge-frame dedupe/projection and HTTP idempotency claim/replay. They never perform socket, provider, object or other network I/O. PostgreSQL integration tests cover crash rollback with no duplicate sequence, command or event. Redis is required for cache/wake behavior; it never replaces database replay as the authoritative history.

### 4.1 Upstream Bridge to server

Bridge allocates monotonically increasing `bridge_seq` in its private durable WAL store and writes the full durable envelope before attempting WSS send. This embedded local crash/reconnect store is not a server deployment database. Gateway first inserts/deduplicates the frame as staged inbox evidence in `agent_events` with unique `(device_id, bridge_seq)`; receipt/inbox storage is distinct from projection. `last_acked_bridge_seq` means the largest contiguous applied/projected cursor, never the largest merely persisted cursor. If `bridgeSeq > last_acked_bridge_seq + 1`, the PostgreSQL transaction commits the frame as staged and returns only the current contiguous ack: it does not project the frame or allocate `thread_seq`. When the next contiguous frame is present, `ApplyBridgeFrame` applies it and drains a bounded contiguous staged run in `bridge_seq` order, atomically per frame or bounded PostgreSQL transaction; each applied frame updates projection, any `thread_seq`, and the contiguous cursor. Staged/applied duplicates are no-ops. A post-commit Redis wake/worker may continue a bounded drain, and database replay remains authoritative.

`agent_events` contains both durable inbox evidence and browser event projection. A frame with a resolved thread receives `thread_seq`; device/profile/command frames have NULL `thread_id` and NULL `thread_seq`. Browser replay filters to its thread and non-NULL sequence, so device activity has no accidental placement in a thread timeline.

### 4.2 Downstream server to Bridge

Command creation serializes the `agent_devices.next_server_seq` read-update and `agent_commands` insert with the PostgreSQL device row lock. It commits the command with its aggregate projection. `next_server_seq` starts at 1 and always represents the next value available for allocation.

Command dispatcher selects the smallest unacknowledged row by `server_seq > last_acked_server_seq AND acked_at IS NULL`. Before WSS write it durably records the first delivery attempt (`delivery_started_at`, attempt metadata and `dispatched`); that first attempt, not receipt ack, transfers terminal ownership to Bridge. Only a queued row with no delivery attempt may become `cancelled`, `expired` or `failed_terminal` and be sent as `CommandDelivery(kind=tombstone)` with the same command ID/server sequence/reason. An explicit transport failure proven before write acceptance may requeue only through the same serialized CAS that clears nullable `delivery_started_at` while retaining attempt/audit metadata; an ambiguous write/connection loss retains that marker and resends with the same command ID/sequence for Bridge WAL/cache/recovery dedup. Once execute delivery may have reached Bridge, cloud expiry/cancel/revoke neither terminalizes the command or linked responding interaction nor replaces it with a tombstone. For `transfer.execute`, Bridge does not ack on envelope receipt: it first persists the sanitized record, claims the ticket with the in-memory bearer, persists the returned non-secret receipt, zeroes bearer, then advances contiguous `ackServerSeq`.

Bridge persists every incoming typed command or tombstone to its private durable WAL store before action. Incoming command WAL state is `received`, `invocation_started`, or `terminal_cached`; `invocation_started` stores the method-specific recovery data and precondition required by the typed recovery registry, while `terminal_cached` stores normalized result/error. For `transfer.execute`, the received record is the sanitized serverSeq/command/ticket-ref form defined above and excludes the bearer; normal commands retain their typed payload. A tombstone advances contiguous ack without provider execution. For an expiry-bearing normal execute command, Bridge checks `expiresAt` immediately before invocation and during recovery: if delivery occurred but invocation has not begun before expiry, it caches/emits normalized `expired` without provider call; after invocation began or provider accepted, deadline does not manufacture a cloud terminal and typed recovery settles result/failure/`outcome_unknown`. Otherwise Bridge checks the command-ID result cache: a hit re-emits the stored result/error with no provider call; a miss persists `invocation_started`, resolves source refs, calls `ProviderAdapter`, persists normalized result/error as `terminal_cached`, then emits the durable up-frame. Ordinary command acknowledgement is receipt/WAL durability rather than provider completion. The Bridge reports contiguous `ackServerSeq` in hello and ack controls. Server advances `last_acked_server_seq` only to the largest contiguous value and writes `acked_at` on covered command rows; acknowledgement does not overwrite delivery-started or terminal business state. It resends every unacknowledged row above that cursor after reconnect. A result/error settles a delivery-started normal command as `completed`, normalized failure, or `failed_terminal` with `outcome_unknown`; ack and completion are separate timestamps. Device revoke leaves delivery-started settlement to Bridge; sequence values remain allocated and never re-enter dispatch.

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

Unique `(device_id, server_seq)` and partial unique `(turn_id) WHERE operation='turn.start' AND initial_thread_create=true`; indexes `(device_id, server_seq)`, `(idempotency_record_id)`, `(thread_id, created_at)`, `(expires_at)`. `operation` is an allowlisted AgentCommand discriminant. `payload_json` contains typed input, public/source refs and allowlisted `expiresAt` where required; a `transfer.execute` row contains only `transferTicketRef` and allowlisted device/workspace/thread public/source refs. It never contains a raw provider ID or transfer bearer. Business statuses are `queued`, `dispatched`, `completed`, `cancelled`, `expired`, `failed_retryable`, `failed_terminal`; `delivery_started_at` is the durable first-attempt ownership boundary and `acked_at` is independent Bridge-receipt evidence. Only terminal-before-delivery rows become same-sequence tombstones. A delivery-started row is settled only by the first matching Bridge terminal result/error/recovery: an identical replay is a no-op; conflicting second terminal, or any terminal frame for a cloud-terminal pre-delivery tombstone, is rejected/audited and cannot regress aggregates. If a pre-delivery `interaction.respond` command terminalizes, it atomically writes the linked responding interaction terminal status/code/time; delivery-started interaction response remains responding until its Bridge settlement. Retention preserves result/error for the Bridge command-ID cache window, then redacts payload/result as required while retaining audit summary.

### 5.10 `agent_idempotency_records`

```text
id, public_id, user_id, operation, idempotency_key, request_hash,
response_status, response_skeleton_json, secret_kind NULL, secret_ref NULL, command_id NULL,
created_at, expires_at
```

Unique `(user_id, operation, idempotency_key)`; indexes `(expires_at)`, `(command_id)` and `(secret_ref)`. `request_hash` is calculated from normalized validated request content. `response_skeleton_json` excludes bearer token material. For enrollment/challenge/connection issuance, `secret_kind` and `secret_ref` point to credential public ID; initial response and same-hash replay rebuild bearer from the issuance row, verify its SHA-256 against `token_hash`, then attach it to the response skeleton. Transfer issuance records the ticket public ID but returns no bearer; dispatcher reconstruction is WSS-only. This works after process restart and even after credential consumption: replay returns the same credential bearer while consume CAS still permits only one use. Retention expires response skeleton after the retry window while retaining minimal audit evidence. Every HTTP mutation transaction claims/inserts the record, changes aggregate state, creates/links command when needed, and stores response before commit. Labels/share policy mutations follow the same transaction. Concurrent same-key requests lock/read the record rather than creating a second command.

### 5.11 `agent_artifacts`, `agent_thread_shares`, and `agent_transfer_tickets`

```text
agent_artifacts(id, public_id, user_id, thread_id, turn_id NULL, item_id NULL,
  workspace_id, file_ref, object_key NULL, name, mime_type, byte_size, sha256,
  status, display_json, expires_at NULL, created_at, updated_at, deleted_at)

agent_thread_shares(id, public_id, user_id, thread_id, snapshot_json,
  status, expires_at NULL, revoked_at NULL, created_at, updated_at)

agent_transfer_tickets(id, public_id, user_id, device_id, workspace_id,
  artifact_id NULL, direction, token_hash, derivation_key_version, object_key NULL, file_ref NULL,
  target_ref NULL, sha256, byte_size, mime_type, status, expires_at,
  claim_command_id NULL, claim_receipt_ref NULL, consumed_at NULL, created_at, updated_at)
```

Artifact unique key is `(workspace_id, file_ref)` while the file ref is active; index `(thread_id, created_at)`. Share unique `public_id`; index `(thread_id, status)`. Transfer ticket unique keys are `public_id` and `token_hash`; index `(workspace_id, status, expires_at)`. Token text never enters the database; only `token_hash` is stored. Artifacts use a local signed file ref until a transfer creates `object_key`; share stores only a redacted snapshot.

Transfer bearer uses the same HMAC-SHA-256 derivation scheme with ticket purpose/domain, ticket public ID, `user_id`/device/workspace/direction and expiry; DB stores only `token_hash`, `derivation_key_version` and immutable ticket fields. On every transfer command send or replay, dispatcher rebuilds the bearer, verifies `token_hash`, and puts it only in the ephemeral WSS execute envelope. Bridge validates the envelope and first writes a sanitized incoming record with `serverSeq`, command ID and ticket/public/source refs, with no bearer. It then calls the separately authenticated claim route using its device identity, command ID, ticket ref and in-memory bearer. The typed claim validates user/device/workspace/direction and CAS-updates `status=issued AND consumed_at IS NULL AND expires_at > now` to `consuming`, recording `claim_command_id` and returning durable `claim_receipt_ref` bound to that User, device, command and ticket. A retry of that same device and command returns the same receipt; another command or device does not acquire the claim. Bridge persists this non-secret receipt before advancing/acking `serverSeq`, then zeroes bearer. Authenticated directional streaming uses Bridge device authentication plus the persisted receipt with `PUT .../data` for upload and `GET .../data` for download, never the transfer bearer, while that claim is `consuming`. Before and during streaming, Bridge and server validate User ownership, device, workspace, direction, SHA-256, byte size, MIME type, canonical root and file ref. A verified terminal transfer CAS-moves `consuming -> consumed` with `consumed_at`; hash mismatch or transport failure CAS-moves it to `failed`. A crash before durable claim receipt/ack leaves the command unacknowledged, so server redelivery re-derives the same bearer; a crash after durable receipt/ack resumes the transfer from the receipt with no bearer and no second consumption. Bridge keeps bearer out of durable WAL/result-cache payloads and zeroes it after receipt persistence; only redacted receipt metadata remains after terminal result. Retention marks elapsed issued/consuming rows `expired` and cleans temporary staging/object data. Thread deletion schedules artifact ticket expiry/object cleanup and share revocation, then follows aggregate retention. Canonical local path is never stored.

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

Business state flow for normal commands is `queued -> dispatched -> completed`; the durable first delivery attempt is recorded before WSS write. A transport failure proven before write acceptance may requeue only through the same serialized CAS that clears nullable `delivery_started_at` while retaining attempt/audit metadata; an ambiguous write retains the marker, stays delivery-started and resends with the same ID/sequence. Cloud-side expiry/cancel/revoke can produce `cancelled`, `expired` or `failed_terminal` only before delivery starts, yielding a same-sequence tombstone; after delivery starts, Bridge terminal result/error/recovery owns settlement and may legitimately emit local normalized `expired` before invocation. `acked_at` remains Bridge-receipt evidence, not provider completion. Claim/send uses compare-and-set on the smallest `server_seq > last_acked_server_seq` with `acked_at IS NULL`. For `transfer.execute`, this claim also rebuilds and hash-verifies the bearer immediately before WSS send; Bridge acknowledges only after its sanitized command record and claim receipt are durable, then uses the receipt for resumable directional data transfer. In the same pre-delivery terminal-command transaction, a linked `interaction.respond` CAS-updates `responding` to `cancelled`, `expired`, or `failed` with normalized code/time; a delivery-started response leaves it responding until Bridge settles it. Revoke enters `revoking`, blocks new user work and ordinary credential issuance, terminalizes only never-delivered work and pending interactions, and permits only the bounded `settlement` credential/epoch for delivery-started work. It finalizes `revoked`, rotates credential and closes connection only after settlement; offline unsettled devices stay `revoking`.

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
