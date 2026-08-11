package response

import (
	"net/http"
	"strings"
)

const (
	CodeRequestInvalidBody       = "request.invalid_body"
	CodeRequestInvalid           = "request.invalid"
	CodeRequestInvalidID         = "request.invalid_id"
	CodeRequestInvalidQuery      = "request.invalid_query"
	CodeRequestRequired          = "request.required"
	CodeAuthUnauthorized         = "auth.unauthorized"
	CodeAuthForbidden            = "auth.forbidden"
	CodeAuthInvalidToken         = "auth.invalid_token"
	CodeAuthInvalidCredentials   = "auth.invalid_credentials"
	CodeAuthInvalidCurrentPass   = "auth.invalid_current_password"
	CodeAuthInvalidRefreshToken  = "auth.invalid_refresh_token"
	CodeAuthInvalidTwoFactorCode = "auth.invalid_two_factor_code"
	CodeAuthTwoFactorExpired     = "auth.two_factor_expired"
	CodeAuthSessionInvalid       = "auth.session_invalid"
	CodeResourceNotFound         = "resource.not_found"
	CodeResourceConflict         = "resource.conflict"
	CodeBillingPaymentRequired   = "billing.payment_required"
	CodeBillingInsufficientFunds = "billing.insufficient_funds"
	CodeRateLimitExceeded        = "rate_limit.exceeded"
	CodeQuotaExceeded            = "quota.exceeded"
	CodeFileInUse                = "file.in_use"
	CodeFileTooLarge             = "file.too_large"
	CodeFileNotReady             = "file.not_ready"
	CodeFileTypeBlocked          = "file.type_blocked"
	CodeUpstreamUnavailable      = "upstream.unavailable"
	CodeServiceUnavailable       = "service.unavailable"
	CodeInternal                 = "internal.error"
)

type errorSpec struct {
	Code    string
	Message string
}

var exactErrorSpecs = map[string]errorSpec{
	"unauthorized":                       {Code: CodeAuthUnauthorized, Message: "unauthorized"},
	"forbidden":                          {Code: CodeAuthForbidden, Message: "forbidden"},
	"admin permission required":          {Code: "auth.admin_required", Message: "admin permission required"},
	"superadmin permission required":     {Code: "auth.superadmin_required", Message: "superadmin permission required"},
	"missing authorization header":       {Code: CodeAuthInvalidToken, Message: "authorization header is required"},
	"invalid authorization header":       {Code: CodeAuthInvalidToken, Message: "invalid authorization header"},
	"invalid token":                      {Code: CodeAuthInvalidToken, Message: "invalid token"},
	"invalid token type":                 {Code: CodeAuthInvalidToken, Message: "invalid token type"},
	"session invalid":                    {Code: CodeAuthSessionInvalid, Message: "session invalid"},
	"invalid email or password":          {Code: CodeAuthInvalidCredentials, Message: "invalid email or password"},
	"invalid current password":           {Code: CodeAuthInvalidCurrentPass, Message: "invalid current password"},
	"invalid refresh token":              {Code: CodeAuthInvalidRefreshToken, Message: "invalid refresh token"},
	"session revoked":                    {Code: CodeAuthSessionInvalid, Message: "session invalid"},
	"invalid two factor code":            {Code: CodeAuthInvalidTwoFactorCode, Message: "invalid two factor code"},
	"two factor challenge expired":       {Code: CodeAuthTwoFactorExpired, Message: "two factor challenge expired"},
	"email registration is disabled":     {Code: "auth.email_registration_disabled", Message: "email registration is disabled"},
	"email already exists":               {Code: "auth.email_already_exists", Message: "email already exists"},
	"email verification is required":     {Code: "auth.email_verification_required", Message: "email verification is required"},
	"email domain is not allowed":        {Code: "auth.email_domain_not_allowed", Message: "email domain is not allowed"},
	"turnstile is not configured":        {Code: "auth.turnstile_not_configured", Message: "turnstile is not configured"},
	"turnstile verification is required": {Code: "auth.turnstile_required", Message: "turnstile verification is required"},
	"turnstile token is too long":        {Code: "auth.turnstile_invalid", Message: "turnstile token is invalid"},
	"turnstile verification failed":      {Code: "auth.turnstile_invalid", Message: "turnstile verification failed"},

	"invalid time zone":                 {Code: "user.invalid_time_zone", Message: "invalid time zone"},
	"invalid timezone":                  {Code: "user.invalid_time_zone", Message: "invalid time zone"},
	"invalid location":                  {Code: "user.invalid_location", Message: "invalid location"},
	"invalid avatar url":                {Code: "user.invalid_avatar_url", Message: "invalid avatar url"},
	"invalid display name":              {Code: "user.invalid_display_name", Message: "invalid display name"},
	"invalid user locale":               {Code: "user.invalid_locale", Message: "invalid user locale"},
	"superadmin management not allowed": {Code: "user.superadmin_management_protected", Message: "superadmin management is not allowed"},

	"invalid conversation title":                              {Code: "conversation.invalid_title", Message: "invalid conversation title"},
	"conversation has no titleable content":                   {Code: "conversation.no_titleable_content", Message: "conversation has no titleable content"},
	"invalid conversation share":                              {Code: "conversation_share.invalid", Message: "invalid conversation share"},
	"conversation share schema outdated":                      {Code: "conversation_share.schema_outdated", Message: "conversation share schema is outdated"},
	"conversation share schema is outdated, rebuild database": {Code: "conversation_share.schema_outdated", Message: "conversation share schema is outdated"},
	"message feedback target invalid":                         {Code: "message.feedback_target_invalid", Message: "message feedback target invalid"},
	"invalid message feedback":                                {Code: "message.invalid_feedback", Message: "invalid message feedback"},
	"invalid message content":                                 {Code: "message.invalid_content", Message: "invalid message content"},
	"message edit target invalid":                             {Code: "message.edit_target_invalid", Message: "message edit target invalid"},
	"message edit state invalid":                              {Code: "message.edit_state_invalid", Message: "message edit state invalid"},
	"invalid message branch":                                  {Code: "message.invalid_branch", Message: "invalid message branch"},
	"message generation canceled":                             {Code: "conversation_run.canceled", Message: "message generation canceled"},
	"too many files in one message":                           {Code: "message.too_many_files", Message: "too many files in one message"},
	"too many selected tools":                                 {Code: "message.too_many_selected_tools", Message: "too many selected tools"},
	"multiple image attachment processors selected":           {Code: "message.multiple_image_processors", Message: "select only one image attachment processor"},
	"image attachment processing failed":                      {Code: "mcp.image_processing_failed", Message: "image processing tool failed"},
	"too many selected skills":                                {Code: "message.too_many_selected_skills", Message: "too many selected skills"},
	"generation stream not found":                             {Code: "conversation_run.stream_not_found", Message: "generation stream not found"},
	"image prompt is required":                                {Code: "media.image_prompt_required", Message: "image prompt is required"},
	"image generation does not accept input images":           {Code: "media.image_generation_rejects_inputs", Message: "image generation does not accept input images"},
	"image edit requires at least one input image":            {Code: "media.image_edit_input_required", Message: "image edit requires at least one input image"},
	"too many image edit input images":                        {Code: "media.image_edit_too_many_inputs", Message: "too many image edit input images"},
	"image edit input image is invalid":                       {Code: "media.image_edit_input_invalid", Message: "image edit input image is invalid"},
	"video prompt is required":                                {Code: "media.video_prompt_required", Message: "video prompt is required"},
	"video generation input is invalid":                       {Code: "media.video_input_invalid", Message: "video generation input is invalid"},
	"too many video generation input images":                  {Code: "media.video_too_many_inputs", Message: "too many video generation input images"},
	"media route protocol does not match task":                {Code: "media.route_protocol_mismatch", Message: "media route protocol does not match task"},
	"invalid media generation task":                           {Code: "media.invalid_task", Message: "invalid media generation task"},
	"invalid mcp tool attachment configuration":               {Code: "mcp.invalid_attachment_configuration", Message: "invalid MCP tool attachment configuration"},

	"file is required":                                     {Code: "file.required", Message: "file is required"},
	"invalid file stream":                                  {Code: "file.invalid_stream", Message: "invalid file stream"},
	"invalid file reference":                               {Code: "file.invalid_reference", Message: "invalid file reference"},
	"invalid file name":                                    {Code: "file.invalid_name", Message: "invalid file name"},
	"storage quota exceeded":                               {Code: CodeQuotaExceeded, Message: "storage quota exceeded"},
	"dangerous file type not allowed":                      {Code: CodeFileTypeBlocked, Message: "file type is not allowed"},
	"mime blocked":                                         {Code: CodeFileTypeBlocked, Message: "file type is not allowed"},
	"embedding unavailable":                                {Code: "file.embedding_unavailable", Message: "embedding is unavailable"},
	"embedding unavailable for this file size":             {Code: "file.embedding_unavailable", Message: "embedding is unavailable for this file size"},
	"embedding unavailable for current file capability":    {Code: "file.embedding_unavailable", Message: "embedding is unavailable for current file capability"},
	"file is in use":                                       {Code: CodeFileInUse, Message: "file is in use"},
	"file too large":                                       {Code: CodeFileTooLarge, Message: "file too large"},
	"file processing not ready":                            {Code: CodeFileNotReady, Message: "file processing is not ready"},
	"file extract not ready":                               {Code: "file.extract_not_ready", Message: "file extract is not ready"},
	"file too large for full context":                      {Code: "file.too_large_for_context", Message: "file is too large for full context"},
	"at least one of file_name or rag_opt_out is required": {Code: CodeRequestRequired, Message: "at least one of file_name or rag_opt_out is required"},

	"permission group not found":                  {Code: "admin.permission_group_not_found", Message: "permission group not found"},
	"invalid permission group name":               {Code: "admin.invalid_permission_group_name", Message: "invalid permission group name"},
	"invalid permission group models":             {Code: "admin.invalid_permission_group_models", Message: "invalid permission group models"},
	"invalid permission group users":              {Code: "admin.invalid_permission_group_users", Message: "invalid permission group users"},
	"default permission group delete not allowed": {Code: "admin.default_permission_group_delete_not_allowed", Message: "default permission group cannot be deleted"},
	"default permission group users are implicit": {Code: "admin.default_permission_group_users_implicit", Message: "default permission group users are implicit"},

	"model route not configured":                  {Code: "llm.model_route_not_configured", Message: "model route is not configured"},
	"model access denied by group policy":         {Code: "llm.model_access_denied", Message: "you do not have access to this model"},
	"model returned empty response":               {Code: "llm.empty_response", Message: "model returned empty response"},
	"upstream returned empty response":            {Code: "llm.empty_response", Message: "model returned empty response"},
	"remote models unavailable":                   {Code: "llm.remote_models_unavailable", Message: "remote models unavailable"},
	"no active api key":                           {Code: "llm.no_active_api_key", Message: "no active api key"},
	"invalid adapter":                             {Code: "llm.invalid_adapter", Message: "invalid adapter"},
	"invalid compatible":                          {Code: "llm.invalid_compatible", Message: "invalid compatible"},
	"invalid json config":                         {Code: "config.invalid_json", Message: "invalid json config"},
	"invalid headers config":                      {Code: "llm.invalid_headers_config", Message: "invalid headers json config"},
	"invalid headers json config":                 {Code: "llm.invalid_headers_config", Message: "invalid headers json config"},
	"invalid api keys config":                     {Code: "llm.invalid_api_keys_config", Message: "invalid api keys config"},
	"invalid protocol defaults config":            {Code: "llm.invalid_protocol_defaults_config", Message: "invalid protocol defaults config"},
	"invalid kinds":                               {Code: "llm.invalid_kinds", Message: "invalid model kinds"},
	"invalid route protocol combination":          {Code: "llm.invalid_route_protocol_combination", Message: "invalid route protocol combination"},
	"invalid platform model name":                 {Code: "llm.invalid_platform_model_name", Message: "invalid platform model name"},
	"system prompt too long":                      {Code: "llm.system_prompt_too_long", Message: "system prompt too long"},
	"platform model name is required":             {Code: "llm.platform_model_name_required", Message: "platform model name is required"},
	"protocol required":                           {Code: "llm.protocol_required", Message: "protocol is required"},
	"platform model name already exists":          {Code: "llm.platform_model_name_exists", Message: "platform model name already exists"},
	"target model already bound on this upstream": {Code: "llm.route_conflict", Message: "target model is already bound on this upstream"},
	"all routes unavailable":                      {Code: "llm.routes_unavailable", Message: "all model routes are unavailable"},
	"upstream source unavailable":                 {Code: "llm.upstream_source_unavailable", Message: "upstream source unavailable"},
	"route not found":                             {Code: "route.not_found", Message: "route not found"},
	"api_keys is required":                        {Code: "llm.api_keys_required", Message: "api_keys is required"},

	"usage balance is insufficient":                                {Code: CodeBillingInsufficientFunds, Message: "insufficient balance"},
	"invalid redemption code":                                      {Code: "billing.invalid_redemption_code", Message: "invalid redemption code"},
	"payment is required":                                          {Code: CodeBillingPaymentRequired, Message: "payment is required"},

	"invalid namespace":                     {Code: "settings.invalid_namespace", Message: "invalid namespace"},
	"invalid setting key":                   {Code: "settings.invalid_key", Message: "invalid setting key"},
	"setting not found":                     {Code: "settings.not_found", Message: "setting not found"},
	"settings service unavailable":          {Code: "settings.service_unavailable", Message: "settings service unavailable"},
	"invalid id":                            {Code: CodeRequestInvalidID, Message: "invalid id"},
	"embedding service not available":       {Code: "embedding.service_unavailable", Message: "embedding service is not available"},
	"embedding service not configured":      {Code: "embedding.service_not_configured", Message: "embedding service is not configured"},
	"tika runtime service unavailable":      {Code: "runtime.tika_unavailable", Message: "tika runtime service unavailable"},
	"docling runtime service unavailable":   {Code: "runtime.docling_unavailable", Message: "docling runtime service unavailable"},
	"tesseract runtime service unavailable": {Code: "runtime.tesseract_unavailable", Message: "tesseract runtime service unavailable"},
	"rapidocr runtime service unavailable":  {Code: "runtime.rapidocr_unavailable", Message: "rapidocr runtime service unavailable"},
	"mineru runtime service unavailable":    {Code: "runtime.mineru_unavailable", Message: "mineru runtime service unavailable"},

	"memory_key is required":          {Code: "memory.key_required", Message: "memory_key is required"},
	"invalid mcp server id":           {Code: "mcp.server.invalid_id", Message: "invalid mcp server id"},
	"invalid mcp tool id":             {Code: "mcp.tool.invalid_id", Message: "invalid mcp tool id"},
	"invalid mcp server name":         {Code: "mcp.invalid_server_name", Message: "invalid mcp server name"},
	"invalid mcp server base url":     {Code: "mcp.invalid_server_base_url", Message: "invalid mcp server base url"},
	"invalid mcp server status":       {Code: "mcp.invalid_server_status", Message: "invalid mcp server status"},
	"invalid mcp server headers json": {Code: "mcp.invalid_server_headers", Message: "invalid mcp server headers json"},
	"invalid mcp tool status":         {Code: "mcp.invalid_tool_status", Message: "invalid mcp tool status"},
	"invalid mcp tool display name":   {Code: "mcp.invalid_tool_name", Message: "invalid mcp tool display name"},
	"invalid mcp tool description":    {Code: "mcp.invalid_tool_description", Message: "invalid mcp tool description"},
	"invalid mcp tool selection":      {Code: "mcp.invalid_tool_selection", Message: "invalid mcp tool selection"},
	"mcp client unavailable":          {Code: "mcp.client_unavailable", Message: "mcp client unavailable"},

	"rate limit exceeded":              {Code: CodeRateLimitExceeded, Message: "rate limit exceeded"},
	"too many refresh attempts":        {Code: "rate_limit.refresh_exceeded", Message: "too many refresh attempts"},
	"too many authentication attempts": {Code: "rate_limit.authentication_exceeded", Message: "too many authentication attempts"},
}

// InferErrorCode provides a compatibility code for legacy response.Error calls.
// New code should prefer ErrorWithCode/ErrorWithDetails with an explicit code.
func InferErrorCode(status int, msg string) string {
	if spec, ok := resolveErrorSpec(status, msg); ok {
		return spec.Code
	}
	switch {
	case status == http.StatusBadGateway:
		return CodeUpstreamUnavailable
	case status == http.StatusServiceUnavailable:
		return CodeServiceUnavailable
	case status >= http.StatusInternalServerError:
		return CodeInternal
	}
	text := normalizeErrorText(msg)
	switch {
	case strings.Contains(text, "invalid request body"):
		return CodeRequestInvalidBody
	case strings.Contains(text, "invalid ") && strings.Contains(text, " id"):
		return invalidIDCode(text)
	case strings.Contains(text, "is required"):
		return CodeRequestRequired
	case strings.Contains(text, "not found"):
		return notFoundCode(text)
	case strings.Contains(text, "already exists") || strings.Contains(text, "conflict"):
		return CodeResourceConflict
	case strings.Contains(text, "quota exceeded") || strings.Contains(text, "exceeded"):
		return CodeQuotaExceeded
	case strings.Contains(text, "insufficient"):
		return CodeBillingInsufficientFunds
	case strings.Contains(text, "payment required"):
		return CodeBillingPaymentRequired
	case strings.Contains(text, "file too large"):
		return CodeFileTooLarge
	case strings.Contains(text, "file processing not ready") || strings.Contains(text, "file extract not ready"):
		return CodeFileNotReady
	case strings.Contains(text, "mime blocked") || strings.Contains(text, "dangerous file type"):
		return CodeFileTypeBlocked
	case strings.Contains(text, "model access denied by group policy"):
		return "llm.model_access_denied"
	case strings.Contains(text, "remote models unavailable") || strings.Contains(text, "model route not configured"):
		return CodeUpstreamUnavailable
	case strings.Contains(text, "verification code"):
		return "auth.verification_code_invalid"
	}

	switch status {
	case http.StatusBadRequest:
		return CodeRequestInvalid
	case http.StatusUnauthorized:
		return CodeAuthUnauthorized
	case http.StatusForbidden:
		return CodeAuthForbidden
	case http.StatusNotFound:
		return CodeResourceNotFound
	case http.StatusConflict:
		return CodeResourceConflict
	case http.StatusPaymentRequired:
		return CodeBillingPaymentRequired
	case http.StatusTooManyRequests:
		return CodeRateLimitExceeded
	case http.StatusBadGateway:
		return CodeUpstreamUnavailable
	case http.StatusServiceUnavailable:
		return CodeServiceUnavailable
	default:
		if status >= http.StatusInternalServerError {
			return CodeInternal
		}
		return CodeRequestInvalid
	}
}

// PublicErrorMessage normalizes legacy handler messages into a safe API fallback.
// It intentionally preserves client-side validation context while hiding 5xx
// internals behind requestId + server logs.
func PublicErrorMessage(status int, code string, msg string) string {
	msg = strings.TrimSpace(msg)
	if spec, ok := resolveErrorSpec(status, msg); ok {
		return spec.Message
	}
	if msg == "" {
		msg = fallbackMessage(status, code)
	}

	switch {
	case status >= http.StatusInternalServerError:
		return fallbackMessage(status, code)
	case status == http.StatusBadGateway:
		return fallbackMessage(status, code)
	case status == http.StatusServiceUnavailable:
		return fallbackMessage(status, code)
	}

	switch code {
	case CodeAuthUnauthorized:
		return "unauthorized"
	case CodeAuthForbidden:
		return "forbidden"
	case CodeRateLimitExceeded:
		return "rate limit exceeded"
	default:
		return msg
	}
}

func fallbackMessage(status int, code string) string {
	if msg, ok := fallbackMessages[code]; ok {
		return msg
	}
	switch code {
	case CodeRequestInvalidBody:
		return "invalid request body"
	case CodeRequestInvalidID:
		return "invalid id"
	case CodeRequestRequired:
		return "required field missing"
	case CodeAuthUnauthorized:
		return "unauthorized"
	case CodeAuthForbidden:
		return "forbidden"
	case CodeAuthInvalidToken:
		return "invalid token"
	case CodeAuthInvalidCredentials:
		return "invalid email or password"
	case CodeAuthInvalidCurrentPass:
		return "invalid current password"
	case CodeAuthInvalidRefreshToken:
		return "invalid refresh token"
	case CodeAuthInvalidTwoFactorCode:
		return "invalid two factor code"
	case CodeAuthTwoFactorExpired:
		return "two factor challenge expired"
	case CodeAuthSessionInvalid:
		return "session invalid"
	case CodeResourceNotFound:
		return "resource not found"
	case CodeResourceConflict:
		return "resource conflict"
	case CodeBillingInsufficientFunds:
		return "insufficient balance"
	case CodeBillingPaymentRequired:
		return "payment required"
	case CodeQuotaExceeded:
		return "quota exceeded"
	case CodeFileTooLarge:
		return "file too large"
	case CodeFileNotReady:
		return "file is not ready"
	case CodeFileTypeBlocked:
		return "file type is not allowed"
	case CodeUpstreamUnavailable:
		return "upstream service unavailable"
	case CodeServiceUnavailable:
		return "service unavailable"
	}
	switch status {
	case http.StatusBadRequest:
		return "invalid request"
	case http.StatusUnauthorized:
		return "unauthorized"
	case http.StatusForbidden:
		return "forbidden"
	case http.StatusNotFound:
		return "resource not found"
	case http.StatusConflict:
		return "resource conflict"
	case http.StatusPaymentRequired:
		return "payment required"
	case http.StatusTooManyRequests:
		return "rate limit exceeded"
	case http.StatusBadGateway:
		return "upstream service unavailable"
	case http.StatusServiceUnavailable:
		return "service unavailable"
	default:
		if status >= http.StatusInternalServerError {
			return "internal server error"
		}
		return "request failed"
	}
}

var fallbackMessages = map[string]string{
	CodeRequestInvalidQuery:                             "invalid query parameter",
	"auth.admin_required":                               "admin permission required",
	"auth.superadmin_required":                          "superadmin permission required",
	"auth.verification_code_invalid":                    "verification code is invalid or expired",
	"auth.verification_code_recent":                     "verification code was sent recently",
	"auth.email_registration_disabled":                  "email registration is disabled",
	"auth.email_already_exists":                         "email already exists",
	"auth.email_verification_required":                  "email verification is required",
	"auth.email_domain_not_allowed":                     "email domain is not allowed",
	"user.invalid_time_zone":                            "invalid time zone",
	"user.invalid_avatar_url":                           "invalid avatar url",
	"user.invalid_location":                             "invalid location",
	"user.invalid_locale":                               "invalid user locale",
	"user.superadmin_management_protected":              "superadmin management is not allowed",
	"conversation.invalid_id":                           "invalid conversation id",
	"conversation.not_found":                            "conversation not found",
	"conversation.invalid_title":                        "invalid conversation title",
	"conversation.no_titleable_content":                 "conversation has no titleable content",
	"conversation_share.invalid":                        "invalid conversation share",
	"conversation_share.not_found":                      "conversation share not found",
	"conversation_share.invalid_id":                     "invalid share id",
	"message.invalid_id":                                "invalid message id",
	"message.not_found":                                 "message not found",
	"file.invalid_id":                                   "invalid file id",
	"file.not_found":                                    "file not found",
	"file.required":                                     "file is required",
	"file.invalid_stream":                               "invalid file stream",
	"file.invalid_reference":                            "invalid file reference",
	"context_artifact.invalid_id":                       "invalid context artifact id",
	"context_artifact.not_found":                        "context artifact not found",
	"admin.permission_group_not_found":                  "permission group not found",
	"admin.invalid_permission_group_name":               "invalid permission group name",
	"admin.invalid_permission_group_models":             "invalid permission group models",
	"admin.invalid_permission_group_users":              "invalid permission group users",
	"admin.default_permission_group_delete_not_allowed": "default permission group cannot be deleted",
	"admin.default_permission_group_users_implicit":     "default permission group users are implicit",
	"llm.model_route_not_configured":                    "model route is not configured",
	"llm.model_access_denied":                           "you do not have access to this model",
	"llm.remote_models_unavailable":                     "remote models unavailable",
	"llm.no_active_api_key":                             "no active api key",
	"llm.invalid_adapter":                               "invalid adapter",
	"llm.invalid_compatible":                            "invalid compatible",
	"llm.invalid_platform_model_name":                   "invalid platform model name",
	"llm.invalid_route_protocol_combination":            "invalid route protocol combination",
	"llm.system_prompt_too_long":                        "system prompt too long",
	"llm.platform_model_name_required":                  "platform model name is required",
	"llm.protocol_required":                             "protocol is required",
	"media.artifact_unavailable":                        "generated media artifact is temporarily unavailable",
	"media.image_stream_unsupported":                    "upstream may not support image streaming; disable image.stream for this model",
	"settings.invalid_namespace":                        "invalid namespace",
	"settings.invalid_key":                              "invalid setting key",
	"settings.not_found":                                "setting not found",
	"settings.invalid_value":                            "invalid setting value",
	"settings.smtp_invalid":                             "invalid smtp settings",
	"settings.model_option_policy_invalid":              "invalid model option policy settings",
	"settings.embedding_invalid":                        "invalid embedding settings",
	"settings.extract_invalid":                          "invalid file extraction settings",
	"embedding.service_unavailable":                     "embedding service is not available",
	"embedding.service_not_configured":                  "embedding service is not configured",
	"user_settings.unknown_key":                         "unknown setting key",
	"user_settings.invalid_value":                       "invalid user setting value",
	"memory.key_required":                               "memory_key is required",
	"rate_limit.refresh_exceeded":                       "too many refresh attempts",
	"rate_limit.authentication_exceeded":                "too many authentication attempts",
	"cors.origin_forbidden":                             "origin is not allowed",
}

func resolveErrorSpec(status int, msg string) (errorSpec, bool) {
	text := normalizeErrorText(msg)
	if text == "" {
		return errorSpec{}, false
	}
	if spec, ok := exactErrorSpecs[text]; ok {
		return spec, true
	}
	if strings.HasPrefix(text, "invalid setting: ") {
		detail := strings.TrimSpace(strings.TrimPrefix(text, "invalid setting: "))
		switch {
		case strings.HasPrefix(detail, "invalid namespace:"):
			return errorSpec{Code: "settings.invalid_namespace", Message: detail}, true
		case strings.HasPrefix(detail, "invalid setting key:"):
			return errorSpec{Code: "settings.invalid_key", Message: detail}, true
		case strings.Contains(detail, "smtp"):
			return errorSpec{Code: "settings.smtp_invalid", Message: detail}, true
		case strings.Contains(detail, "model_option_"):
			return errorSpec{Code: "settings.model_option_policy_invalid", Message: detail}, true
		case strings.Contains(detail, "embedding") || strings.Contains(detail, "rag") || strings.Contains(detail, "semantic"):
			return errorSpec{Code: "settings.embedding_invalid", Message: detail}, true
		case strings.Contains(detail, "extract:"):
			return errorSpec{Code: "settings.extract_invalid", Message: detail}, true
		default:
			return errorSpec{Code: "settings.invalid_value", Message: detail}, true
		}
	}
	if strings.HasPrefix(text, "unknown setting key:") {
		return errorSpec{Code: "user_settings.unknown_key", Message: "unknown setting key"}, true
	}
	if strings.HasPrefix(text, "invalid value for ") {
		return errorSpec{Code: "user_settings.invalid_value", Message: text}, true
	}
	if strings.HasPrefix(text, "invalid ") && strings.HasSuffix(text, " id") {
		return errorSpec{Code: invalidIDCode(text), Message: text}, true
	}
	if strings.HasPrefix(text, "invalid ") && strings.HasSuffix(text, "_id") {
		return errorSpec{Code: CodeRequestInvalidID, Message: text}, true
	}
	if strings.HasPrefix(text, "invalid ") {
		return errorSpec{Code: "request.invalid_" + slug(strings.TrimPrefix(text, "invalid ")), Message: text}, true
	}
	if strings.HasSuffix(text, " not found") {
		return errorSpec{Code: notFoundCode(text), Message: text}, true
	}
	if strings.HasSuffix(text, " already exists") {
		return errorSpec{Code: slug(strings.TrimSuffix(text, " already exists")) + ".already_exists", Message: text}, true
	}
	if strings.Contains(text, "verification code is invalid or expired") {
		return errorSpec{Code: "auth.verification_code_invalid", Message: "verification code is invalid or expired"}, true
	}
	if strings.Contains(text, "verification code was sent recently") {
		return errorSpec{Code: "auth.verification_code_recent", Message: "verification code was sent recently"}, true
	}
	if strings.Contains(text, "verification code attempts exceeded") {
		return errorSpec{Code: "auth.verification_code_attempts_exceeded", Message: "verification code attempts exceeded"}, true
	}
	if strings.Contains(text, "email already exists") {
		return errorSpec{Code: "auth.email_already_exists", Message: "email already exists"}, true
	}
	if strings.Contains(text, "smtp") {
		return errorSpec{Code: "settings.smtp_invalid", Message: text}, true
	}
	if strings.Contains(text, "payment") || strings.Contains(text, "stripe") || strings.Contains(text, "checkout") {
		return errorSpec{Code: CodeBillingPaymentRequired, Message: fallbackMessage(status, CodeBillingPaymentRequired)}, true
	}
	if strings.Contains(text, "required") {
		return errorSpec{Code: CodeRequestRequired, Message: text}, true
	}
	return errorSpec{}, false
}

func normalizeErrorText(msg string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(msg)), " "))
}

func invalidIDCode(text string) string {
	resource := strings.TrimPrefix(text, "invalid ")
	resource = strings.TrimSuffix(resource, " id")
	resource = strings.TrimSpace(resource)
	if resource == "" {
		return CodeRequestInvalidID
	}
	return slug(resource) + ".invalid_id"
}

func notFoundCode(text string) string {
	resource := strings.TrimSuffix(text, " not found")
	resource = strings.TrimSpace(resource)
	if resource == "" || resource == "resource" || resource == "record" {
		return CodeResourceNotFound
	}
	return slug(resource) + ".not_found"
}

func slug(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "resource"
	}
	replacer := strings.NewReplacer(
		" ", "_",
		"-", "_",
		".", "_",
		":", "_",
		"/", "_",
		"\\", "_",
	)
	return replacer.Replace(value)
}
