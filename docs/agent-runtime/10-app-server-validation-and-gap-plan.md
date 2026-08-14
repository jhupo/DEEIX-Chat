# Codex app-server 功能验收与差距计划

> 验收日期：2026-08-14。官方依据为 [Codex app-server 文档](https://learn.chatgpt.com/docs/app-server)，实现依据为仓库锁定的 `rust-v0.147.0` schema、`CodexAdapter` 和统一 Conversation Gateway 链路。

## 1. 结论

本轮完成了 app-server 适配器全部已声明命令的真实进程或确定性 JSONL 验收：

- `@deeix/agent-bridge`：32/32 通过。
- schema lock：`ClientRequest` 98、`ClientNotification` 1、`ServerRequest` 10、`ServerNotification` 72，生成文件、哈希和 union 全部通过。
- 真实进程：官方 `rust-v0.147.0` Windows x64 release，stdio JSONL，真实 API key 认证和真实 Responses 上游。
- 真实生命周期：初始化、认证证明、资源读取、创建、改名、Git 元数据、模型回合、历史读取、压缩、分叉、引导、中断、Review、归档、恢复、重新载入和删除全部通过。
- 六类服务端主动请求：命令审批、文件审批、用户提问、权限申请、MCP elicitation、动态工具调用的请求投影和响应映射全部通过 fixture。
- 测试线程由 `finally` 删除，没有保留测试会话或测试文件。

这表示当前适配器基础链路可用，但还不是官方 app-server 的完整产品能力。主要缺口是输入类型、审批决策、App/Skill/MCP 操作、线程分页、账户与配置资源、终端/文件能力，以及官方当前文档相对 `0.147.0` 的实验 API 漂移。

## 2. 验收口径

| 状态 | 含义 |
| --- | --- |
| 真实进程通过 | 调用了锁定的 app-server 可执行文件，并验证返回值、事件或持久化历史 |
| Fixture 通过 | 通过双向 JSONL fixture 验证 DEEIX 映射、脱敏和响应格式，没有依赖外部 MCP/App |
| 仅运行时通过 | app-server 原生方法调用成功，但 DEEIX `AgentCommand` 尚未暴露该能力 |
| 缺口 | 官方 schema 或文档存在，DEEIX 当前协议、适配器或 Cloud 投影尚未实现 |
| 非目标 | 有意留在本机或实验面，当前产品不暴露 |

“完整验收”在本文中表示每个官方能力都有明确归类和验收条件，不把存在于 schema 中的方法写成已经进入 DEEIX 产品链路。

## 3. 实际执行命令

```powershell
pnpm.cmd --filter @deeix/agent-bridge check
pnpm.cmd --filter @deeix/agent-bridge check:schema
$env:CODEX_E2E_EXECUTABLE = (Resolve-Path '.cache/codex-e2e-v0.147.0/bin/codex-x86_64-pc-windows-msvc.exe').Path
pnpm.cmd --filter @deeix/agent-bridge test
```

真实模型测试需要访问本机 Codex 已配置的 Responses 上游。受限网络环境会得到 `stream disconnected before completion`，开放网络后同一测试通过。这是测试运行前置条件，不属于协议缺陷。

## 4. 已通过矩阵

### 4.1 适配器命令

| DEEIX 命令 | app-server 方法 | 结果 | 证据 |
| --- | --- | --- | --- |
| `thread.create` | `thread/start` | 真实进程通过 | 创建隔离线程并取得 opaque source ref |
| `thread.lifecycle.resume` | `thread/resume` | 真实进程通过 | 归档恢复后重新载入 |
| `thread.lifecycle.fork` | `thread/fork` | 真实进程通过 | 复制已完成历史并取得新 source ref |
| `thread.lifecycle.archive` | `thread/archive` | 真实进程通过 | 状态转换成功 |
| `thread.lifecycle.unarchive` | `thread/unarchive` | 真实进程通过 | 状态转换成功 |
| `thread.lifecycle.delete` | `thread/delete` | 真实进程通过 | 适配器删除分叉线程，清理器删除主线程 |
| `thread.rename` | `thread/name/set` | 真实进程通过 | `thread/read` 返回新名称 |
| `thread.metadata.update` | `thread/metadata/update` | 真实进程通过 | `thread/read` 返回测试 branch |
| `thread.compact` | `thread/compact/start` | 真实进程通过 | 等待压缩内部 turn 到达终态 |
| `review.start` | `review/start` | 真实进程通过 | 等待 Review turn 自然完成 |
| `turn.start` | `turn/start` | 真实进程通过 | 精确回复进入持久化 turn history |
| `turn.steer` | `turn/steer` | 真实进程通过 | 对活动长回合追加输入 |
| `turn.interrupt` | `turn/interrupt` | 真实进程通过 | 被引导回合到达终态 |
| `interaction.respond` | 六类 server request response | Fixture 通过 | 每类请求均验证 source request ref 和响应结构 |
| `resource.refresh` | 10 种资源 | 真实进程通过 | profile 与 workspace 资源均返回投影 |

### 4.2 资源

| DEEIX 资源 | app-server 方法 | 结果 |
| --- | --- | --- |
| `models` | `model/list` | 真实进程通过 |
| `model-capabilities` | `modelProvider/capabilities/read` | 真实进程通过 |
| `permission-profiles` | `permissionProfile/list` | 真实进程通过 |
| `apps` | `app/list` | 真实进程通过 |
| `mcp` | `mcpServerStatus/list` | 真实进程通过 |
| `plugins` | `plugin/list` | 真实进程通过；官方仍标为 under development |
| `auth-status` | `getAuthStatus` | 真实进程通过；只投影模式和是否需要认证 |
| `sessions` | `thread/list` + `thread/read` | 真实进程通过；消息历史可恢复 |
| `skills` | `skills/list` | 真实进程通过 |
| `hooks` | `hooks/list` | 真实进程通过 |

### 4.3 只验证上游运行时的方法

以下方法在真实 app-server 进程中通过，但当前 Web/Cloud/Bridge 协议没有对应命令：

| 方法 | 实测内容 | 当前状态 |
| --- | --- | --- |
| `account/read` | 返回认证状态结构 | 仅运行时通过 |
| `config/read` | 返回 cwd 对应的有效配置 | 仅运行时通过，未向 Cloud 传递内容 |
| `app/installed` | 返回已安装 App runtime snapshot | 仅运行时通过 |
| `fs/getMetadata` | 读取测试工作区目录元数据 | 仅运行时通过 |
| `command/exec` | read-only sandbox 中执行 Node 并校验 stdout | 仅运行时通过 |
| `thread/loaded/list` | 返回当前进程已加载线程 | 仅运行时通过 |

这些探测用于证明版本和运行时行为，不代表 DEEIX 已对远端开放原生文件、命令、账户或配置接口。

### 4.4 服务端主动请求

| 方法 | 请求投影 | 响应映射 | 结果 |
| --- | --- | --- | --- |
| `item/commandExecution/requestApproval` | opaque request/thread/turn ref | `decision` | Fixture 通过 |
| `item/fileChange/requestApproval` | opaque request/thread/turn ref | `decision` | Fixture 通过 |
| `item/tool/requestUserInput` | provider question id 改写为 question ref | answers 反向映射 | Fixture 通过 |
| `item/permissions/requestApproval` | 权限请求脱敏投影 | permissions + scope | Fixture 通过 |
| `mcpServer/elicitation/request` | 支持无 turn 的 thread scope | action + content | Fixture 通过 |
| `item/tool/call` | dynamic tool 请求投影 | typed content items | Fixture 通过 |

真实 MCP/App 环境相关的 elicitation 和 tool call 尚未建立固定测试服务器，因此当前证据是协议 fixture，不是第三方服务器互操作测试。

## 5. Schema 与官方当前文档的边界

### 5.1 锁定 schema

`codex-app-server-v0.147.0.lock.json` 是生产依据：

| Union | 数量 | 当前 disposition |
| --- | ---: | --- |
| `ClientRequest` | 98 | mapped 25，extension 54，disabled 19 |
| `ClientNotification` | 1 | mapped 1 |
| `ServerRequest` | 10 | mapped 6，extension 1，disabled 3 |
| `ServerNotification` | 72 | mapped 42，extension 19，disabled 11 |

`mapped` ClientRequest 现在与 `CODEX_DISPATCHED_CLIENT_REQUESTS` 严格相等。CI 会在任一侧漏登记、额外登记或状态不一致时失败。

### 5.2 官方当前文档比稳定锁更宽

官方当前文档还列出以下未进入 `0.147.0` 稳定锁的实验能力：

- `thread/turns/list`、`thread/items/list`。
- `thread/backgroundTerminals/clean|list|terminate`。
- `process/spawn|writeStdin|resizePty|kill` 及 process notifications。
- `environment/info`、`collaborationMode/list`。
- 当前文档新增的实验字段和筛选项，例如 thread ancestor/parent filters。

Bridge 初始化明确使用 `experimentalApi: false`。升级前必须重新生成稳定与实验 schema，形成新的版本锁；不从网页文档直接猜字段。

### 5.3 传输

- Bridge 使用官方默认的 stdio JSONL，已通过真实进程验收。
- 官方 WebSocket transport 仍标为 experimental/unsupported，且非 loopback 暴露存在认证要求。DEEIX 的远程链路继续使用自有 WSS 到 Bridge，由 Bridge 在本机启动 stdio app-server。
- Unix socket 和 app-server WebSocket 不进入当前产品协议，也不需要作为 DEEIX 兼容层存在。

## 6. 详细缺口与修改方案

### P0：先修正确性和能力声明

#### 6.1 AgentTurn 终态投影不完整

现状：`projectAgentEvent` 收到任何 `turn/completed` 都把 `AgentTurn.status` 写成 `completed`；Conversation 层会解析 `completed`、`interrupted`、`failed`，两个存储口径不一致。

修改：

1. 在 `backend/internal/infra/persistence/postgres/agentgateway/repository.go` 解析 `payload.turn.status`。
2. 只允许 `completed|interrupted|failed`，同时保存 error code/message。
3. 增加三种终态和乱序 terminal/result 的 repository 测试。

验收：AgentTurn、Conversation Run、最终 assistant message 三者终态一致。

#### 6.2 Interaction 类型丢失

现状：数据库 `AgentInteraction.Kind` 保存的是统一事件名 `interaction.requested`，真正方法名位于 `payload.method`。API 使用 `Kind` 返回，前端难以稳定区分审批、提问、权限和 MCP。

修改：

1. 在事件投影时校验 `payload.method` 属于六类 allowlist。
2. `AgentInteraction.Kind` 保存语义枚举，例如 `command_approval`、`file_approval`、`user_input`、`permission`、`mcp_elicitation`、`dynamic_tool`。
3. `RequestJSON` 只保存已投影的 `payload.request`。
4. 后端根据 Kind 校验响应 union，而不是只按客户端提交的 `response.kind` 校验。

验收：错误响应类型被拒绝，正确类型可完成 CAS 状态转换，刷新页面后仍可继续处理 pending interaction。

#### 6.3 Manifest 只声明命令名

现状：`ProviderManifest` 只有 provider、runtimeVersion、protocolVersion、schemaHash、commands。资源、输入类型、设置项、审批决策和实验状态没有机器可读声明。

修改：扩展单个 manifest 结构，增加 `resources`、`inputKinds`、`threadSettings`、`interactionKinds`。保持一个结构，不创建第二套 capability registry。

验收：Cloud 持久化 manifest snapshot；API/UI 只根据 manifest 渲染控件；Bridge 仍在执行前重新校验。

#### 6.4 缺少可重复的生产 WSS 全链路测试

现状：本轮真实进程测试覆盖 Adapter；Cloud socket、队列、Conversation 投影主要由 Go fixture 覆盖。

修改：新增 opt-in `CODEX_GATEWAY_E2E`，启动测试 Bridge，完成 enrollment、runtime proof、workspace sync、gateway conversation、turn stream、interaction response、断线重连和清理。

验收：同一测试同时验证 Web API、PostgreSQL 状态、WSS ack、Bridge WAL、app-server history 和最终 Conversation message。

### P1：补齐 Web 端本地 Codex 的核心体验

#### 6.5 输入类型不足

官方 `UserInput` 支持 text、image URL、localImage、audio URL、localAudio、skill 和 mention。DEEIX 当前只有 text 与 artifact；artifact 仅落为 localImage/localAudio。

修改：

- `AgentInput` 增加 `skill` 和 `app-mention`，字段只用资源 snapshot 中的 opaque ref。
- Bridge 用本机 snapshot 将 ref 解析为 skill path 或 `app://id`，浏览器不提交本机路径。
- 远程 URL 图片继续先进入 DEEIX 文件系统并做 size/SHA-256 校验，再下发 artifact grant；不直接把任意 URL 交给本机。

验收：显式 skill item 和 `$skill-name` 同时发送；App mention 与 `app://id` 一致；伪造 ref、跨 workspace ref 和过期 snapshot 被拒绝。

#### 6.6 Turn 设置项不足

当前仅支持 model、`low|medium|high|xhigh`、三种 approval policy、read-only/workspace-write。官方还支持 service tier、model provider、personality、reasoning summary、output schema、approvals reviewer、granular approval 和更细 sandbox read access。

修改：

- 优先增加 `personality`、`reasoningSummary`、`serviceTier` 和 `approvalsReviewer`。
- reasoning effort 必须从 `model/list` 的 supported values 校验，不继续写死枚举。
- `outputSchema` 作为独立受限 JSON Schema 类型，限制深度、节点数和字节数。
- `dangerFullAccess`、external sandbox 和任意 readable roots 留在本机管理策略，不从普通 Web 会话下发。

验收：设置值同时通过 Cloud allowlist、manifest capability 和生成 schema 三层校验。

#### 6.7 审批决策不完整

官方命令审批支持 `acceptForSession`、`cancel`、exec-policy amendment 和 network-policy amendment；文件审批支持 `acceptForSession`、`cancel`。DEEIX 当前 approval 只有 accept/decline。

修改：扩展 `InteractionResponse` 的语义 decision；amendment 必须来自请求中服务端提出的候选项，Web 只回传 opaque candidate ref，Bridge 解析为原始 amendment。

验收：决策逐项 fixture；请求未提供 amendment 时提交 candidate ref 会失败；session decision 不跨 thread/profile。

#### 6.8 Session 资源固定 30 条且状态写死

现状：`sessions` 固定 `limit: 30`，逐条 `thread/read`，忽略 cursor，投影状态固定为 `active`。这会产生 N+1 调用并把 archived/idle/notLoaded 状态写错。

修改：

- 新增分页 resource command，携带 opaque cursor 和 limit。
- 使用 `thread/list` summary；只在打开详情时调用 `thread/read(includeTurns:true)`。
- 投影官方 `ThreadStatus`，并保留 archived filter。
- 增加 `thread/unsubscribe`，离开详情时释放订阅。

验收：100+ 会话分页、归档筛选、重启后 notLoaded、并发刷新和 cursor 失效测试通过。

#### 6.9 App、Skill、Plugin 和 MCP 目前以列表为主

现状：App 仅 `app/list`，Skill 仅 list，Plugin 仅 list，MCP 仅 status/list。`app/installed`、`app/read`、skill 启停、MCP resource/tool/OAuth/reload 未进入命令层。

修改顺序：

1. 先加只读 `app/installed`、`app/read`、plugin read、MCP resource read。
2. 再加显式 Skill/App 输入。
3. MCP tool call 仅从已加载 tool snapshot 生成 typed command。
4. OAuth、reload、skill config 和 plugin install 作为独立管理命令，带审计与管理员/设备所有者策略。

验收：建立仓库内固定 MCP stdio fixture，覆盖 tools/resources/list/read/call、OAuth completion、elicitation 和断线重连。

### P2：补只读诊断和可选工具面

#### 6.10 Account 与 Config

当前运行时已验证 `account/read`、`config/read`，产品只使用 `getAuthStatus` 和 HMAC proof。用户账户订阅额度仍由 Sub2API 提供，Codex account 只应作为本机 runtime 诊断。

修改：新增脱敏的 `runtime-account` 与 `runtime-config-summary` profile resource，只保留 auth type、provider、受管要求和 feature flags。API key、token、endpoint secret、绝对路径均不进入 Cloud。

#### 6.11 Filesystem 与 command/exec

官方提供 fs 和独立 command API。当前 Agent 工作已经可通过 turn 工具执行，立即增加第二套 Web terminal 会重复权限、流和审计模型。

建议：先保持 extension。产品确实需要集成终端时，增加一个 `terminal.exec` 语义命令，路径只用 workspace-relative path；stdin/resize/terminate 复用同一 process ref 和 WAL，不暴露 raw argv 之外的 shell 字符串拼接接口。

### P3：版本升级后再评估实验能力

- thread turn/item pagination。
- background terminals。
- process API。
- environment info。
- collaboration modes。

这些能力先生成新版本 experimental lock，再决定是否进入稳定 Agent 协议。升级提交必须同时包含生成文件、锁、方法状态、fixture 和真实进程 smoke。

## 7. 文件级修改清单

| 文件 | 修改职责 |
| --- | --- |
| `packages/agent-bridge/src/protocol/agent-command.ts` | 输入、设置、交互响应、资源命令的唯一公开 union |
| `packages/agent-bridge/src/commands/resolve-provider-command.ts` | opaque ref 到本机 path/provider ID 的解析 |
| `packages/agent-bridge/src/providers/codex/codex-adapter.ts` | 生成类型到语义命令/事件的适配 |
| `packages/agent-bridge/src/providers/codex/codex-method-policy.ts` | 实际调度方法 registry |
| `docs/agent-runtime/codex-app-server-*.lock.json` | 版本、生成物和全 union 状态 |
| `backend/internal/application/agentgateway/service.go` | typed command 创建、响应 union 校验、资源刷新 |
| `backend/internal/infra/persistence/postgres/agentgateway/repository.go` | thread/turn/item/interaction 状态投影与幂等 |
| `backend/internal/application/conversation/service_gateway_projection.go` | 统一 Conversation 流和终态 |
| `backend/internal/transport/http/agentgateway` | 设备、资源和管理命令 API |
| `packages/agent-bridge/test/codex-adapter.test.ts` | 双向 JSONL fixture |
| `packages/agent-bridge/test/codex-process.e2e.test.ts` | 真实锁定进程验收 |

## 8. 文档修改规则

1. [02-codex-app-server.md](./02-codex-app-server.md) 只记录当前锁、实际映射和协议边界，不再把目标 UI 写成已实现。
2. [03-protocol-and-data-model.md](./03-protocol-and-data-model.md) 只在对应 union 已进入代码后增加字段。
3. [05-web-experience.md](./05-web-experience.md) 根据 manifest capability 描述 UI，不直接引用 raw app-server method。
4. [09-local-gateway-design.md](./09-local-gateway-design.md) 保持 Cloud 只见 opaque ref、本机路径与 provider ID 留在 Bridge 的边界。
5. 本文是当前验收状态和后续修改顺序；每次 schema pin 升级后更新日期、测试证据和差距表。

## 9. 发布门槛

每次 app-server 相关发布至少通过：

```text
schema lock exact match
TypeScript check
Agent Bridge fixture suite
real pinned process lifecycle E2E
backend agentgateway unit/repository/socket tests
Conversation gateway projection tests
API generation/check
package smoke on target platform
```

涉及 MCP/App/Skill/approval 新能力时，还要加入对应固定 fixture；涉及 schema 版本变更时，不接受只改版本字符串或只更新生成文件。
