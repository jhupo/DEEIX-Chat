# DEEIX Native Agent

## 目标

本地网关只发布一个原生可执行文件：Windows 为 `deeix-agent.exe`，Linux 和 macOS 为 `deeix-agent`。Agent 使用用户机器上已经安装并登录的官方 Codex CLI，通过 stdio 启动 `codex app-server`。发布包不包含 Node.js、JavaScript 运行时或 Codex 运行时。

```text
DEEIX Web
  -> Conversation / Turn API
  -> Agent Gateway (HTTPS + WSS)
  -> deeix-agent (native Go process)
  -> codex app-server (local stdio JSONL)
  -> local workspace
```

服务端继续使用 `deeix.bridge.v2`。原生 Agent 是该协议唯一的客户端实现，不保留 Node Bridge、双协议分支或运行时回退。

## 发布文件

每个稳定 Release 生成以下文件，并把同一份文件放进 Full 应用包的 `/agent/releases/current`：

| 平台 | 文件 |
| --- | --- |
| Windows x64 | `deeix-agent-windows-x64.exe` |
| Linux x64 | `deeix-agent-linux-x64` |
| macOS Apple Silicon | `deeix-agent-macos-arm64` |

每个文件都有同名 `.sha256`。安装脚本先下载到临时目录、校验 SHA-256、执行配置和认证检查，再停止旧服务并替换程序。新进程没有产生运行状态时，安装器恢复旧二进制。

## 安装与更新

Web 的“添加设备”仍生成一条命令：

```powershell
& ([scriptblock]::Create((irm 'SERVER/agent/install.ps1'))) -Server 'SERVER' -User 'PUBLIC_USER_ID' -Workspace (Get-Location).Path
```

```bash
curl -fsSL 'SERVER/agent/install.sh' | sh -s -- --server 'SERVER' --user 'PUBLIC_USER_ID' --workspace "$PWD"
```

安装命令是幂等更新入口：

- 首次执行创建 Ed25519 设备身份，通过本机 Codex API key 完成一次性 enrollment challenge，并保存服务端返回的 `deviceId`。
- 再次执行且 `server + user` 相同时复用设备身份和 `deviceId`，更新原生程序、Codex CLI 绝对路径，或追加/刷新当前工作区。
- 已有身份与传入的 server 或 user 不一致时立即终止，不重绑设备。
- 已有设备配置缺少身份私钥时立即报错，不生成新私钥冒充原设备。
- Windows 注册自动启动的 `DEEIX Agent` 系统服务。SCM 只运行一个 `deeix-agent.exe` 服务进程；该进程在已登记用户登录后，以该用户令牌启动唯一必要的 `codex app-server` 子进程，因此项目、Git、MCP、Skills 和 Codex 登录状态仍来自该用户，且不会显示控制台窗口。
- Linux 使用 `systemd --user` 的 `deeix-agent.service`。
- macOS 使用 `com.deeix.agent` LaunchAgent。

远程服务地址必须使用 HTTPS；仅本机开发地址 `localhost` 或 loopback IP 可使用 HTTP。操作系统进程名为 `deeix-agent`，Windows 服务名为 `DEEIXAgent`、显示名为 `DEEIX Agent`。任务管理器中只有一个 `deeix-agent.exe`；运行本地 Codex 时出现的 `codex.exe app-server` 是 provider 子进程，不是第二个 Agent。

用户数据和程序分开保存。Windows 数据目录默认为 `%LOCALAPPDATA%\DEEIX\Agent`，Linux 为 `~/.local/share/deeix-agent`，macOS 为 `~/Library/Application Support/DEEIX/Agent`。更新程序不会删除身份、配置、sourceRef、命令终态或未确认事件。

## 本机前置条件

Agent 使用本机官方独立 Codex CLI。`codex --version` 必须可执行，`codex app-server` 必须能初始化，且 `getAuthStatus(includeToken=true)` 必须返回 API key 登录。Codex Desktop 安装目录中的内部资源文件不作为独立 CLI 使用。

安装时把解析后的 Codex CLI 绝对路径写入配置，后台服务不依赖交互式终端的 PATH。用户升级 Codex CLI 后再次执行同一条安装命令即可刷新路径并验证 app-server。

## 命令

```text
deeix-agent install --server URL --user PUBLIC_ID --workspace PATH [--name NAME] [--codex PATH]
deeix-agent start
deeix-agent doctor
deeix-agent status
deeix-agent update
deeix-agent uninstall [--purge]
deeix-agent version
```

`doctor` 验证配置、设备身份、工作区、Codex CLI、app-server 初始化和设备连接凭据。`status` 读取 `runtime-status.json`，报告 PID、连接状态、Codex 版本、最后错误和更新时间。`update` 使用保存的 server、user、Codex 路径与首个工作区重新执行带校验和回滚的安装流程。`uninstall` 默认保留身份和状态，显式 `--purge` 才删除数据目录。详细运行日志在数据目录的 `agent.log`。

## app-server 映射

原生适配器实现现有 Gateway 合同中的全部命令：thread 创建、恢复、分叉、归档、取消归档、删除、重命名、Git 元数据、压缩和按需历史读取；turn 启动、steer、中断；review；交互响应；profile/workspace 资源刷新。

资源包括模型、模型能力、权限方案、Apps、MCP、Plugins、认证状态、本地会话及消息历史、Skills 和 Hooks。通知包括消息增量、reasoning、plan、diff、文件 patch、goal、token usage、生命周期与告警。命令审批、文件审批、用户输入、权限申请、MCP elicitation 和动态工具调用通过 `sourceRequestRef` 往返映射。

本地绝对路径、Codex token、credential、authorization 和 secret 字段不会发到服务端。附件限定在已注册工作区的 `.deeix/artifacts`，下载后校验声明大小与 SHA-256。

## 持久化与恢复

`state.json` 使用私有权限和原子替换，统一保存：

- 已收件的 `serverSeq`
- 未被服务端确认的 terminal/event `bridgeSeq`
- command 原文、状态和附件 grant
- provider ID 与 opaque sourceRef 的双向映射

进程退出前尚未开始的命令会继续执行。已经开始的资源刷新、重命名、Git 元数据、interrupt 和非 fork 生命周期命令可重放；其他执行返回 `outcome_unknown`，避免重复创建 thread、turn、review 或工具副作用。服务端确认后的上行帧会被压缩清理。

## 验证

```bash
cd backend
go test ./internal/agentclient ./cmd/deeix-agent
go vet ./internal/agentclient ./cmd/deeix-agent
```

CI 还会以 `CGO_ENABLED=0` 构建 Windows amd64、Linux amd64 和 Darwin arm64，保证 Release 中的文件是独立原生可执行程序。
