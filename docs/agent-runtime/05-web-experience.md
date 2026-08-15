# 统一会话 Web 设计

> 当前实现使用同一套 Conversation 页面承载聊天与工作。

## 信息架构

继续使用现有左侧 Conversation 列表，不新增 `/agent` 工作台，不增加独立任务入口，也不在会话条目上显示执行位置标签。聊天与工作一次只展示当前 execution context 的项目和会话，不混合两套数据；项目树、最近、置顶、搜索、分享和消息历史使用同一套组件与 Conversation API。

项目与对话是两个维度：Gateway 的 Project 来自 canonical `cwd`/Workspace，Conversation 来自 app-server thread。聊天与工作使用相同的“置顶、项目、最近”导航顺序；没有置顶会话时不显示“置顶”区。工作模式的“项目”区只显示 Workspace，不在项目节点下嵌套 thread，“置顶”和“最近”独立显示当前设备的 thread。点击项目进入按 Workspace 筛选的会话列表，项目右侧加号在该 Workspace 新建会话。Cloud 继续使用可展开项目树。前端不从标题猜测项目，也不为 Gateway 建第二套侧栏。

主输入框只保留两个显式资源触发器：`/` 打开 Skill 选择，`@` 打开 Plugin 能力选择。模型继续使用模型选择器，文件继续使用附件入口，Prompt 不再混入触发菜单。Cloud 当前将 Plugin 能力投影到可执行 MCP 工具；Gateway 的 Workspace Skill、Profile Plugin/App 仍须通过统一 Resource DTO 接入，页面不直接读取 `/agent/*`。

“插件”保留原 `/skills-prompt` 页面。页面未来通过统一 Resource API 获取平台或设备资源，不直接调用 Agent resource endpoint，也不增加设备专用插件页。

## 新建会话

主界面顶部使用“聊天 / 工作” segmented control：

1. 聊天：使用默认 Chat key 和管理员配置的模型。
2. 工作：使用账户默认设备及其可用 Workspace；没有可用设备时禁用。

切换执行位置会打开新 Conversation。“设置 -> 账户”在活跃会话上方显示活跃设备表，并提供 Windows、macOS、Linux 安装命令；左下用户菜单在语言项下方选择默认设备。

前端创建会话统一提交 `projectID + execution(type/device)`。Cloud 将 `projectID` 解析为 DEEIX Project；Gateway Adapter 将同一字段解析为所选设备的 Workspace/Profile，再把固定的 `deviceID + profileID + workspaceID` 执行绑定写入 Conversation。前端不组装 provider profile/workspace 参数。

目标在创建后锁定。切换目标创建新 Conversation，避免会话历史与本地 provider thread 脱钩。一个用户的多个设备按在线状态、名称和最近使用时间排序；设备离线时仍可查看历史，但发送按钮显示明确原因。

## 消息流

Cloud 与 Gateway 使用同一 composer 和消息列表：

- 文本 delta 更新 assistant message。
- reasoning 进入现有推理展示。
- tool/item 事件进入过程区。
- approval/input interaction 使用消息流内控件回答。
- terminal 后刷新最终 Message 与 Run。

协议名、raw thread ID、WSS 状态和命令序列不直接展示给普通用户。设备断开、等待审批、执行失败和取消是清晰的业务状态。

## 重连

页面先建立实时订阅，再用 `GET /conversations/:id/events?after=<lastSeq>` 补齐间隙。事件 reducer 只接受大于最后已应用序列的事件。完成事件后以服务端 Message/Run 为最终状态，不依赖浏览器拼接内容作为事实源。

## 可访问性

- 执行位置与设备选择控件有可见 label。
- 仅图标按钮有 accessible name 和 tooltip。
- 状态不只依赖颜色，必须有文字或图标含义。
- interaction 控件可键盘操作，错误与等待状态通过 live region 提示。
