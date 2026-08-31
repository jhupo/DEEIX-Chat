# Codex app-server 交互 UI 实施规范

> 状态：本次实施与逐项验收合同。
>
> 协议基线：Codex app-server `rust-v0.151.0`，以
> [codex-app-server-v0.151.0.lock.json](./codex-app-server-v0.151.0.lock.json) 为最终字段依据。
> 官方协议说明：https://developers.openai.com/codex/app-server/

## 1. 目标与边界

在现有 Gateway/Work 对话中补齐：模型与推理等级、三种审批与权限模式、Turn/Plan/命令/
文件/用量展示、六类 interaction、刷新恢复，以及真正的 `turn/steer`。

本次不新增第二套聊天页、任务中心或通用事件总线；不开放 lock 中的 `extension`/`disabled`
方法；浏览器不直连 app-server，也不接触 JSON-RPC request id。

## 2. 端到端链路

```text
Frontend
  -> DEEIX conversation / agent-gateway HTTP API
  -> Backend 严格校验、持久化、设备路由
  -> authenticated Agent Bridge
  -> deeix-agent 映射 app-server JSON-RPC
  -> Codex app-server 0.151.x (0.151.0 stable schema contract)

Codex notifications/server requests
  -> deeix-agent typed projection + opaque refs
  -> Backend AgentEvent/Interaction/ConversationExecutionEvent
  -> conversation stream + history APIs
  -> frontend agent-run-store
  -> 现有消息 trace 与审批控件
```

前端只消费 DEEIX DTO。协议版本、provider ID 映射、设备路由、幂等、审批状态和历史 cursor
由后端负责。

## 3. 复用已有能力

- `GET /api/v1/conversations/:id/events?after=`：持久 execution event 历史。
- `GET /api/v1/conversations/:id/interactions?status=`：交互列表。
- `POST /api/v1/conversation-interactions/:id/respond`：交互响应。
- Agent profile resource API：`models` 来自设备上的 `model/list`。
- `MessageProcessTrace` 和现有 Markdown renderer：统一工作日志与最终回答视觉。
- `ChatModelPicker`、`ChatModelConfig`、`InputGroup` 和现有 shadcn/ui primitives。

直接修改 Gateway 提交、事件流和消息渲染旧路径，不保留“Gateway 清空 model/options”等兼容分支。

## 4. 模型与 Turn 设置

模型目录按当前 `deviceId + profileId` 读取 manifest 和 `models` resource；没有 snapshot 时通过
现有 refresh API 排队刷新。模型 DTO 只保留标识、展示名、说明、默认标记、
`supportedReasoningEfforts` 和默认 effort。推理等级只展示所选模型实际支持的值。

```ts
type AgentTurnSettings = {
  model: string;
  reasoningEffort: "low" | "medium" | "high" | "xhigh";
  approvalPolicy: "on-request" | "never";
  approvalsReviewer: "user" | "auto_review";
  sandboxPolicy: "workspace-write" | "danger-full-access";
};
```

| DEEIX 字段 | app-server |
| --- | --- |
| `model` | `thread/start.model`、`turn/start.model` |
| `reasoningEffort` | `turn/start.effort` |
| `approvalPolicy` | `turn/start.approvalPolicy` |
| `approvalsReviewer` | `turn/start.approvalsReviewer` |
| `workspace-write` | `{ type: "workspaceWrite", networkAccess: false }` |
| `danger-full-access` | `{ type: "dangerFullAccess" }` |

Composer 设置是“下一轮 draft”；AgentTurn 保存提交时的 immutable settings snapshot。活动 Turn 中
禁用切换。已有会话保存最后使用的 Gateway 设置；新会话使用 profile/model 默认值。

## 5. 审批与权限模式

Composer 左下工具区使用一个组合式单选菜单：

| 模式 | approvalPolicy | approvalsReviewer | sandboxPolicy |
| --- | --- | --- | --- |
| 请求批准 | `on-request` | `user` | `workspace-write`，network off |
| 帮我批准 | `on-request` | `auto_review` | `workspace-write`，network off |
| 完全访问 | `never` | `user` | `danger-full-access` |

- 使用现有 `DropdownMenuRadioGroup/RadioItem`，不是三个独立开关。
- `auto_review` 只在 manifest 声明支持时展示，不静默降级。
- 首次选择“完全访问”必须经现有 `AlertDialog` 确认，使用 `destructive` 语义色。
- 活动 Turn 中禁用切换；菜单不替代 pending approval 卡片。

## 6. Interaction 闭环

| kind | UI | response |
| --- | --- | --- |
| `command_approval` | 命令、原因、接受/拒绝 | `approval` |
| `file_approval` | 文件变化、原因、接受/拒绝 | `approval` |
| `user_input` | questionRef 对应输入/选项 | `user-input` |
| `permission` | 权限摘要、turn/session scope | `permission` |
| `mcp_elicitation` | 提示及受控字段 | `mcp-elicitation` |
| `dynamic_tool` | 工具摘要、成功/失败和受控内容 | `dynamic-tool` |

```text
pending -> responding -> resolved
                     \-> failed
pending -- turn terminal/serverRequest resolved -> resolved
```

响应创建和 `pending -> responding` 原子化；所有 mutation 使用 `Idempotency-Key`。多标签并发时
只允许一个响应命令。Agent 端按六种 method 做专用 allowlist 投影，不能用通用敏感字段清洗
误删 command/file/question；provider request id 和原始 question id 只留在 Agent 私有映射中。

## 7. Task、步骤与执行轨迹

- 一个 UI“任务”对应一个 app-server Turn/`AgentTurn`，不创建第二个 Task 实体。
- Assistant message 通过现有 `runID` 绑定 Turn activity。
- 步骤只来自 `turn/plan/updated.plan[]`；`item/plan/delta` 只改善实时文字。
- 命令来自 `item/started|completed` 与 `item/commandExecution/outputDelta`。
- 文件来自 fileChange item、`item/fileChange/patchUpdated` 和 `turn/diff/updated`。
- `agentMessage.phase=commentary` 与 reasoning summary 进入同一个 `MessageProcessTrace`
  工作日志；原始 reasoning content 不展示，也不能跨 run 合并。
- 缺少 phase 的 `agentMessage` 按 `final_answer` 处理；只有 final answer delta 进入普通
  Assistant Markdown 正文，不能与 commentary 重复展示。
- 命令行默认只展示状态、耗时和命令摘要，悬停展示完整命令与工作目录，点击展开输出。
- Pending approval 放在匹配的命令或文件事件后；无法匹配时仍放在同一工作日志末尾。
- Turn 结束后在回答下方展示变更文件摘要；点击后复用右侧工作区的 Monaco Diff，移动端全屏展示。
- `thread/tokenUsage/updated.tokenUsage.total` 是 thread 累计量；消息元数据只能使用最新的 `last` 快照，不能把重复快照相加。
- `model/rerouted` 更新当前 Turn 实际模型，不修改下一轮 draft。
- 只有 `turn/completed` 能结束 Turn。

## 8. 实时事件与恢复

generation stream 的 `seq` 负责 Run 断线续流；持久 `ConversationExecutionEvent.Seq` 作为
`executionSeq` 随 `execution_event` 发送，负责 activity 幂等和历史补拉，两者不可混用。

```ts
type ConversationExecutionEvent = {
  type: "execution_event";
  seq: number;
  executionSeq: number;
  runID: string;
  kind: string;
  payload: unknown;
  occurredAt: string;
};
```

恢复顺序：加载 messages；从 `after=0` 分批读取 events；按 `runID + executionSeq` 写入 store；
加载 pending interactions；活动流继续应用新事件。`executionSeq` 重复时丢弃，出现 gap 时从最后
已应用水位补拉，不由浏览器猜测状态。

## 9. 组件与状态边界

允许新增以下 feature-local 文件：

- `components/sections/chat-agent-settings.tsx`
- `components/message/message-agent-activity.tsx`
- `components/message/message-agent-interaction.tsx`
- `hooks/use-agent-run-hydration.ts`
- `model/agent-run-store.ts`

`AppChatArea` 解析 device/profile/workspace 并管理下一轮设置；`ChatInput` 组合模型与审批模式；
`use-chat-message-submit` 快照 Gateway model/settings；store 直接规范化 allowlist events；消息组件
只负责渲染对应 run 的 activity 和 interaction。

## 10. 主题与组件约束

- 复用 `InputGroupButton`、`DropdownMenu`、`Popover`、`Accordion`、`Collapsible`、
  `AlertDialog`、`Tooltip`、`Badge`、`Button`、`Sheet`。
- 活动轨迹复用 `TRACE_ROOT_CLASS`、Marker、Accordion 和现有 tool trace 行布局。
- 只用 `bg-popover`、`bg-pure`、`text-foreground`、`text-muted-foreground`、`border-border`、
  `bg-accent`、`text-destructive`、`ring-ring` 等语义 token。
- 不新增 raw hex/OKLCH、审批专属主题、独立 timeline、menu、radio、spinner 或嵌套卡片。
- `--radius` 和 `--ui-font-scale` 由主题控制；窄屏长文本不得覆盖按钮。

## 11. `turn/steer`

```text
POST conversation active-run steer
  -> Conversation service 校验活动 Gateway Run
  -> AgentGateway queue turn.steer
  -> Agent Bridge
  -> app-server turn/steer { threadId, expectedTurnId, input }
```

steer 不创建新 Run/Turn，不先 interrupt。活动 Turn 已结束时返回 conflict，前端保留输入供用户
作为下一轮发送。

## 12. 逐功能验收

### A. 模型与推理

- [x] Gateway 显示设备 `model/list`，不再隐藏模型控件。
- [x] 所选模型只显示其支持的 reasoning efforts。
- [x] payload 和 Agent 映射包含正确 model/effort。
- [x] 离线/目录错误不自动换模型，错误态符合主题。

### B. 审批模式

- [x] 三种模式的四字段组合与表格一致。
- [x] `auto_review` capability gate 生效且不静默回退。
- [x] 完全访问首次确认、活动 Turn 禁用正确。
- [x] `danger-full-access` 映射为 `dangerFullAccess`，旧 validator 不再拒绝。

### C. Interaction

- [x] 六类请求均有控件和合法响应，非法字段被拒绝。
- [x] 必要展示字段存在，provider/RPC ID 不进入 Browser。
- [x] pending/responding/resolved/failed 和多标签竞态正确。
- [x] 刷新后 pending 恢复，设备重连后响应只投递一次。

### D. Task/步骤/命令/文件

- [x] Task=Turn，Plan、命令、文件和终态按 run 隔离。
- [x] 实时事件可见，刷新后从 history 恢复且不重复。
- [x] 空 Plan/Diff 不删除 item journal，移动端长文本不溢出。
- [x] `turn/completed` 前不提前结束。

### E. 调整方向与恢复

- [x] 调整方向只调用一次 `turn/steer`，不产生 interrupt + turn.start。
- [x] generation seq 与 executionSeq 各司其职，不丢事件。
- [x] 切换 conversation/device/profile/workspace 后旧 activity 不残留。

## 13. 定向验证策略

每完成 A-E 一个功能点立即对照对应清单核验。只运行相关 Go package 测试、公开 DTO 变动时的
`pnpm api:check`、前端 typecheck 和改动文件的 Biome 检查。手工覆盖 Gateway payload、审批继续
执行、刷新恢复、移动端和明暗主题。不运行全量 monorepo test/build；定向检查发现跨模块合同
问题时再扩大到对应模块。

### 13.1 本次验收记录（2026-08-22）

- 前端：`pnpm --filter @deeix/web typecheck` 与改动文件的 `biome check` 通过。
- API 合同：`pnpm api:check` 通过，Swagger 与 TypeScript 生成合同一致。
- Agent：user-input 投影、审批 item 关联、MCP schema 与 scalar 响应的定向测试通过。
- Gateway：设置 allowlist、六类 interaction request-aware 响应校验、原子响应与事件投影的定向测试通过。
- Conversation：`turn/steer` 仅调用一次 steer，未调用 start/interrupt；事件投影与重复发布保护测试通过。
- 恢复：execution history 排序/幂等、interaction 状态恢复、context 隔离与 gap 补拉经代码审计和定向测试核实。
- UI：真实组件在本地 320x800 与 1280x900 视口核验；模型/审批菜单、完全访问确认、六类 interaction、
  长文本、浅色/深色主题均无水平溢出。320px 下页面 `scrollWidth=320`，模型菜单宽度 256px。
- 最终 `git diff --check` 通过。未运行全量 monorepo test/build，未部署，也未触发在线更新。
