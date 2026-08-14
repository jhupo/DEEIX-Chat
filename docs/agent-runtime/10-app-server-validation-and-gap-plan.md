# Codex app-server 功能验收与差距计划

> 验收日期：2026-08-14。官方依据为 [Codex app-server 文档](https://learn.chatgpt.com/docs/app-server)，实现依据为仓库锁定的 `rust-v0.147.0` schema、`CodexAdapter` 和统一 Conversation Gateway 链路。

## 1. 结论

本轮完成了 app-server 适配器全部已声明命令的真实进程或确定性 JSONL 验收：

- 原生 `deeix-agent`：配置、状态、协议、JSONL 并发与三平台构建回归通过。
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
Set-Location backend
go test ./internal/agentclient ./cmd/deeix-agent
go vet ./internal/agentclient ./cmd/deeix-agent
deeix-agent doctor
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

### 4.5 Web 产品入口状态

下表区分 app-server/Bridge 已有能力与 Web 产品闭环。只有 Cloud 命令、Bridge 调度、本地 app-server、事件投影和 Web 状态恢复全部接通，才记为“已接通”。

| 用户能力 | app-server/Bridge | Web 当前状态 | 结论 |
| --- | --- | --- | --- |
| 新建本地会话 | `thread/start` 已映射并通过真实进程测试 | Web 在设备 Workspace 中创建 gateway conversation，首次发送输入时创建并绑定本地 thread | 已接通；空白 Web 会话在首次输入后才产生 app-server thread |
| 读取本地会话历史 | `thread/list` + `thread/read(includeTurns:true)` 已映射并实测 | `sessions` 刷新会导入 user/assistant/reasoning 消息并显示在对应 Workspace | 已接通，但当前只取最近 30 个未归档会话 |
| 从 Web 继续本地会话 | `thread/resume` + `turn/start` 已映射并实测 | Conversation 通过持久化 `sourceThreadRef` 找回同一 provider thread | 已接通；当前 Web 输入限普通文本与已授权附件 |
| 输入队列 | app-server 以同一 thread 的连续 turn 表示顺序输入；活动 turn 可用 `turn/steer` | Web 已有排队、编辑、删除和优先发送 UI，但队列只在 React 内存中 | 普通聊天已有临时队列；刷新会丢失，本地 gateway 尚未形成正确闭环 |
| 调整方向 | `turn/steer` 已映射并通过真实活动 turn 测试 | 当前“调整方向”会中断当前生成，再把选中项作为下一轮发送 | UI 语义与 app-server 不一致，需直接接入 `turn/steer` |
| 模型生成任务 | 每个本地回合有 app-server turn | Cloud 已持久化 Run，gateway 额外持久化 AgentTurn，并支持 run list、stream resume 与 interrupt | 任务数据已具备；缺少跨会话任务中心和队列任务恢复 |
| 模型生成 Plan | `turn/plan/updated`、`item/plan/delta` 已映射 | 当前只作为通用 `execution_event` 传输和 AgentEvent 保存 | 事件已接收，缺少结构化 Plan 投影、历史恢复和步骤 UI |
| Thread Goal | `thread/goal/updated`、`thread/goal/cleared` 已映射；`set|get|clear` 仍为 extension | 当前只保存通用事件，没有 Goal 状态或控制面 | 本地模型产生的 Goal 可到达 Cloud，Web 尚未展示或管理 |
| 新建本地项目 | app-server 没有 DEEIX Workspace 注册语义；Bridge 只上报客户端已配置根目录 | Web 只能选择现有 Workspace | 缺口；需要受控的 Workspace 创建/注册流程 |
| 删除本地会话 | `thread/delete` 已映射并实测 | 当前删除接口只软删除 DEEIX Conversation | 未形成产品闭环；本地 thread 仍存在且可能被再次导入 |
| 归档/恢复本地会话 | `thread/archive`、`thread/unarchive` 已映射并实测 | 当前归档接口只更新 DEEIX Conversation | 未形成产品闭环；需要把生命周期命令送到设备 |
| 读取本地 Skill | Workspace `skills/list` 已映射并实测 | Cloud 可保存 `skills` snapshot，页面尚未读取展示 | 后端能力已具备，UI 待接入 |
| 读取本地 Plugin | Profile `plugin/list` 已映射并实测 | Cloud 保存 `plugins` snapshot；Web 通过 profile resource client 缓存优先读取并支持手动刷新 | 已接入独立设备插件页；安装、卸载仍未开放 |
| 展示文件 Diff | `turn/diff/updated`、`item/fileChange/patchUpdated` 已映射；item/event 可持久化 | Conversation SSE 会下发通用 `execution_event`，前端尚未解析，刷新后也没有 Diff 历史查询入口 | 传输与存储基础已具备，UI 和历史恢复待接入 |

聊天模式“插件”页面属于 DEEIX 服务端 Skill/Prompt 库；工作模式“插件”页面读取选中设备 Codex profile 的 `plugins` snapshot。两者使用不同数据源和路由，不混用同名数据。

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

### P0：正确性和能力声明（已完成）

实现基线：2026-08-14。定向 repository/socket/Adapter 测试已进入默认测试集；真实生产链路测试为显式 opt-in。

兼容性边界：产品协议只接受 `deeix.bridge.v2` 与 frame `version: 2`，没有 v1 解码、双写、降级或旧路由分支。Bridge 配置、设备身份和 WAL 中各自的 `version: 1` 是这些本地持久化格式的当前首版，不代表 Bridge v1。`generated/codex-app-server-v0.147.0` 中出现的 legacy/deprecated 类型来自官方锁定 schema，保持原样用于版本审计；项目方法策略将不采用的旧通知设为 `disabled`，不会把它们接入产品事件链路。

#### 6.1 AgentTurn 终态投影不完整

实现：`projectAgentEvent` 与乱序 terminal result 都解析 `payload.turn.status`，只接受 `completed|interrupted|failed`；`AgentTurn` 同时持久化失败 code/message，迟到的 `turn/started` 不会覆盖终态。

已完成：

1. 在 `backend/internal/infra/persistence/postgres/agentgateway/repository.go` 解析 `payload.turn.status`。
2. 只允许 `completed|interrupted|failed`，同时保存 error code/message。
3. 增加三种终态和乱序 terminal/result 的 repository 测试。

验收：AgentTurn、Conversation Run、最终 assistant message 三者终态一致。

#### 6.2 Interaction 类型丢失

实现：事件投影校验六类 method allowlist，并把 `AgentInteraction.Kind` 保存为稳定语义枚举；API 返回脱敏 request 与语义 kind。

已完成：

1. 在事件投影时校验 `payload.method` 属于六类 allowlist。
2. `AgentInteraction.Kind` 保存语义枚举，例如 `command_approval`、`file_approval`、`user_input`、`permission`、`mcp_elicitation`、`dynamic_tool`。
3. `RequestJSON` 只保存已投影的 `payload.request`。
4. 后端根据 Kind 校验响应 union，而不是只按客户端提交的 `response.kind` 校验。

验收：错误响应类型被拒绝，正确类型可完成 CAS 状态转换，刷新页面后仍可继续处理 pending interaction。

#### 6.3 Manifest 只声明命令名

实现：单一 `ProviderManifest` 已包含 commands、resources、input kinds、thread settings 与 interaction kinds；runtime proof 传输并持久化该快照，profile API 返回同一结构。

已完成：扩展单个 manifest 结构，增加 `resources`、`inputKinds`、`threadSettings`、`interactionKinds`。未创建第二套 capability registry。

验收：Cloud 持久化 manifest snapshot；API/UI 只根据 manifest 渲染控件；Bridge 仍在执行前重新校验。

#### 6.4 缺少可重复的生产 WSS 全链路测试

实现入口：`backend/internal/agentclient/gateway.go`。生产验收使用已安装的原生 Agent、官方独立 Codex CLI 和目标 Full 部署。

该测试完成 enrollment、runtime proof、manifest/profile 持久化读取、workspace sync、gateway conversation、turn stream、interaction response、WSS ack、Bridge WAL、进程级断线重连、sessions/app-server history 回读和清理。Web API 在重连后读取的 profile、message、interaction 和 resource snapshot 用于验证 PostgreSQL 持久化状态。

运行前，测试使用 SHA-256 锁定的 Codex 0.147.0 官方完整 `codex-package` 与隔离的临时 `CODEX_HOME`；该目录中的 API key 归属于 `CODEX_GATEWAY_E2E_ACCESS_TOKEN` 对应的 Sub2 用户，runtime proof 会验证这层归属关系。完整包必须同时存在主程序与 `codex-code-mode-host`，避免只测到 app-server 初始化却在首次工具调用时失败。测试并发等待同步 turn 请求和 pending interaction，使用 `untrusted` 与 `workspace-write`，通过写入注册 Workspace 和系统临时目录之外的受控 marker 稳定触发越界命令审批；批准后验证真实文件内容并清理。

```powershell
$env:CODEX_GATEWAY_E2E='1'
$env:CODEX_GATEWAY_E2E_URL='https://HOST'
$env:CODEX_GATEWAY_E2E_USER_PUBLIC_ID='USER_PUBLIC_ID'
$env:CODEX_GATEWAY_E2E_ACCESS_TOKEN='ACCESS_TOKEN'
$env:CODEX_GATEWAY_E2E_WORKSPACE='D:\path\to\fixture'
$env:CODEX_GATEWAY_E2E_CODEX='codex' # optional
deeix-agent doctor
```

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

#### 6.10 本地生命周期、Workspace 注册与 Diff 产品闭环

现状：新建本地会话、导入历史和继续输入已经使用同一 `sourceThreadRef` 闭环。删除与归档页面只修改 DEEIX Conversation，尚未调用已经映射的 `thread/delete|archive|unarchive`。本地 Workspace 只来自 Bridge 配置；Diff 通知虽已进入统一事件流，前端没有结构化展示或刷新恢复。

修改：

1. 在 Conversation 使用的 `gatewayExecutor` 增加单一 typed lifecycle 方法，只允许 `archive|unarchive|delete`，根据 conversation 绑定生成现有 `thread.lifecycle` 命令。
2. 生命周期 HTTP 接口先校验 conversation 属于当前用户和 gateway execution，再持久化命令；由命令 terminal result 投影最终 Conversation 状态。设备离线时显示 pending，不把排队状态伪装成已完成。
3. 删除得到设备 ACK 后再软删除 Conversation。保留以 `runtime_profile_id + source_thread_ref` 唯一定位的 AgentThread tombstone；`syncWorkspaceSessions` 使用包含软删除记录的查询，命中 tombstone 时跳过导入，防止已删本地会话重新出现。
4. Workspace 创建使用客户端配置的 allowlisted parent roots。Web 仅提交 `rootRef`、合法单段目录名和显示名；Bridge 将 opaque root ref 解析为本机根目录，拒绝绝对路径、`.`、`..`、分隔符、符号链接越界和已存在的非目录目标，然后创建目录并同步 Workspace。注册任意现有目录继续由本机 CLI 或原生目录选择器完成，Cloud 不接收绝对路径。
5. Profile resource client 与独立设备插件页已完成，使用缓存优先、显式刷新和 device/profile/refreshed-at 状态；Workspace `skills` 页面仍待接入。
6. Conversation SSE 将 `turn/diff/updated` 和 `item/fileChange/*` 解析成 typed execution event。后端按 thread/turn/item 保存最新结构化 patch，并增加 conversation-scoped、分页、只读 Item/Diff API；前端按文件分组展示 additions/deletions、状态和 unified diff，刷新或重连后从历史 API 恢复。
7. Diff 内容继续经过事件大小限制、字段 allowlist 和 workspace 归属校验；Web 只展示 patch，不接收本机绝对路径，也不直接根据 patch 执行文件写入。

验收：

- 新会话首次输入只创建一个本地 thread；重试使用同一 idempotency key。
- 归档、恢复和删除在设备在线、离线排队、ACK、失败重试和重复通知下保持 Cloud 与 app-server 一致。
- 已删除 thread 在后续 `sessions` 刷新中不会重新生成 Conversation。
- Workspace 创建覆盖合法目录、重复目录、越界、符号链接逃逸和并发创建。
- Skill/Plugin snapshot 在在线、离线、过期和切换设备时展示正确来源。
- Diff 覆盖实时增量、最终 patch、多文件、重连补拉、乱序/重复事件和大内容截断。

#### 6.11 输入队列、生成任务、Plan 与 Goal

现状：聊天输入队列已经支持排队、编辑、删除和“调整方向”，但 `queuedSubmissions` 只存在浏览器 React state。排队项尚未提交 Cloud，因此刷新、关闭标签页或浏览器异常会丢失。队列成为真实请求后才创建持久化 Run。gateway 的 `startGatewayTurn` 又会拒绝分支 parent 字段，因此现有普通聊天队列不能直接视为本地 app-server 队列。“调整方向”当前执行 interrupt 后优先发送下一轮，没有调用已经映射的 `turn/steer`。

每次实际生成已有 Run；gateway 还有 AgentTurn、AgentItem 和 AgentEvent。`turn/plan/updated`、`item/plan/delta`、`thread/goal/updated`、`thread/goal/cleared` 会经 Bridge 到达 Cloud，但 Conversation 只把它们作为通用 `execution_event` 下发。Plan、Goal 没有结构化快照，刷新后页面也没有读取入口。Goal 的 `thread/goal/set|get|clear` 当前仍为 extension。

修改：

1. 增加单一 `ConversationQueuedSubmission` 持久化模型，保存用户、conversation、branch parent、输入、附件引用、生成设置、顺序、状态和 idempotency key。输入与设置继续复用现有 SendMessage 校验，不复制第二套请求结构。
2. 增加 enqueue/list/update/delete/promote API。队列写入和附件引用在同一事务完成；每个 branch 同时只 dispatch 一个 submission。使用数据库行锁领取任务，服务重启后继续处理 `queued`，卡在 `dispatching` 的项目按 idempotency key 对账。
3. 普通“排队”在前一 Run 到达 terminal 后创建下一 Run。gateway 队列不携带 Web 分支 parent 给 app-server，而是通过 Conversation 与 AgentThread 绑定继续同一 `sourceThreadRef`，调用新的 `turn/start`。
4. “调整方向”在目标 AgentTurn 仍活动时生成 `turn.steer`，等待 Bridge command ACK 后从队列移除；活动 turn 已终止时，将该项提升为下一条普通输入。steer 不先调用 interrupt，也不创建第二个并行 turn。
5. 继续使用 Run 作为跨 cloud/gateway 的唯一模型生成任务。增加用户级分页任务查询，支持 conversation、execution type、task type 和 status 筛选；状态统一为 `queued|running|waiting_interaction|completed|interrupted|failed`。AgentTurn 是 provider 细节，不再创建另一套用户任务实体。
6. 将 `turn/plan/updated` 作为 turn 当前 Plan 的权威快照，保存 explanation 与 `pending|inProgress|completed` steps；`item/plan/delta` 仅用于实时显示，不用拼接结果覆盖最终快照。
7. 将 `thread/goal/updated` 投影为 thread 当前 Goal，保存 objective、`active|paused|blocked|usageLimited|budgetLimited|complete`、token budget/used、time used 和更新时间；`thread/goal/cleared` 清空当前快照并保留审计事件。
8. 第一阶段 Goal 只读展示。需要 Web 管理目标时，再把 `thread/goal/set|get|clear` 从 extension 提升为 typed command，并同时更新 schema lock、dispatch registry、fixture、权限和审计；Web 不直接提交 raw app-server payload。
9. 聊天输入框上方继续使用现有紧凑队列样式；任务中心展示跨会话运行。Plan 作为当前回复内的步骤列表，Goal 作为 thread 顶层状态，两者不与输入队列混成一个列表。

验收：

- 队列跨刷新、重新登录和服务重启保留，编辑、删除、提升顺序后附件引用与 parent 关系正确。
- 同一 branch 没有并行 turn；不同 conversation 的并发仍受用户级上限控制。
- gateway 普通队列连续执行同一 app-server thread；“调整方向”真实发出一次 `turn/steer`，不产生 interrupt + turn/start 组合。
- Run、AgentTurn、队列项在完成、中断、失败、审批等待、设备离线和重复回执下状态一致。
- Plan 覆盖增量、最终快照、乱序、重复事件和刷新恢复；最终 steps 以 `turn/plan/updated` 为准。
- Goal 覆盖 updated、cleared、预算耗尽、暂停、阻塞和完成，并按 user/thread 校验归属。

#### 6.12 Web UI 组件布局与状态边界

现状：`chat-input.tsx` 只把临时输入队列硬编码在输入框上方，没有可复用的会话状态容器。前端没有处理 gateway 的通用 `execution_event`，因此 `turn/diff/updated`、`item/fileChange/patchUpdated`、`turn/plan/updated` 和 `thread/goal/updated|cleared` 虽已到达 SSE，页面仍无对应展示。现有 `message-tool-trace.tsx` 面向普通聊天的工具 trace，不能代替 app-server Plan、Goal、Diff 或跨会话任务 UI。

UI 位置固定如下：

```text
ChatComposerStatusStack                  thread/conversation scope
  GoalStatusBar                         当前 Thread Goal，最多一行
  ActiveRunStatus                       当前运行、审批等待或设备离线状态
  QueuedSubmissionList                  当前 conversation/branch 输入队列
ChatInput                               输入框

AssistantMessage                        turn scope
  PlanSteps                             当前 turn 的模型计划
  TurnProgressDock                      紧凑进度控件：第 X/Y 步 · N 个文件已更改 +A -D
    PlanProgressPopover                 当前步骤、计划清单与完成状态
    FileChangePopover                   文件名与逐文件增删统计
      UnifiedDiffSheet                  完整 diff

TaskCenter                              user scope，独立于输入框
  Runs across conversations             跨会话 queued/running/terminal 任务
```

实现：

1. 增加一个显式 `ChatComposerStatusStack`，由 Chat runtime 直接传入 goal、active run 和 queue props。它不是动态插件 registry，也不接受任意组件注入；只解决已经存在的三个会话级状态，保持单一数据流。
2. `GoalStatusBar` 仅在 gateway conversation 存在 Goal 时显示，固定高度、单行截断；显示状态、objective、耗时和预算摘要。第一阶段只读。Goal 管理命令接通后再增加编辑、暂停/继续和清除按钮。
3. `ActiveRunStatus` 只显示当前 conversation 的运行状态以及待处理 interaction。跨会话运行不堆在输入框上方，统一进入 TaskCenter。
4. 复用现有 QueuedSubmission UI 的编辑、删除和优先操作，抽离到 `QueuedSubmissionList`；改为读取服务端队列 snapshot，并保留乐观更新与失败回滚。
5. `PlanSteps` 属于产生它的 turn，按 `pending|inProgress|completed` 渲染。默认收进对应 assistant message 底部的 `TurnProgressDock`；点击“第 `X/Y` 步”打开 `PlanProgressPopover`，展示截图中的步骤清单、当前步骤和完成状态。Goal 属于 thread，Plan 属于 turn，两者不合并。
6. `TurnProgressDock` 是展示容器，不是新的状态模型。它在一条稳定的紧凑控件内并列显示“第 `X/Y` 步”和“`N 个文件已更改 +A -D`”；两段各自是独立按钮，分别打开计划与文件 popover。仅有计划或仅有文件变化时只显示对应按钮，两者都没有时不渲染空占位。
7. `FileChangePopover` 逐项显示 workspace-relative 文件名和增删统计；选择文件后进入 `UnifiedDiffSheet` 查看完整 diff。完成 turn 后，`TurnProgressDock` 仍从历史 Plan 与 Item/Diff API 恢复，不依赖浏览器内存。
8. 增删统计从结构化 `FileUpdateChange.diff` 或最终 `turn/diff/updated.diff` 解析；最终汇总按规范 unified diff parser 计算，不用字符串搜索文件名。已废弃且 app-server 不再发送的 `item/fileChange/outputDelta` 在 Bridge 方法表中明确为 `disabled`，不进入事件、存储或 UI 链路。
9. Diff 中的路径必须转换为 workspace-relative display path；绝对路径、越界路径和无法归属当前 Workspace 的 change 不进入浏览器。超大 diff 显示截断状态并提供按文件分页读取。
10. TaskCenter 复用 Run 作为列表源，展示 conversation、设备、Workspace、模型、状态、开始时间和耗时；运行中任务可进入对应对话或中断，terminal 任务只读。AgentTurn/AgentItem 作为详情来源，不与 Run 并列成重复任务。
11. 桌面端 popover 和 sheet 使用项目现有组件；移动端计划、文件列表与 diff 使用全宽 sheet。两个触发按钮均带可访问名称和展开状态，Goal、Plan、Diff 的状态变化使用克制的 live region，不抢占输入焦点。

验收：

- cloud conversation 不显示设备 Goal/Plan/Diff 占位；切换 conversation、设备或 branch 后没有上一上下文残留。
- 输入框高度不会因状态刷新抖动；长 Goal、长文件名、100+ 队列项和窄屏下均不溢出或遮挡输入。
- Plan、Diff 精确绑定 source turn；Goal 精确绑定 source thread；TaskCenter 精确绑定用户和 Run。
- TurnProgressDock 的步骤进度绑定当前 turn 的最终 Plan；文件数与 `+A -D` 绑定同一 turn 的最终 unified diff，rename/add/delete、多 hunk、二进制文件和截断均有测试。
- 页面刷新和 SSE 重连后，Goal、Plan、队列、任务与 Diff 均由服务端快照恢复，再继续应用新事件。
- 键盘可分别打开计划清单与文件汇总、浏览步骤和文件、关闭 popover/sheet；屏幕阅读器可获得按钮名称、展开状态和任务状态更新。

### P2：补只读诊断和可选工具面

#### 6.13 Account 与 Config

当前运行时已验证 `account/read`、`config/read`，产品只使用 `getAuthStatus` 和 HMAC proof。用户账户订阅额度仍由 Sub2API 提供，Codex account 只应作为本机 runtime 诊断。

修改：新增脱敏的 `runtime-account` 与 `runtime-config-summary` profile resource，只保留 auth type、provider、受管要求和 feature flags。API key、token、endpoint secret、绝对路径均不进入 Cloud。

#### 6.14 Filesystem 与 command/exec

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
| `backend/internal/agentclient/protocol.go` | 输入、设置、交互响应与资源命令的严格边界校验 |
| `backend/internal/agentclient/state.go` | opaque ref、命令收件、终态和事件的持久状态 |
| `backend/internal/agentclient/codex.go` | app-server 语义命令、资源、事件与交互适配 |
| `backend/internal/agentclient/gateway.go` | WSS、恢复、调度与上行确认 |
| `docs/agent-runtime/codex-app-server-*.lock.json` | 版本、生成物和全 union 状态 |
| `backend/internal/application/agentgateway/service.go` | typed command 创建、响应 union 校验、资源刷新 |
| `backend/internal/infra/persistence/postgres/agentgateway/repository.go` | thread/turn/item/interaction 状态投影与幂等 |
| `backend/internal/infra/persistence/models/agent_gateway.go` | AgentTurn Plan 与 AgentThread Goal 当前快照 |
| `backend/internal/infra/persistence/models/chat.go` | 持久化输入队列与统一 Run 状态 |
| `backend/internal/application/conversation/service_conversation.go` | gateway 会话归档、恢复、删除的生命周期编排 |
| `backend/internal/application/conversation/service_gateway_projection.go` | 统一 Conversation 流和终态 |
| `backend/internal/transport/http/agentgateway` | 设备、资源和管理命令 API |
| `frontend/shared/api/agent-gateway.ts` | Workspace/Profile 资源与生命周期 API client |
| `frontend/features/layouts/components/navigation/nav-projects.tsx` | 共享项目树；聊天映射 DEEIX Project，工作映射设备 Workspace |
| `frontend/features/devices/components/agent-plugins-page.tsx` | 设备 Codex Plugin 快照与刷新入口 |
| `frontend/features/chat/components/sections/chat-input.tsx` | 输入框与会话级 status stack 的组合边界 |
| `frontend/features/chat/components/message` | turn-scoped Plan、文件汇总和 unified diff 入口 |
| `frontend/features/chat/hooks/use-chat-message-submit.ts` | 服务端输入队列、乐观更新和真实 steer |
| `frontend/features/chat` | typed execution event、Goal/Run 状态与历史恢复 |
| `backend/internal/agentclient/agentclient_test.go` | 配置、状态、严格协议与双向 JSONL fixture |
| `deeix-agent doctor` | 本机官方 Codex CLI、app-server 与设备凭据验收 |

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
Native Agent fixture suite
real pinned process lifecycle E2E
backend agentgateway unit/repository/socket tests
Conversation gateway projection tests
API generation/check
package smoke on target platform
```

涉及 MCP/App/Skill/approval 新能力时，还要加入对应固定 fixture；涉及 schema 版本变更时，不接受只改版本字符串或只更新生成文件。
