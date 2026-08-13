# 当前协议与数据模型

## 1. Conversation API

```text
POST /api/v1/conversations
POST /api/v1/conversations/:id/turns
POST /api/v1/conversations/:id/turns/stream
GET  /api/v1/conversations/:id/messages
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

Turn 请求对两种执行方式相同。Cloud 要求 Chat key binding；Gateway 使用已有本地 Runtime 准入，不读取或改写用户机器的 Codex 配置。Gateway 当前接受 text/markdown 与附件，设置 allowlist 为 `model`、`reasoningEffort`、`approvalPolicy`、`sandboxPolicy`。

非流式 `/turns` 对两种执行方式都等待终态后返回；`/turns/stream` 对两种执行方式都输出 NDJSON 实时事件并在最后返回同一消息结果。

## 2. 设备和资源 API

设备、Profile、Workspace 与资源选择仍属于控制面，不是第二套会话 API：

```text
POST   /api/v1/agent/devices/enrollments
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

Bridge 的 enroll、challenge、token、WSS 和 artifact content 路径只供本地进程使用。

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
