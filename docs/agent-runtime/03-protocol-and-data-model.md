# 当前协议与数据模型

## 1. Conversation API

```text
POST /api/v1/conversations
POST /api/v1/conversations/:id/turns
POST /api/v1/conversations/:id/turns/stream
GET  /api/v1/conversations/:id/messages
GET  /api/v1/conversations/:id/history
POST /api/v1/conversations/:id/history
GET  /api/v1/conversations/:id/runs
GET  /api/v1/conversations/:id/events?after=N
GET  /api/v1/conversations/:id/interactions?status=
POST /api/v1/conversation-interactions/:interaction_id/respond
POST /api/v1/conversation-runs/:run_id/interrupt
```

创建 Cloud Conversation：

```json
{
  "title": "New chat",
  "model": "gpt-5.6",
  "execution": { "type": "cloud" }
}
```

创建 Gateway Conversation：

```json
{
  "title": "Local project",
  "model": "gpt-5.6-codex",
  "execution": {
    "type": "gateway",
    "deviceID": "agd_DEVICE",
    "profileID": "agrp_PROFILE",
    "workspaceID": "agw_WORKSPACE"
  }
}
```

Turn 请求对两种执行方式相同。Cloud 要求 Chat key binding；Gateway 使用已有本地 Runtime 准入，不读取或改写用户机器的 Codex 配置。Gateway 接受 text/markdown、附件和最多 16 个 `inputResourceRefs`；ref 必须属于该用户、设备、Workspace 的当前 Skill/App 快照。设置 allowlist 为 `reasoningEffort`、`approvalPolicy`、`sandboxPolicy`。

非流式 `/turns` 对两种执行方式都等待终态后返回；`/turns/stream` 对两种执行方式都输出 NDJSON 实时事件并在最后返回同一消息结果。

Conversation 目录使用服务端已投影的摘要。打开本地工作会话时，Web 先通过 `POST /conversations/:id/history` 准备历史，再用 `GET /conversations/:id/history` 轮询 `loaded|loading|error`，完成后继续调用原消息分页接口。Cloud 会话始终返回 `loaded`；页面不直接调用 Agent API。

工作输入资源通过 `GET /api/v1/conversation-input-resources?device=<id>&workspace=<id>` 读取。该接口属于 Conversation 业务面，由后端 adapter 合并 Profile App 与 Workspace Skill 的脱敏快照，并返回 `ready + items`；浏览器只接收并提交 opaque ref，在线设备未就绪时轮询至 `ready=true`。

## 2. 设备和资源 API

设备、Profile、Workspace 与资源选择仍属于控制面，不是第二套会话 API：

```text
GET    /api/v1/agent/devices
GET    /api/v1/agent/devices/:device_id
PATCH  /api/v1/agent/devices/:device_id
DELETE /api/v1/agent/devices/:device_id
GET    /api/v1/agent/devices/:device_id/profiles
GET    /api/v1/agent/devices/:device_id/workspaces
POST   /api/v1/agent/workspaces/:workspace_id/artifacts
GET    /api/v1/agent/devices/:device_id/profiles/:profile_id/resources/:resource
POST   /api/v1/agent/devices/:device_id/profiles/:profile_id/resources/:resource/refresh
GET    /api/v1/agent/devices/:device_id/workspaces/:workspace_id/resources/:resource
POST   /api/v1/agent/devices/:device_id/workspaces/:workspace_id/resources/:resource/refresh
```

Bridge 的设备注册不使用浏览器令牌或配对码。首次注册先调用
`POST /api/v1/agent/bridge/enrollment-challenges`，再调用
`POST /api/v1/agent/bridge/enrollments` 完成注册。请求只包含用户公开 ID、设备公钥与设备展示信息；Bridge 使用本机 Codex 当前 API key 计算 HMAC proof，并用设备 Ed25519 私钥签名同一 canonical challenge。

服务端以公开 ID 定位用户后，用该用户实时 Sub2 key 列表验证 proof。公开 ID 是稳定的外部关联标识，不是凭据；内部外键仍使用数据库 `user_id`。同一设备私钥重复完成注册返回原设备，已移除设备重新通过 proof 后恢复。安装命令不携带浏览器 token、Sub2 token 或 API key。

Bridge 线协议固定为 `deeix.bridge.v2`，所有帧的 `version` 固定为 `2`。v2 将完整 `ProviderManifest` 作为 runtime proof 的必填字段；服务端和 Bridge 都不保留 v1 分支。

## 3. 事件合同

`GET /conversations/:id/events?after=N` 返回：

```json
{
  "runID": "RUN",
  "seq": 17,
  "kind": "turn.completed",
  "payload": { "status": "completed" },
  "occurredAt": "2026-08-13T00:00:00Z"
}
```

`seq` 只在 Conversation 内递增；它与 Bridge 上行序列和 device 下行命令序列互相独立。客户端保存最后应用的 `seq`，重连后以 `after` 重放。非法游标返回 400。

需要用户输入或审批时，Gateway 事件投影为 Conversation interaction。响应接口要求 `Idempotency-Key`，并由 interaction ownership、状态和 source request ref 共同校验。

`AgentInteraction.kind` 使用六个稳定语义值：`command_approval`、`file_approval`、`user_input`、`permission`、`mcp_elicitation`、`dynamic_tool`。原始 app-server method 只在 Bridge 事件边界用于 allowlist 映射；数据库的 `request_json` 仅保存脱敏后的 request。响应先通过公开 union 校验，再在同一数据库事务内按已存 kind 校验 response kind，错误类型不会进入命令队列。

## 4. 数据关系

```text
Conversation 1:N Message
Conversation 1:N ChatRun
Conversation 1:N ConversationExecutionEvent

Gateway Conversation 1:1 AgentThread
AgentThread 1:N AgentTurn
AgentTurn.run_id 1:1 ChatRun.run_id
AgentThread 1:N AgentItem / AgentInteraction / AgentEvent
```

关键不变量：

- `AgentThread.conversation_id > 0` 且唯一。
- `AgentTurn.run_id` 非空且唯一。
- `AgentTurn.status` 的终态只允许 `completed|interrupted|failed`；失败同时保存规范化 error code/message。
- `AgentRuntimeProfile.manifest_json` 是 runtime proof 时严格校验并持久化的能力快照。
- Conversation event `source_key` 全局唯一，避免重复投影。
- Conversation event `(conversation_id, seq)` 唯一。
- Conversation 的 execution target 不在活动期修改。

## 5. 生命周期

```text
queued -> running -> success
                  -> error
                  -> canceled
```

首个 Gateway Turn 先原子创建普通用户消息、pending assistant 与 ChatRun，再事务化创建内部 AgentThread/AgentTurn/command；第二步失败时第一步会被明确终结为 error，不留下永久 pending。后续 Turn 复用 Conversation 对应的 AgentThread。interrupt 通过公开 `run_id` 找到内部 Turn 并只生成强类型 `turn.interrupt` 命令。
