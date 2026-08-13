# 统一会话 Web 设计

> 当前实现使用同一套 Conversation 页面承载聊天与工作。

## 信息架构

继续使用现有左侧 Conversation 列表，不新增 `/agent` 工作台。每个会话条目在标题旁显示短标签：

- `聊天`：Sub2API Responses。
- `工作`：已注册设备上的 app-server。

标签表示执行位置，不表示 Conversation 类型。项目、置顶、搜索、分享、消息历史仍属于同一套会话界面。

## 新建会话

主界面顶部使用“聊天 / 工作” segmented control：

1. 聊天：使用默认 Chat key 和管理员配置的模型。
2. 工作：使用账户默认设备及其可用 Workspace；没有可用设备时禁用。

切换执行位置会打开新 Conversation。“设置 -> 账户”在活跃会话上方显示活跃设备表，并提供 Windows、macOS、Linux 安装命令；左下用户菜单在语言项下方选择默认设备。

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
