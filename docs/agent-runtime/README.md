# DEEIX Chat 与 Agent Runtime 并存设计

> 状态：本地 Bridge 与 Cloud Agent 后端控制面已实现；Web 工作台待实现
> 源码基线：`dev` 当前工作树
> 调研日期：2026-08-09

## 决策

DEEIX 保留两个执行域，边界由路由、API、聚合、reducer 和持久表共同确定：

```text
Web /chat  -> /api/v1/conversations/* -> Conversation / Message / Run
Web /agent -> /api/v1/agent/*         -> AgentThread / Turn / Item
                                             -> outbound WSS -> Local Bridge
                                                                    -> Codex app-server (stdio)
```

普通聊天继续使用现有 LLM、RAG、服务端 MCP、DB Skill、媒体、分享和导出链路。订阅、充值、兑换、余额和用量的既有页面与
`/api/v1/billing/*` Web contract 保留，但其后端在 clean-slate cutover 改为固定 Sub2API 的具体 BFF；本地结算不再是聊天链路的一部分。
`backend/internal/infra/llm/adapter.go` 的 `Generate` / `GenerateStream` 是单向生成抽象；Codex app-server 的双向 server request 由本地 Bridge 驱动。

`frontend/next.config.ts` 使用 `output: "export"`。Agent 的静态入口固定为 `/agent`，活动 thread 使用 `/agent?thread_id=<thread_public_id>`。`/chat?conversation_id=...` 保持原状。

## 索引

| 文档 | 实现决策 |
| --- | --- |
| [01-architecture.md](./01-architecture.md) | 组件责任、数据所有权、时序、恢复、传输、部署与安全 |
| [02-codex-app-server.md](./02-codex-app-server.md) | Codex method matrix、schema pin、adapter 和 mapper |
| [codex-app-server-v0.147.0.lock.json](./codex-app-server-v0.147.0.lock.json) | 初版稳定 Codex release、生成物哈希、四个 union 的穷尽 disposition 与 drift checker 合约 |
| [03-protocol-and-data-model.md](./03-protocol-and-data-model.md) | REST/WSS contract、表、事务、状态机与保留策略 |
| [04-source-migration.md](./04-source-migration.md) | 当前源码 retain/add/modify 清单与实施批次 |
| [05-web-experience.md](./05-web-experience.md) | `/agent` launcher/workbench、侧栏、reducer 与可访问流程 |
| [06-full-deployment-and-online-update.md](./06-full-deployment-and-online-update.md) | Full Docker、tag 全量应用包、持久化运行卷和 Superadmin 在线更新 |
| [07-sub2api-account-and-billing.md](./07-sub2api-account-and-billing.md) | 历史 Sub2API 双 authority 调研；已由 08 替代 |
| [08-clean-slate-identity-commerce-runtime.md](./08-clean-slate-identity-commerce-runtime.md) | 只改 DEEIX：Sub2 REST 登录与 commerce BFF、保留订阅/充值/兑换 UI、Chat-only key binding、本地现有 key 的 HMAC 准入证明 |
| [09-local-gateway-design.md](./09-local-gateway-design.md) | 复用统一 Agent/Provider 合同的 Local Gateway、Runtime Registry、Codex Adapter、WAL、认证与恢复实施基线 |

## 不变量

- Agent 不建立第二套用户实体。表内 ownership 外键统一使用 `user_id -> identity_users.id`；Browser API 响应中的 `userId` 使用现有 `identity_users.public_id`（例如账户页显示的用户公开 ID），请求体不接受调用方提交用户 ID。Bridge 只持有 opaque device/profile/workspace/thread refs。
- Sub2API 是固定外部服务；账号、套餐、余额、支付订单、兑换、订阅、Chat key 与 Runtime auth-proof 方案只修改 DEEIX Cloud/Web/Bridge，并只调用 Sub2 当前已有的 REST auth、user、payment、redeem、keys、subscriptions、usage 与 gateway routes。DEEIX 的 opaque browser session、User ownership、session/security UI、general/chat preferences 和 Agent ownership 保留；本地 password、2FA、identity credential、Plan/Price/Subscription/PaymentOrder/余额/ledger 与 admin commerce authority 在 cutover 删除。详见 08。
- Cloud `AgentCommand` 是 allowlisted discriminated union，只含 opaque device/profile/workspace/thread refs、适用的 Bridge-issued source refs 与类型化用户输入。`thread.create` 在 Bridge 绑定前没有 source ref 和用户输入；浏览器初始输入保存在 `awaiting_thread` provisional turn，source ref 绑定后才生成唯一的 `turn.start`。附件输入只保存 `artifactRef`；Bridge 在本地解析 canonical cwd、raw provider ID 和已校验的临时文件，再生成 `ProviderCommand`。
- `ProviderAdapter.execute` 只接收 `ProviderCommand`。一个本地 TypeScript adapter interface 覆盖 lifecycle、execute 与 capabilities；Codex 使用生成 app-server types，未来 Claude 只增加 adapter 与 mapper。
- Web 与 Cloud 只使用 provider-neutral 的 `AgentCommand`、`AgentEvent` 与 `AgentInteraction`；Cloud 根据 Thread 绑定的 Profile 选择并校验目标，本地 Runtime Registry 才选择 Adapter 并把 `ProviderCommand` 转成 provider 协议。Codex JSON-RPC、raw ID 与 canonical cwd 不越过本地边界；详见 09。
- Web 重放固定为 `GET /api/v1/agent/threads/:thread_id/events?after_seq=N`。`AgentEvent.seq` 是 thread projection 的 `thread_seq`，与 Bridge `bridge_seq`、下行 `server_seq` 分离。
- Bridge 上行 durable frame 先写 private `Bridge durable WAL store`，以 `(device_id, bridge_seq)` 去重。云端下行只用 `agent_commands` 队列。Bridge 先持久化不含授权串的 command，再下载并校验附件，最后写 `command.receipt-ready`；连续 `ackServerSeq` 只覆盖 receipt-ready 命令。
- Agent server target deployment has one Full Docker Compose profile: application, PostgreSQL with pgvector, and Redis. PostgreSQL is the only server Gorm database and Redis is required for cache/wake behavior; see [06-full-deployment-and-online-update.md](./06-full-deployment-and-online-update.md). Current product deployment instructions remain separate until that implementation phase lands.
- enrollment、challenge 和 connection credential 由 active derivation key version 的 HMAC-SHA-256 确定性生成，DB 只保存 token hash 与发行字段。附件 grant 由 server 对 artifact、command、User、device、workspace 和过期时间做 HMAC 绑定，仅出现在临时 WSS envelope 和下载请求头，永不写入 command payload、Bridge WAL 或响应缓存。Node 24 WebSocket 通过 `Sec-WebSocket-Protocol` 携带 WSS connection token。
- `agent_cleanup_jobs` 是不回指用户或 Agent aggregate 的持久清理 outbox：账户删除事务先撤销访问并写入去重 job，post-commit worker 重试删除对象和临时数据。
- AgentThread 通过 device/workspace 与 Agent-owned metadata 分组；它没有 ConversationProject 外键。普通聊天 Conversation/Message/Run 和所有当前行为保持独立。
- Go Swagger DTO/annotation 是 API 事实源，`pnpm api:generate` 生成 `@deeix/api-contract`；Swagger 与 TypeScript 生成物不手工编辑。

## 官方资料

- [Codex app-server overview](https://developers.openai.com/codex/app-server/)
- [Codex app-server source](https://github.com/openai/codex/tree/main/codex-rs/app-server)
- [Codex repository](https://github.com/openai/codex)
- [Claude Agent SDK overview](https://code.claude.com/docs/en/agent-sdk/overview)
