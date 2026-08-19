# Codex Desktop 项目、最近与归档边界

> 本文是 DEEIX 工作导航分组的当前实现依据。Codex app-server 版本锁为 `0.147.0`。

## 1. 实测结论

2026-08-19 使用当前 Codex Desktop 自带的 `codex app-server` 手工分页调用 `thread/list`：

- 活动 thread 由 app-server 返回，归档 thread 使用同一方法并设置 `archived: true` 返回。
- app-server 返回 thread 的 ID、标题、预览、`cwd`、状态与时间，但当前锁定 schema 没有 Desktop project ID，也没有 `project/*` 方法。
- Desktop 的“项目”和“最近”互斥。项目中的 thread 不再重复进入“最近”。
- Desktop 的“最近”对应 `.codex-global-state.json` 中 `projectless-thread-ids` 标记的活动 thread。部分 Desktop 版本不保存这些 thread 的 `thread-workspace-root-hints`；Agent 仍会创建一个仅用于同步的隐藏 Recent scope，并以 Codex home 作为内部锚点，实际恢复/继续会话始终使用 `thread/list` 返回的真实 `cwd`。
- Desktop 项目来自 `local-projects` 的名称和 `rootPaths`；thread 的最近归属来自 `projectless-thread-ids`，项目会话按项目 `rootPaths` 请求 app-server。
- 归档 thread 不进入项目树或“最近”，只进入已归档筛选。

一次本机快照中，app-server 返回 87 个活动 thread 和 76 个归档 thread；Desktop 状态包含 6 个项目、108 个 projectless 历史 ID，其中当前活动且属于“最近”的 thread 为 25 个。这说明 rollout 文件数、app-server 活动总数和 Desktop“最近”数量不是同一概念。

`Documents/Codex` 是 Desktop 为 projectless task 创建工作目录时常见的根目录，不是 thread 目录的权威数据库。thread 目录和消息继续通过 app-server 读取；DEEIX 不直接解析 `$CODEX_HOME/sessions` 或 `$CODEX_HOME/archived_sessions` JSONL。

## 2. 权威数据源

| 数据 | 权威来源 | DEEIX 用途 |
| --- | --- | --- |
| 活动/归档、标题、预览、`cwd`、详情与消息 | app-server `thread/list` / `thread/read` | Conversation 目录与历史 |
| Desktop 项目名称和根目录 | `.codex-global-state.json` 的 `local-projects` | Agent 同步可见 Workspace |
| Desktop 项目顺序/置顶 | Desktop 状态中的 `project-order`、`pinned-project-ids` | 仅属于 Desktop UI 元数据；当前不作为运行时 Workspace 排序依据 |
| 项目会话范围 | `local-projects.rootPaths` 与 app-server `thread/list` 的 cwd 过滤 | thread 进入一个项目 |
| 最近归属 | `projectless-thread-ids` 与 `thread-workspace-root-hints` | thread 进入“最近” |
| 归档 rollout 文件 | app-server 内部实现 | DEEIX 不直接读取 |

Desktop 状态只在 Agent 本机以配置用户身份读取。Cloud 只接收 opaque workspace/thread ref、名称、状态与时间摘要，不接收绝对路径或 Desktop 状态原文。

## 3. DEEIX 投影

Agent 为每个可见 Desktop 项目同步一个普通 Workspace。另为 projectless Recent 同步一个 `hidden` Workspace：

- 普通 Workspace 进入项目 API 和项目树。
- hidden Workspace 只承载 Recent thread 的执行绑定与会话投影，不进入项目 API。
- Gateway Conversation 只有绑定普通 Workspace 时才返回 `projectID/projectName`；绑定 hidden Workspace 时项目字段为空。
- 前端“最近”统一请求 `project=unassigned`，Cloud 按空项目过滤，Gateway 按 hidden Workspace 过滤。
- 置顶 Conversation 独立显示；归档 Conversation 从项目、置顶和最近中移除。
- Agent 为每个 Workspace 同步仅由 roots/thread membership 生成的 opaque revision；同一小时内项目归属或 Recent ID 变化也会产生新的 sessions 刷新命令。

Web 添加的本地文件夹使用同一个 Workspace 投影，但额外标记为 `managed`：

- 只有 `managed` Workspace 在工作项目菜单中显示“重命名”和“从 DEEIX 移除”；Desktop 自动发现的项目保持只读，避免建立第二套 Desktop 项目元数据。
- 重命名只修改 DEEIX/Agent 配置中的显示名称，不修改本地目录名或路径。
- 移除会在 Agent 配置中保存排除记录，再把 Cloud 投影标为不可用；本地目录、文件及 Codex 历史不做删除。
- 排除记录阻止 Desktop 状态或历史扫描在下一次刷新时立即把同一路径重新加入。用户后续从 Web 再次添加该目录时，登记命令会恢复它。
- rename/unregister 都通过 Bridge 持久命令执行并等待终态；服务端在入队和终态投影时校验用户、设备、Profile、Workspace、`managed` 标志及 manifest capability。

本机 provider thread ID 与 thread `cwd` 只保存在 Agent 内存/WAL source 映射中。Web 继续执行 projectless thread 时，Adapter 使用该 thread 的真实 `cwd` 调用 `thread/resume` 和 `turn/start`，不会把 Recent 根目录错误地当作所有 thread 的工作目录。

## 4. 不采用的做法

- 不把全局 `thread/list` 中出现过的每个 Git `cwd` 自动创建为 Desktop 项目。
- 不把项目 thread 同时复制到“最近”。
- 不用 rollout 文件数量推断侧栏任务数量。
- 不把 `Documents/Codex` 当作会话数据库。
- 不读取旧的 `electron-saved-workspace-roots` 字段；当前实现直接使用 `local-projects` 数据模型。

## 5. 验证

最小回归需要覆盖：当前 Desktop 状态解析、projectless 精确过滤、活动/归档分页、hidden Workspace 不进入项目列表、Gateway `project=unassigned` 仅返回 Recent、继续 projectless thread 时沿用其真实 `cwd`，以及 managed Workspace 重命名/移除后本地目录保持不变。

官方协议参考：[Codex app-server](https://developers.openai.com/codex/app-server/)；仓库锁定证据见 [codex-app-server-v0.147.0.lock.json](./codex-app-server-v0.147.0.lock.json)。
