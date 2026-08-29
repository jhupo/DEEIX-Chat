package middleware

const (
	// ContextKeyUserID 当前登录用户ID。
	ContextKeyUserID = "ctx_user_id"
	// ContextKeyUserPublicID is the current user's browser-safe public ID.
	ContextKeyUserPublicID = "ctx_user_public_id"
	// ContextKeyUsername 当前登录用户名。
	ContextKeyUsername = "ctx_username"
	// ContextKeyUserRole 当前登录用户角色。
	ContextKeyUserRole = "ctx_user_role"
	// ContextKeyAuthProvider 当前登录身份来源。
	ContextKeyAuthProvider = "ctx_auth_provider"
	// ContextKeySessionID 当前登录会话ID。
	ContextKeySessionID = "ctx_session_id"
	// ContextKeyRequestID 请求追踪ID。
	ContextKeyRequestID = "ctx_request_id"
	// ContextKeyTraceID 分布式链路追踪 ID（对齐 OpenTelemetry TraceID 格式）。
	ContextKeyTraceID = "ctx_trace_id"
)
