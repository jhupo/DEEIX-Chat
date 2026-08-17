# Local Gateway 当前设计

## 1. 责任

Local Gateway 是本地 provider transport，不是另一套聊天后端：

```text
Conversation Gateway Adapter
  -> typed AgentCommand
  -> per-device durable queue
  -> authenticated WSS
  -> Local Bridge WAL
  -> Runtime Registry
  -> Codex ProviderAdapter
  -> codex app-server stdio
```

Cloud 持有用户、Conversation、执行绑定和业务投影；Bridge 持有本机进程、canonical path、provider raw ID、WAL 与 app-server 协议转换。

## 2. 多设备模型

一个用户可以有多台 Device，每台 Device 可以报告多个 Runtime Profile 和 Workspace。设备连接、credential、server sequence 与 bridge sequence 相互隔离。同一设备的新 WSS 接管旧连接，不同设备并发在线。

Conversation 创建时保存公开 `device_id/profile_id/workspace_id`。Cloud 每次创建首个 thread 前重新校验：

- Device 属于当前用户且未撤销。
- Runtime Profile 已完成准入证明且未过期。
- Workspace 属于同一设备和 Profile，状态可用。

聊天与工作是互斥的数据域。Conversation 列表、搜索和 Project 查询必须显式传入 `execution=cloud|gateway`；工作域还必须传入当前 `device`。聊天域返回 Cloud Conversation 和 Cloud Project；Gateway Adapter 将所选设备的 Workspace 和 Gateway Conversation 投影为相同 Project/Conversation DTO。切换模式或设备会重建数据作用域，界面不会同时合并或标注两套数据。

Bridge 启动后合并两类本机 Workspace 来源：无 `cwd` 过滤的 app-server `thread/list` 返回的历史线程目录，以及用户在 Web 中明确添加的文件夹。Agent 不读取 Codex Desktop 的 `.codex-global-state.json`：该文件是 Desktop UI 私有的项目、排序和置顶状态，不属于 app-server 协议，也不是会话历史的权威来源。Agent 安装目录由平台固定选择，安装命令不把终端当前目录注册成项目；`--workspace` 只保留给明确的手动注册。所有路径先在设备端 canonicalize，再按 opaque Workspace ID 去重；Cloud 只接收 ID、名称和可用状态，不接收本机绝对路径。没有历史的空 Workspace 需由用户在 Web 中明确添加；有线程历史的 Git 工作区会由 `thread/list` 自动发现。Cloud 的 Workspace 查询只返回当前所选 Device 上本次连接仍为 `available` 的记录，其他设备和旧目录均不进入结果。

`CODEX_HOME` 与 Codex Desktop 的程序安装目录是两件事。Bridge 在 `initialize` 结果中读取 app-server 返回的 `codexHome`；默认通常是当前登录用户的 `~/.codex` 或 `%USERPROFILE%\.codex`，但产品代码不猜测该路径。app-server 的 `thread/list` 默认读取状态数据库并扫描 JSONL rollout 修复目录元数据；活动 rollout 位于 `$CODEX_HOME/sessions`，归档 rollout 位于 `$CODEX_HOME/archived_sessions`，详情通过 `thread/read(includeTurns=true)` 读取。Windows 系统服务只负责守护，Codex 子进程使用登记用户的会话 token 和环境启动，因此读取的是该用户的 Codex home，而不是 `LocalSystem` 的配置目录。

创建请求只携带统一 `projectID` 和 `execution(type/device)`。Cloud Project 直接解析；Gateway Adapter 校验 Workspace 属于该用户和设备，解析对应 Profile 后写入 Conversation 执行绑定。Web 不提交 `profileID/workspaceID` 组合，也不直接请求 Workspace sessions 来拼装导航。

Workspace 的 `sessions` 刷新在 Bridge 内部消费 `thread/list` cursor，分别读取最多 500 个活动线程和 500 个归档线程，只上传 opaque thread ref、标题、预览、状态和时间摘要。Cloud 在资源终态事务中创建或更新 Conversation 与 AgentThread 目录投影，历史状态初始为 `unloaded`。用户打开会话时，Web 通过 Conversation history API 排队强类型 `thread.read`；Bridge 校验 source ref 后调用 `thread/read(includeTurns=true)`，只上传该线程裁剪后的用户/助手消息并把状态改为 `loaded`。provider raw ID、本地路径、命令输出和凭据留在设备。用户继续发送前 Bridge 先调用 `thread/resume`，再向同一个 app-server thread 发起 `turn/start`。

每次完成 Runtime proof 与 Workspace 同步后，Cloud 按设备、Profile/Workspace 和小时桶幂等下发 `apps`、`skills` 与 `sessions` 刷新。首次连接会自动导入历史和输入资源；同一小时内的 WSS 重连复用原命令，不重复扫描。

## 2.1 安装、注册与更新

账户页根据当前 DEEIX origin 与用户公开 ID 生成命令。命令在当前项目目录运行，下载站内 `/agent/install.sh` 或 `/agent/install.ps1`，再从同源 `/agent/releases/v<VERSION>/` 获取并校验平台包。稳定全量 Release 将 Windows x64、Linux x64 与 macOS arm64 Agent 包一并放入前端静态目录，用户机器不直接连接 GitHub。Agent 包只有原生可执行文件，不携带 Node.js 或 Codex；用户机器必须预先安装组件完整的官方 Codex CLI。安装时解析并保存 CLI 绝对路径，也可通过 `--codex` / `-Codex` 明确指定。

Agent 配置与 Ed25519 设备私钥保存在用户目录。安装脚本先在 staging 目录使用本机 Codex 完成版本、app-server 初始化、runtime proof 与引导目录校验，再原子替换程序目录。引导目录仅用于安装阶段的本地路径校验，不进入运行时 Project 列表。重复运行安装命令会停止原常驻进程、校验并覆盖程序、复用私钥完成幂等注册，最后用 systemd user service、LaunchAgent 或 Windows SCM 系统服务重新启动。Windows SCM 只承载一个 Agent 进程，并以安装用户的活动会话令牌启动 `codex app-server` 子进程。程序包与身份数据分目录，客户端更新不会改变设备 ID。

设备列表同时返回 Agent 当前版本和服务端版本。在线且版本落后时，Web 通过 `POST /api/v1/agent/devices/{device_id}/update` 入队 `agent.update`；该命令要求已认证 Profile 的 manifest 声明能力，并按设备幂等合并。Agent 先持久化待更新标记、确认命令，再让独立更新进程等待旧服务退出并重跑同源安装脚本。这样更新期间允许设备短暂离线，身份、配置、WAL、Workspace 与 Codex 登录状态不变；旧版本没有 `agent.update` 能力时，账户页仍提供一次手动安装升级入口。

## 3. 强类型命令

Conversation 当前只会创建：

- `thread.create`：创建 provider thread；可附带一个待启动的首 Turn。
- `turn.start`：在已有 thread 上执行。
- `turn.interrupt`：按 Run 中断活动 Turn。
- `interaction.respond`：回答 app-server server request。
- `resource.refresh`：刷新模型、sessions、skills、MCP、插件等只读快照；sessions 终态同时更新工作会话投影。
- `agent.update`：由 Web 设备管理触发，更新当前稳定 Release 的 Agent 原生包，不进入 Codex app-server。

应用服务没有接受任意 JSON 的通用入队方法。Bridge 内部 adapter 可以覆盖 app-server 更多原生方法，但新增产品能力必须先进入 Conversation 用例和强类型 Cloud command，不能直接开放成 Web provider RPC。

## 4. 引用与附件

Cloud command 只携带 opaque source ref、artifact ref 和 input resource ref。Conversation 发送前按当前用户、Device、Workspace 快照校验 Skill/App ref；Bridge 再从本机状态解析 skill path 或 App ID。Cloud 快照只保存名称、说明、类型和 ref，不保存本机绝对路径或 `app://id`。附件下载 grant 与 command、User、Device、Workspace、Artifact 和过期时间绑定，不写入持久 command payload 或 WAL。

## 5. 投影和恢复

Bridge 在 provider 调用前持久化 command receipt，在 provider event/result 后持久化上行 frame。Cloud 使用连续序列去重并确认；网络断开后双方从各自最后连续确认点恢复。

Agent repository 写事件时同时生成 Conversation projection outbox。投影失败不会确认 outbox；服务启动、终态处理和后续 frame 都会再次冲刷。Conversation projector 以 AgentEvent public ID 作为幂等源键。

## 6. Provider 扩展

未来 Claude 或其他本地 runtime 复用 `ProviderAdapter` 与 Runtime Registry：

- Adapter 输入是解析后的强类型 `ProviderCommand`。
- Adapter 输出 provider-neutral event/interaction。
- Provider capability 由 Profile resource snapshot 暴露。
- Web 仍只调用 Conversation API。

新增 Adapter 不增加 Conversation 类型，也不复制消息、Run、事件或交互表。
