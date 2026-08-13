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

## 2.1 安装、注册与更新

账户页根据当前 DEEIX origin 与用户公开 ID 生成命令。命令在当前项目目录运行，下载站内 `/agent/install.sh` 或 `/agent/install.ps1`，再从同源 `/agent/releases/v<VERSION>/` 获取并校验平台包。稳定全量 Release 将 Windows x64、Linux x64 与 macOS arm64 Bridge 包一并放入前端静态目录，用户机器不直接连接 GitHub。

Bridge 配置与 Ed25519 设备私钥保存在用户目录。重复运行安装命令会停止原常驻任务、校验并覆盖程序、复用私钥完成幂等注册、增加或更新当前 Workspace，最后用 systemd user service、LaunchAgent 或 Windows 当前用户计划任务重新启动。程序包与身份数据分目录，客户端更新不会改变设备 ID。

## 3. 强类型命令

Conversation 当前只会创建：

- `thread.create`：创建 provider thread；可附带一个待启动的首 Turn。
- `turn.start`：在已有 thread 上执行。
- `turn.interrupt`：按 Run 中断活动 Turn。
- `interaction.respond`：回答 app-server server request。
- `resource.refresh`：刷新模型、skills、MCP、插件等只读快照。

应用服务没有接受任意 JSON 的通用入队方法。Bridge 内部 adapter 可以覆盖 app-server 更多原生方法，但新增产品能力必须先进入 Conversation 用例和强类型 Cloud command，不能直接开放成 Web provider RPC。

## 4. 引用与附件

Cloud command 只携带 opaque source ref 和 artifact ref。Bridge 在执行前校验 source mapping、Workspace 根目录和 symlink 边界，再解析 raw provider ID 与本地文件。下载 grant 与 command、User、Device、Workspace、Artifact 和过期时间绑定，不写入持久 command payload 或 WAL。

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
