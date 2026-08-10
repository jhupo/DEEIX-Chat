# 源码迁移边界

## 1. Source baseline findings

| Source | Current behavior | Design consequence |
| --- | --- | --- |
| `frontend/next.config.ts:4`, `server_test.go` static assertions | `output: "export"` without `trailingSlash`; page files are `<route>.html` | static host rewrites `/agent` to `/agent.html` while preserving query `thread_id` |
| `backend/internal/app/app.go:261+` | conversation construction couples channel, LLM, media, MCP, RAG, processing and billing | Agent receives a separate module/service wiring; Chat aggregate and UI remain, while 08 replaces local billing authorization/settlement with Sub2 key-bound gateway execution |
| `backend/internal/transport/http/server.go:52-67,139-174` | server owns module fields and protected route registration | add Agent HTTP/browser module fields and routes there; register Bridge credential/WSS and transfer routes through a separate Bridge authenticator |
| `backend/internal/infra/llm/adapter.go:35+` | one-way `Generate`, `GenerateStream`, `ListModels` | app-server sits behind Local Bridge, not LLM adapter |
| `repository_project.go:137+` | ConversationProject delete transaction owns Conversation reassignment/deletion | Agent threads have independent workspace/device grouping and no project relation |
| `backend/internal/infra/persistence/postgres/user/repository.go:882-1075` | legacy `DeleteAccountHard` owns final user cleanup | replacement entry point only: retire it in favor of Principal aggregate deletion, without changing ConversationProject cleanup |
| `application/auth/service.go:888-953`, `application/user/service.go:423-431`, `application/admin/service.go:1220` | legacy self/admin deletion entry points converge on repository deletion | replace/retire these user-domain paths so all deletion reaches the Principal aggregate service; no self-delete-only file cleanup owns Agent data |
| `application/conversation/generation_stream.go` | Redis retained stream is run-scoped | Agent copies cursor/replay/overflow pattern with configured-database thread events |
| `project-layout.tsx` | chat session and sidebar providers wrap project layout | `/chat` retains providers; Agent reducer is page-scoped |
| `frontend/features/layouts/components/navigation/app-sidebar.tsx`, `NAVIGATION_ITEMS`, `NavMainItem` | new-chat action/shortcut and conversation sections are established | retain `newChat`; add Agent link and `NavAgentThreads` seam |
| `packages/api-contract/README.md` | Swagger document generates TypeScript contracts | Agent DTO/annotation precedes `pnpm api:generate` |

## 2. Retain without modification

| Area | Retained source and behavior |
| --- | --- |
| Chat API and schema | `/api/v1/conversations/*`, Conversation/Message/Run, share/export, project transaction behavior |
| Chat execution | `application/conversation`, `infra/llm`, server MCP, RAG, DB Skill, media, chat reducers, `/chat` components and conversation routes; 08 replaces only local billing authorization/settlement with Sub2 key-bound gateway execution |
| Platform | audit writer, PostgreSQL + pgvector Gorm database, required Redis cache/wake behavior, object storage and static frontend deployment; legacy Bearer JWT/user-role middleware is retired for Browser routes in favor of Principal-session cookie middleware |
| Frontend chat shell | `/chat`, `SidebarConversationsProvider`, `ChatSessionProvider`, existing new-chat callback and shortcut |
| Contract pipeline | Go Swagger DTO/annotations, `pnpm api:generate`, `pnpm api:check`, generated `@deeix/api-contract` |

No conversation table gains Agent columns. No chat endpoint accepts Agent payloads. No shared chat/Agent reducer, provider interface, send endpoint or persistence mapping is introduced.

## 3. Backend additions

### 3.1 Target directories

```text
backend/internal/domain/agent/
  types.go, commands.go, events.go, errors.go
backend/internal/application/agent/
  service.go, service_thread.go, service_turn.go, service_interaction.go,
  dispatcher.go, projector.go, retention.go
backend/internal/infra/persistence/agent/
  repository.go, repository_command.go, repository_event.go, repository_device.go
backend/internal/infra/persistence/models/
  agent.go or agent_*.go by aggregate
backend/internal/infra/persistence/schema/
  schema.go (Models() registration)
backend/internal/transport/http/agent/
  module.go, router.go, handler_*.go, dto_*.go
backend/internal/transport/ws/agentbridge/
  gateway.go, hub.go, protocol.go
```

These are new Agent domain directories. Existing conversation packages remain chat-only. `app.go` creates Agent repository, service, HTTP module, Bridge gateway and Hub after PostgreSQL/Redis/auth/audit/object-storage readiness. Agent service receives repository, audit writer, object-store ticket service and Hub interface; it does not receive conversation.Service, LLM client or server-MCP client. `backend/internal/transport/http/server.go:52-67,139-174` gains the smallest module fields plus protected Agent/browser route registration; it separately registers Bridge credential/WSS/transfer routes behind the Bridge authenticator rather than browser session middleware.

### 3.2 Persistence and Principal deletion

Add Gorm models under `backend/internal/infra/persistence/models/` and register each in `backend/internal/infra/persistence/schema/schema.go` `Models()`. Add migrations for devices, credentials, profiles, workspaces, threads, turns, items, interactions, events, commands, idempotency records, artifacts, thread shares, transfer tickets and `agent_cleanup_jobs` from [03-protocol-and-data-model.md](./03-protocol-and-data-model.md). Migration defines partial source-ref uniques, initial-turn command uniqueness, thread event sequence key, device command sequence, HTTP idempotency record keys, one-time token hash/key-version fields, and cleanup-job dedupe/backoff fields. At the 08 clean-slate cutover, all these ownership columns and public-ID lookup constraints use `principal_id`, not legacy local `user_id`. PostgreSQL with pgvector remains the only server Gorm database. Row locks, upserts, unique constraints and partial indexes serialize device `server_seq`, thread `thread_seq`, idempotency claim/replay, bridge-frame projection and aggregate mutation/command/response commit. Transactions never perform socket/provider/object/network I/O.

Target `PrincipalRepository.DeleteAggregate` is the immediate deletion boundary. Legacy `application/auth|user|admin` paths are replacement/retirement entry points only; handlers read the authenticated Principal/session and pass `principal_id`, never a legacy user aggregate. The Principal transaction invalidates access, deletes cloud aggregates, accepts no late frames and does not claim to cancel already local provider activity. It revokes device access, transfer tickets and shares, inserts deduped `agent_cleanup_jobs` for eligible object/temp refs, and prevents further command delivery. Before deletes it clears `agent_threads.last_turn_id`, `agent_transfer_tickets.claim_command_id`, and nullable command/idempotency cross-links. Its FK-safe delete order is `agent_events`; transfer tickets, shares, artifacts; commands/idempotency records; interactions; items; turns; threads; workspaces/profiles; credentials/devices; then Principal aggregates. Cleanup jobs remain because they have no Principal FK. Cleanup repository API uses bounded `FOR UPDATE SKIP LOCKED` lease claims with lease-token CAS/backoff and commits before object I/O. It does not enter `conversation/repository_project.go` or alter ConversationProject behavior. Tests cover Principal aggregate deletion crash/retry, no orphaned Agent rows/tickets/credentials, eligible blob cleanup for every deletion entry point, and PostgreSQL concurrent/reclaimed lease claims.

### 3.3 HTTP and Swagger

Agent handler follows current Gin module style:

1. DTO request binds and validates bounded IDs/enums/payload size.
2. handler reads authenticated Principal/session from existing middleware.
3. application service performs ownership and state validation.
4. response uses current response envelope conventions and Swagger annotations.
5. generator refreshes `backend/docs` and `packages/api-contract/src/types.generated.ts` through `pnpm api:generate`.

Handlers expose typed route groups from the protocol document. Standard Chat/billing/Agent/Admin Browser routes use new Principal-session cookie middleware: it reads only the opaque Secure `HttpOnly` DEEIX cookie, hashes and finds `principal_sessions`, validates expiry/revocation and active Principal, sets trusted `principal_id`/`principal_session_id`/DEEIX role, and rejects legacy Browser Bearer. Unsafe protected routes apply exact Origin, Fetch Metadata and session-CSRF checks before handlers. Bridge enrollment/WSS/transfer and deletion receipt retain separately narrow verifiers; Sub2 bearer stays server-to-server. Contract tests cover cookie/session/role boundaries and run `pnpm api:check` so Go, Swagger and generated TypeScript remain synchronized.

### 3.4 Dispatcher, projector, and Hub

`application/agent/dispatcher.go` owns ordered per-device claims by `server_seq > last_acked_server_seq`, retry/backoff, tombstone/no-op delivery and status timestamps. It durably records first delivery attempt before WSS write; only never-delivered work may terminalize into a tombstone, while delivery-started work is settled by Bridge result/error/recovery. HTTP handlers normalize/hash after pure DTO validation, then repository transaction claims idempotency record before mutable ownership/state checks; that same transaction allocates `server_seq`, mutates aggregate and persists first response. `projector.go` owns `ApplyBridgeFrame`, durable deduplication, `thread_seq` allocation and post-commit browser notifier. PostgreSQL repositories use row locks/upserts/unique constraints for these atomic operations. `transport/ws/agentbridge/hub.go` owns only live connection references and dispatch wake-up calls.

Required Redis supplies cache/wake behavior through a tiny notifier adapter patterned after conversation replay publication; PostgreSQL replay is the source of truth. The durable command queue is `agent_commands`; `agent_cleanup_jobs` is the separate durable post-commit object/temp cleanup outbox, not a command-intent queue or cache key. Multi-instance socket ownership is later work.

## 4. Bridge package

Add a TypeScript workspace package:

```text
packages/agent-bridge/
  src/index.ts
  src/config/workspace-registry.ts
  src/wal/durable-wal-store.ts
  src/transport/wss-client.ts
  src/protocol/bridge-envelope.ts
  src/providers/provider-adapter.ts
  src/providers/codex/codex-adapter.ts
  src/providers/codex/method-registry.ts
  src/providers/codex/event-mapper.ts
  src/commands/resolve-provider-command.ts
  src/resources/redaction.ts
  scripts/check-codex-app-server-lock.mjs
```

Bridge durable WAL store records cover outgoing frames, incoming commands, result cache, runtime cursors and durable raw mapping `(profile_ref, source_kind, source_ref) -> raw provider ID`. This private embedded local store is not a server deployment database. Incoming commands record `received`, `invocation_started` with typed method-specific recovery data/precondition, or `terminal_cached` with normalized result/error. Before every provider call, Bridge persists the invocation marker. On startup it initializes provider, ingests/reconciles provider state/events and source mappings, then drains incoming commands by `server_seq`: tombstone has no provider call, cached terminal re-emits, received-without-invocation executes, and invocation-started-without-terminal is handled by one typed per-operation recovery registry. That registry proves postcondition/source binding and synthesizes a cached result, proves non-application and permits one execution, or persists/emits `outcome_unknown` for inspection/reconciliation; it never blindly re-executes an indeterminate invocation. `thread.create` and the follow-on initial `turn.start` reconcile source bindings through the existing cloud conditional transition/partial unique before recovery can create work, so ambiguous acceptance creates no duplicate provider thread/turn. `resolve-provider-command.ts` is the only location that converts cloud public/source refs to canonical cwd/raw provider IDs. It persists a source mapping before any up-frame first references a raw ID. Serialized thread/turn/interaction commands retain applicable source refs, while `transfer.execute` retains only `transferTicketRef` and allowlisted public/source refs; its bearer stays out of WAL payloads and is re-derived only for WSS send/replay. Codex adapter consumes generated app-server schema. A later Claude implementation adds `providers/claude/*`, capability manifest and mapper; cloud domain/API tables do not gain a provider interface.

## 5. Frontend changes

### 5.1 Route and data client

```text
frontend/app/(project)/agent/page.tsx
frontend/features/agent/api/
frontend/features/agent/model/
frontend/features/agent/components/
frontend/features/layouts/components/navigation/nav-agent-threads.tsx
```

`page.tsx` reads `thread_id` via search params and supports `/agent` plus `/agent?thread_id=<public_id>`. It imports generated Agent API contract types. Dynamic thread route directories are absent. `features/agent/model` owns a reducer and replay client only while Agent page is mounted.

### 5.2 Sidebar seam

Keep `NAVIGATION_ITEMS`, `NavMainItem`, `newChat`, callback and keyboard plumbing. Add a primary `/agent` navigation item and `NavAgentThreads` beside `NavProjects`/`NavRecents`; it renders Agent device/workspace thread data and uses `/agent?thread_id=<public_id>` links. `AppSidebar` gains the smallest typed data/callback seam needed to show this section. Chat providers continue to own current sidebar conversations.

### 5.3 Files, audit and billing

Reuse existing object-storage upload/download and audit infrastructure through Agent ticket/audit adapters. Agent events produce audit records for command class, result, device, workspace and interaction decision with redaction. Agent billing is absent from the first command path until a separate product policy defines provider-local work accounting; it does not piggyback on conversation Run fields. The Chat page, conversation routes and billing UI remain, but 08 replaces their local financial authorization/settlement implementation with the concrete Sub2 BFF and key-bound gateway execution.

## 6. Full profile consolidation before Agent rollout

Status: completed. The repository now has one canonical Full profile with application, PostgreSQL + pgvector, and Redis health readiness; server persistence tests run against PostgreSQL. The legacy names below are retained only as historical migration records.

| Historical source | Completed action |
| --- | --- |
| `docker-compose.sqlite.yml` | Deleted. |
| `docker-compose.yml` | Deleted with the app-only external-service profile. |
| `docker-compose.full.yml` | Promoted/renamed to the sole canonical `compose.yaml`. |
| `config.sqlite.example.yaml` | Deleted. |
| `config.full.example.yaml` | Merged into sole `config.example.yaml`, then deleted. |
| `backend/internal/infra/persistence/sqlite/` and `backend/internal/infra/persistence/sqlitevec/` | Deleted after callers moved to direct PostgreSQL/pgvector paths. |
| `backend/internal/infra/cache/memory/` | Deleted; `backend/internal/infra/cache/redis/` remains. |
| `backend/internal/app/infrastructure.go` | Constructs PostgreSQL and Redis directly, requires their health at startup, and has no driver switch/fallback. |
| `backend/internal/app/app.go` | Uses direct Redis-backed settings/provider-auth/channel/conversation/rate-limiter/health construction and requires PostgreSQL/Redis readiness. `InvalidateMemoryCache` remains only as unrelated conversation-memory domain terminology. |
| `backend/internal/infra/config/config.go` | Removed SQLite and memory YAML/env/runtime fields, including `DATABASE_DRIVER`, `SQLITE_*`, `CACHE_DRIVER` choices, and memory aliases; startup requires PostgreSQL and Redis. |
| `backend/go.mod` and `backend/go.sum` | Removed SQLite dependencies after imports were removed. |
| `backend/internal/infra/persistence/postgres/{billing,conversation,memory}/repository.go` | Removed SQLite/sqlite-vec branches and retains direct PostgreSQL/pgvector behavior. |
| `backend/internal/infra/openwebui/loader.go` | Accepts PostgreSQL DSNs only and rejects legacy SQLite, implicit file/path, and unsupported DSNs before opening a connection. |

Tests had two different migration actions; behavior coverage was preserved when its previous harness was SQLite:

| Historical tests | Completed action |
| --- | --- |
| Implementation-only tests under `persistence/sqlite`, `persistence/sqlitevec`, and `cache/memory`; `conversation/repository_sqlite_vector_test.go`; `memory/repository_sqlite_vector_test.go` | Deleted with the removed implementation. |
| `persistence/schema/schema_test.go`; `application/conversation/service_trace_integration_test.go`; `persistence/postgres/conversation/repository_test.go`; `logcleanup/repository_test.go`; `skill/repository_test.go`; `announcement/billing/channel/mcp/user/repository_sqlite_test.go` files | Ported domain/repository semantics to a shared PostgreSQL integration harness and renamed SQLite-oriented test names. |
| `backend/internal/application/auth/provider_bridge_test.go`; `backend/internal/application/channel/service_routing_test.go`; `backend/internal/application/channel/service_model_update_test.go` | Ported with focused fakes, preserving provider bridge, routing, invalidation, and update assertions. |
| Existing PostgreSQL integration suites | Retained with pgvector/Redis readiness coverage. |

Root, backend, and current product documentation now describe the implemented sole-Full deployment. [06-full-deployment-and-online-update.md](./06-full-deployment-and-online-update.md) defines the release/update path.

## 7. Ordered implementation batches

### Batch 1: domain and schema

Create Agent Gorm models, migration, domain IDs/states/errors, repository transactions and PostgreSQL integration tests for public-ID ownership, device sequence allocation, provisional source-ref partial unique indexes, serialized command source-ref requirements, two-phase initial thread/turn command guard across duplicate event/restart and thread-create terminalization, turn/thread interaction scope, response-command terminalization, HTTP idempotency same-hash replay/conflicting-hash conflict, and enrollment/challenge/connection/transfer replay-after-restart, different-hash conflict, consume-once, hash mismatch/expiry/repeated claim coverage. Add settlement credential field/derivation tests for wrong device/status/purpose, expired/consumed token, out-of-range command/frame, reconnect reissue through fresh challenge, rate limits, and final revoke/credential-rotation invalidation. Cover expiry before first delivery producing tombstone/no provider; delivery/ack before invocation then deadline producing Bridge-local expired/no provider; provider acceptance before deadline followed by cloud deadline producing Bridge recovery/result/`outcome_unknown` with no interaction/provider divergence; ambiguous socket write resend with same command ID; revoke with delivery-started work entering `revoking` settlement then `revoked`; concurrent `server_seq`/`thread_seq`, idempotency, bridge-frame projector, replay/restart, crash rollback and no duplicated sequence/command/event. Include N+1-before-N staged frame receipt, concurrent delivery and restart, proving no `thread_seq` before N projects and the bounded contiguous drain restores order. Add `transfer.execute` WSS envelope tests proving bearer re-derivation/hash verification, sanitized incoming record, receipt-before-ack, no bearer in `payload_json`/response cache, same-command claim retry and terminal redaction. Cover crash before durable receipt/ack (server redelivery and same bearer derivation) and crash after receipt/ack (receipt-only resume without second claim/consumption). Keep old derivation keys until every issuance expiry plus idempotency replay window has passed; key material remains server secret/KMS configuration rather than database data.

### Batch 2: HTTP contract and thread control plane

Add DTOs, Swagger annotations, Gin module/routes, thread/workspace/device services and command insertion. `POST /threads` stores optional initial input in an awaiting turn but allocates only `thread.create`; projector later creates one guarded `turn.start`. Add idempotent `PATCH /threads/:thread_id/name`, accepting bounded normalized `name` and queuing typed `thread.rename`; fixtures cover same-key replay, source-ref/raw-thread/cwd resolution, `thread/name/set`, and title projection after provider result. Add separate idempotent `PATCH /threads/:thread_id/provider-metadata`, validating generated `gitInfo` only with optional nullable `sha`, `branch`, and `originUrl`, and queuing typed `thread.metadata.update`; fixtures cover same-key replay, source-ref resolution and Git projection after provider result. `PATCH /threads/:thread_id` stays DEEIX labels/share/is_pinned only. Add the atomic aggregate snapshot DTO carrying thread, included turns/items/interactions and `snapshotSeq`. Run `pnpm api:generate` and `pnpm api:check`. Browser API client can list projection and create queued thread/turn intent.

Keep `thread/section/move` and `threadSection/*` separately locked extensions with no first-release control.

### Batch 3: WSS durability

Implement credential exchange, `Sec-WebSocket-Protocol` upgrade, Hub, dispatcher, hello cursor recovery, Bridge frame insertion, staged-inbox projector, required Redis notifier and command state tests. Cover PostgreSQL replay/restart, concurrent projector/idempotency/sequence behavior, duplicate frame, N+1-before-N concurrent delivery/restart with only contiguous applied `ackBridgeSeq`, stale/higher hello `ackBridgeSeq` that cannot advance cloud cursor or suppress WAL resend, ordered resend, a middle `server_seq` that expires/cancels before first delivery then delivers tombstone on reconnect before its suffix, ambiguous write resend with unchanged command ID/sequence, settlement epoch restricting below-bound already-delivery-started rows/related frames/transfer receipt data and rejecting all new or unrelated work, fresh-challenge settlement reissue, final revocation invalidation, snapshot events committed between subscribe/snapshot/replay, and `interaction.respond` pre-delivery expiry versus delivery-started Bridge terminalization without regression from late provider events.

### Batch 4: Codex Bridge

Implement Bridge durable WAL store, registry, generated app-server adapter, lifecycle/method registry, event/interaction mapper and fixture tests. Carry allowlisted expiry deadlines, check them before invocation and recovery, and emit Bridge-local `expired` without provider call when invocation never began. Cover typed `thread.metadata.update` `gitInfo` patch with optional nullable `sha`, `branch`, and `originUrl`, source-ref/raw-thread/cwd resolution, Git projection after provider result/event, and idempotent API fixture. Cover crash before invocation marker (one execution), after provider acceptance/before terminal cache (typed recovery with no blind replay), and cached terminal replay. Include initial `thread.create`/follow-on `turn.start` recovery proving source reconciliation and the cloud conditional transition/partial unique prevent duplicate provider thread/turn creation. Generate the stable schema, update [codex-app-server-v0.147.0.lock.json](./codex-app-server-v0.147.0.lock.json), then run `packages/agent-bridge/scripts/check-codex-app-server-lock.mjs` with the lock and generated TypeScript/JSON directories. CI runs that generated-artifact checker after schema generation and fails on artifact SHA-256, recorded Git/blob evidence, union set/count or disposition drift. The lock's `self_check_command` remains a runnable JSON-invariant check only; it does not compare generated artifacts. Commit generated schema, lock, checker evidence and fixtures atomically. Include no-turn MCP elicitation from request through response to `serverRequest/resolved`, plus server-initiated `item/tool/requestUserInput` source-request correlation and typed response fixture. `tool/requestUserInput` is absent from this stable pin. Start with stable lifecycle/thread/turn/item/interaction/resource reads; gate a separately locked experimental surface by manifest.

Keep section-operation fixtures out of the mapped method: `thread/section/move` and `threadSection/*` remain separately locked extensions with no first-release control.

### Batch 5: Web workbench

Add static `/agent` launcher/workbench, `NavAgentThreads`, page-scoped reducer, atomic-snapshot/cursor replay, interactions, resource pages and accessibility scenarios. Keep `/chat` visual/state behavior unchanged.

### Batch 6: hardening

Add file transfer ticket claim/failed/expired cleanup, directional streaming, share snapshot redaction, `agent_cleanup_jobs` worker, retention workers, revoke flow, observability, load/overflow tests and end-to-end recovery tests. Test ticket hash mismatch, expiry, repeated consumption, reconnect claim resume, Bridge bearer redaction, PostgreSQL concurrent/reclaimed cleanup leases, cleanup-job idempotent delete, and `DeleteAccountHard` FK cleanup with eligible blob removal. PostgreSQL + pgvector and Redis readiness are mandatory before application startup. Production rollout enables one profile/device cohort first and expands by verified capability manifest.
