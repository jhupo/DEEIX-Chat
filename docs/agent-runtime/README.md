# DEEIX Chat 统一会话运行时

> 当前实现基线：`Conversation` 是 Web 唯一会话聚合；Cloud Responses 与本地 app-server 通过执行适配器接入同一条 Turn 链路。

## 当前决策

```text
Web
  -> Conversation API
  -> Turn Service
  -> Execution Router
       -> Cloud execution -> Sub2API Responses
       -> Gateway execution -> Local Bridge -> Codex app-server
  -> Conversation Message / Run / ExecutionEvent
  -> Web
```

- Web 不创建或操作第二套 Agent 会话。
- `AgentThread`、`AgentTurn`、`AgentItem` 只属于 Gateway Adapter 的内部执行状态。
- 一个用户可以配对多个设备；一个 Conversation 创建时固定绑定一种执行方式。
- Gateway Conversation 固定绑定 `deviceID + profileID + workspaceID`，活动会话不热切换目标。
- Cloud 与 Gateway 都使用 `/conversations/:id/turns`、消息、Run、事件和交互接口。
- 不保留旧 Agent Thread Web API 或自动回退逻辑。

## 文档索引

| 文档 | 内容 |
| --- | --- |
| [01-architecture.md](./01-architecture.md) | 统一链路、组件边界、数据所有权与可靠性 |
| [02-codex-app-server.md](./02-codex-app-server.md) | app-server method/schema 调研；其中旧 Web 路径示例仅是调研记录，不是当前 API 合同 |
| [03-protocol-and-data-model.md](./03-protocol-and-data-model.md) | 当前 REST、事件、表关系与状态机 |
| [04-source-migration.md](./04-source-migration.md) | 早期迁移研究记录，不作为当前源码合同 |
| [05-web-experience.md](./05-web-experience.md) | 后续统一会话 UI 设计 |
| [06-full-deployment-and-online-update.md](./06-full-deployment-and-online-update.md) | Full Docker 与在线更新 |
| [08-clean-slate-identity-commerce-runtime.md](./08-clean-slate-identity-commerce-runtime.md) | Sub2API identity/commerce 设计 |
| [09-local-gateway-design.md](./09-local-gateway-design.md) | Local Bridge、设备、多网关与信任边界 |
| [10-app-server-validation-and-gap-plan.md](./10-app-server-validation-and-gap-plan.md) | app-server 真实进程验收、官方文档差距与实施顺序 |
| [codex-app-server-v0.147.0.lock.json](./codex-app-server-v0.147.0.lock.json) | app-server schema 锁定证据 |

## 实现入口

- Conversation 编排：`backend/internal/application/conversation/service_turn.go`
- Gateway 事件归一化：`backend/internal/application/conversation/service_gateway_projection.go`
- Conversation 执行事件事务：`backend/internal/infra/persistence/postgres/conversation/repository_execution.go`
- Gateway 应用服务：`backend/internal/application/agentgateway/service.go`
- Local Bridge：`packages/agent-bridge`

## P0 回归

- 默认回归：`pnpm --filter @deeix/agent-bridge test` 与 `go test ./internal/application/agentgateway ./internal/infra/persistence/postgres/agentgateway ./internal/transport/http/agentgateway`。
- 生产 WSS 全链路：按 [10-app-server-validation-and-gap-plan.md](./10-app-server-validation-and-gap-plan.md) 设置 `CODEX_GATEWAY_E2E_*` 后运行 `pnpm --filter @deeix/agent-bridge test:gateway:e2e`。
