# 源码迁移边界

> 历史实施记录。当前代码已经采用统一 Conversation 执行链路；现行合同见 [01-architecture.md](./01-architecture.md) 与 [03-protocol-and-data-model.md](./03-protocol-and-data-model.md)。下列早期批次和路径不再作为实现要求。

## 1. Source baseline findings

| Source | Current behavior | Design consequence |
| --- | --- | --- |
| `frontend/next.config.ts:4`, `server_test.go` static assertions | `output: "export"` without `trailingSlash`; page files are `<route>.html` | static host rewrites `/agent` to `/agent.html` while preserving query `thread_id` |
| `backend/internal/app/app.go:261+` | conversation construction couples channel, LLM, media, MCP, RAG, processing and billing | Agent receives a separate module/service wiring; Chat aggregate and UI remain, while 08 replaces local billing authorization/settlement with Sub2 key-bound gateway execution |
| `backend/internal/transport/http/server.go:52-67,139-174` | server owns module fields and protected route registration | add Agent HTTP/browser module fields and routes there; register Bridge credential/WSS/artifact routes separately |
| `backend/internal/infra/llm/adapter.go:35+` | one-way `Generate`, `GenerateStream`, `ListModels` | app-server sits behind Local Bridge, not LLM adapter |
| `repository_project.go:137+` | ConversationProject delete transaction owns Conversation reassignment/deletion | Agent threads have independent workspace/device grouping and no project relation |
| `backend/internal/infra/persistence/postgres/user/repository.go:882-1075` | legacy `DeleteAccountHard` owns final user cleanup | replacement entry point only: retire it in favor of User aggregate deletion, without changing ConversationProject cleanup |
| `application/auth/service.go:888-953`, `application/user/service.go:423-431`, `application/admin/service.go:1220` | legacy self/admin deletion entry points converge on repository deletion | replace/retire these user-domain paths so all deletion reaches the User aggregate service; no self-delete-only file cleanup owns Agent data |
| `application/conversation/generation_stream.go` | Redis retained stream is run-scoped | Agent copies cursor/replay/overflow pattern with configured-database thread events |
| `project-layout.tsx` | chat session and sidebar providers wrap project layout | `/chat` retains providers; Agent reducer is page-scoped |
| `frontend/features/layouts/components/navigation/app-sidebar.tsx`, `NAVIGATION_ITEMS`, `NavMainItem` | new-chat action/shortcut and conversation sections are established | retain `newChat`; add Agent link and `NavAgentThreads` seam |
| `packages/api-contract/README.md` | Swagger document generates TypeScript contracts | Agent DTO/annotation precedes `pnpm api:generate` |

## 2. Retain without modification

| Area | Retained source and behavior |
| --- | --- |
| Chat API and schema | `/api/v1/conversations/*`, Conversation/Message/Run, share/export, project transaction behavior |
| Chat execution | `application/conversation`, `infra/llm`, server MCP, RAG, DB Skill, media, chat reducers, `/chat` components and conversation routes; 08 replaces only local billing authorization/settlement with Sub2 key-bound gateway execution |
| Platform | audit writer, PostgreSQL + pgvector Gorm database, required Redis cache/wake behavior, object storage and static frontend deployment; legacy Bearer JWT/user-role middleware is retired for Browser routes in favor of User-session cookie middleware |
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

These are new Agent domain directories. Existing conversation packages remain chat-only. `app.go` creates Agent repository, service, HTTP module and Bridge WSS after PostgreSQL/Redis/auth/file-store readiness. Agent service receives its repository and the existing file content store; it does not receive conversation.Service, LLM client or server-MCP client. Browser Agent routes use the current authenticated User middleware; Bridge credential/WSS/artifact content routes use their narrow credential or command-bound grant.

### 3.2 Persistence and User deletion

Agent Gorm models are registered in `schema.Models()`: devices, credentials, runtime profiles, workspaces, resource snapshots, threads, turns, items, interactions, events, commands, idempotency records and artifacts. Ownership columns use the existing identity `user_id`. PostgreSQL row locks, upserts and unique constraints serialize device `server_seq`, thread `thread_seq`, idempotency replay and bridge-frame projection. Transactions perform no socket/provider/file network I/O.

Account deletion must include the Agent aggregate in FK-safe order: events and bridge frames; commands/idempotency records; interactions/items/turns; threads; artifacts/resource snapshots/workspaces/profiles; credentials/devices. This remains a hardening batch and does not alter ConversationProject behavior.

### 3.3 HTTP and Swagger

Agent handler follows current Gin module style:

1. DTO request binds and validates bounded IDs/enums/payload size.
2. handler reads authenticated User/session from existing middleware.
3. application service performs ownership and state validation.
4. response uses current response envelope conventions and Swagger annotations.
5. generator refreshes `backend/docs` and `packages/api-contract/src/types.generated.ts` through `pnpm api:generate`.

Handlers expose typed route groups from the protocol document. Standard Agent Browser routes reuse current User authentication. Bridge enrollment and token routes validate device proof; WSS uses a one-use connection token; artifact content uses the command-bound HMAC grant. Contract checks run `pnpm api:check` so Go, Swagger and generated TypeScript remain synchronized.

### 3.4 Dispatcher, projector, and Hub

`application/agent/dispatcher.go` owns ordered per-device claims by `server_seq > last_acked_server_seq`, retry/backoff, tombstone/no-op delivery and status timestamps. It durably records first delivery attempt before WSS write; only never-delivered work may terminalize into a tombstone, while delivery-started work is settled by Bridge result/error/recovery. HTTP handlers normalize/hash after pure DTO validation, then repository transaction claims idempotency record before mutable ownership/state checks; that same transaction allocates `server_seq`, mutates aggregate and persists first response. `projector.go` owns `ApplyBridgeFrame`, durable deduplication, `thread_seq` allocation and post-commit browser notifier. PostgreSQL repositories use row locks/upserts/unique constraints for these atomic operations. `transport/ws/agentbridge/hub.go` owns only live connection references and dispatch wake-up calls.

Required Redis supplies cache/wake behavior through a tiny notifier adapter patterned after conversation replay publication; PostgreSQL replay is the source of truth. The durable command queue is `agent_commands`; `agent_cleanup_jobs` is the separate durable post-commit object/temp cleanup outbox, not a command-intent queue or cache key. Multi-instance socket ownership is later work.

## 4. Bridge package

Add a TypeScript workspace package:

```text
backend/internal/agentclient/
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

Bridge durable WAL covers outgoing frames, incoming commands, receipt readiness, result cache and `(profile_ref, source_kind, source_ref) -> raw provider ID`. Commands are persisted without artifact grants. `artifact-downloader.ts` verifies attachments before `command.receipt-ready`; `resolve-provider-command.ts` alone converts source refs, workspace refs and local attachments into raw provider IDs/canonical paths. Before every provider call the Bridge persists `invocation_started`; terminal results are cached before the durable up-frame. Codex adapter consumes the generated pinned app-server schema. A later provider adds only its adapter and mapper; Cloud tables keep the same provider-neutral contract.

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

Implemented: Agent models, ownership and idempotency transactions, device sequence allocation, initial thread/turn guard, event ordering/projection, interaction scope, artifact binding and PostgreSQL integration coverage. Bridge tests cover grant-free WAL records, hash/size validation, receipt-ready ACK ordering, cached terminal replay and unsafe crash recovery.

### Batch 2: HTTP contract and thread control plane

Add DTOs, Swagger annotations, Gin module/routes, thread/workspace/device services and command insertion. `POST /threads` stores optional initial input in an awaiting turn but allocates only `thread.create`; projector later creates one guarded `turn.start`. Add idempotent `PATCH /threads/:thread_id/name`, accepting bounded normalized `name` and queuing typed `thread.rename`; fixtures cover same-key replay, source-ref/raw-thread/cwd resolution, `thread/name/set`, and title projection after provider result. Add separate idempotent `PATCH /threads/:thread_id/provider-metadata`, validating generated `gitInfo` only with optional nullable `sha`, `branch`, and `originUrl`, and queuing typed `thread.metadata.update`; fixtures cover same-key replay, source-ref resolution and Git projection after provider result. `PATCH /threads/:thread_id` stays DEEIX labels/share/is_pinned only. Add the atomic aggregate snapshot DTO carrying thread, included turns/items/interactions and `snapshotSeq`. Run `pnpm api:generate` and `pnpm api:check`. Browser API client can list projection and create queued thread/turn intent.

Keep `thread/section/move` and `threadSection/*` separately locked extensions with no first-release control.

### Batch 3: WSS durability

Implemented: credential exchange, `Sec-WebSocket-Protocol` upgrade, command dispatch, hello cursor recovery, durable Bridge frames and staged-inbox projector. Multi-instance socket notification remains a deployment-scaling hardening item; PostgreSQL replay is authoritative.

### Batch 4: Codex Bridge

Implement the native Agent durable state store, source registry, app-server adapter, lifecycle method registry, event/interaction mapper and fixture tests. Carry bounded deadlines, check them before invocation and recovery, and emit a local terminal error without a provider call when invocation never began. Cover typed `thread.metadata.update` `gitInfo`, source-ref/provider ID/cwd resolution, crash recovery without blind replay, source reconciliation and idempotent cloud projection. Keep [codex-app-server-v0.151.0.lock.json](./codex-app-server-v0.151.0.lock.json) as the reviewed stable schema evidence and run `go test ./internal/agentclient` from `backend`. Include no-turn MCP elicitation and server-initiated `item/tool/requestUserInput` source-request correlation. Start with stable lifecycle/thread/turn/interaction/resource reads; gate separately reviewed experimental methods by the provider manifest.

Keep section-operation fixtures out of the mapped method: `thread/section/move` and `threadSection/*` remain separately locked extensions with no first-release control.

### Batch 5: Web workbench

Add static `/agent` launcher/workbench, `NavAgentThreads`, page-scoped reducer, atomic-snapshot/cursor replay, interactions, resource pages and accessibility scenarios. Keep `/chat` visual/state behavior unchanged.

### Batch 6: hardening

Add output-file download/share redaction, aggregate deletion cleanup, retention, observability, load/overflow checks and multi-instance dispatch notification. Input images/audio already use `AgentArtifact` and command-bound grants; no second transfer command is planned.
