package conversation

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	appconversation "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/conversation"
	model "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/conversation"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/response"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/transport/http/middleware"
	"github.com/gin-gonic/gin"
)

const (
	resumeActiveCheckInterval         = 5 * time.Second
	usageAuthorizationRenewalInterval = 30 * time.Minute
)

var reservedMessageOptionKeys = map[string]struct{}{
	"contents":          {},
	"instructions":      {},
	"input":             {},
	"messages":          {},
	"model":             {},
	"prompt":            {},
	"stream":            {},
	"system":            {},
	"systemInstruction": {},
}

func shouldPersistStreamEvents(executionType string) bool {
	return executionType != model.ExecutionTypeGateway
}

func sanitizeMessageOptions(options map[string]interface{}) map[string]interface{} {
	if len(options) == 0 {
		return nil
	}
	sanitized := make(map[string]interface{}, len(options))
	for key, value := range options {
		if _, ok := reservedMessageOptionKeys[key]; ok {
			continue
		}
		sanitized[key] = value
	}
	if len(sanitized) == 0 {
		return nil
	}
	return sanitized
}

// parseSendMessageInput 解析消息发送请求的公共参数。
func (h *Handler) parseSendMessageInput(c *gin.Context) (appconversation.SendMessageInput, *model.Conversation, *SendMessageRequest, error) {
	userID := middleware.MustUserID(c)
	publicID, err := stringParam(c, "id")
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid conversation id")
		return appconversation.SendMessageInput{}, nil, nil, err
	}

	var req SendMessageRequest
	if err = bindConversationJSON(c, &req, 1024*1024+128*1024); err != nil {
		response.InvalidRequestBody(c, err)
		return appconversation.SendMessageInput{}, nil, nil, err
	}
	req.ClientRunID = appconversation.EnsureMessageGenerationRunID(req.ClientRunID)
	req.Options = sanitizeMessageOptions(req.Options)
	// 流式接口写入响应头前先拦截明显超限请求，避免后续只能用 NDJSON error 表达 400。
	if err = h.service.ValidateSelectedToolIDs(req.SelectedToolIDs); err != nil {
		handleSendMessageError(c, err)
		return appconversation.SendMessageInput{}, nil, nil, err
	}

	conversation, err := h.service.GetConversationByPublicID(c.Request.Context(), userID, publicID)
	if err != nil {
		if errors.Is(err, appconversation.ErrConversationNotFound) {
			response.Error(c, http.StatusNotFound, "conversation not found")
			return appconversation.SendMessageInput{}, nil, nil, err
		}
		response.Error(c, http.StatusInternalServerError, "load conversation failed")
		return appconversation.SendMessageInput{}, nil, nil, err
	}

	input := appconversation.SendMessageInput{
		UserID:                  userID,
		ConversationID:          conversation.ID,
		RequestID:               middleware.MustRequestID(c),
		KeyBindingID:            req.KeyBindingID,
		ContentType:             req.ContentType,
		Content:                 req.Content,
		PlatformModelName:       req.Model,
		Options:                 req.Options,
		ClientRunID:             req.ClientRunID,
		FileIDs:                 req.FileIDs,
		SelectedToolIDs:         req.SelectedToolIDs,
		SkillIDs:                req.SkillIDs,
		KnowledgeBaseIDs:        req.KnowledgeBaseIDs,
		InputResourceRefs:       req.InputResourceRefs,
		HTMLVisualPromptEnabled: req.HTMLVisualPromptEnabled,
		ParentMessagePublicID:   req.ParentMessagePublicID,
		SourceMessagePublicID:   req.SourceMessagePublicID,
		BranchReason:            req.BranchReason,
	}

	return input, conversation, &req, nil
}

// recordSendMessageAudit 记录审计日志（同步，供非流式路径使用）。
func (h *Handler) recordSendMessageAudit(c *gin.Context, conversation *model.Conversation, req *SendMessageRequest, result *appconversation.SendMessageResult, action string) {
	h.recordSendMessageAuditCtx(
		c.Request.Context(),
		middleware.MustUserID(c),
		middleware.MustRequestID(c),
		c.ClientIP(),
		c.Request.UserAgent(),
		conversation, req, result, action,
	)
}

// recordStreamSendMessageAuditAsync 在 Handler 返回前提取 gin.Context 值，goroutine 内不持有 gin.Context。
func (h *Handler) recordStreamSendMessageAuditAsync(
	c *gin.Context,
	conversation *model.Conversation,
	req *SendMessageRequest,
	result *appconversation.SendMessageResult,
	action string,
) {
	bgUserID := middleware.MustUserID(c)
	bgRequestID := middleware.MustRequestID(c)
	bgClientIP := c.ClientIP()
	bgUserAgent := c.Request.UserAgent()
	go h.recordSendMessageAuditCtx(
		context.Background(),
		bgUserID, bgRequestID, bgClientIP, bgUserAgent,
		conversation, req, result, action,
	)
}

// recordSendMessageAuditCtx 接受显式参数，可在 goroutine 中安全调用（不依赖 gin.Context）。
func (h *Handler) recordSendMessageAuditCtx(
	ctx context.Context,
	userID uint,
	requestID string,
	clientIP string,
	userAgent string,
	conversation *model.Conversation,
	req *SendMessageRequest,
	result *appconversation.SendMessageResult,
	action string,
) {
	if conversation == nil || req == nil || result == nil {
		return
	}
	h.service.RecordAudit(ctx, appconversation.AuditInput{
		UserID:     userID,
		RequestID:  requestID,
		Action:     action,
		Resource:   "conversation",
		ResourceID: conversation.PublicID,
		ClientIP:   clientIP,
		UserAgent:  userAgent,
		Detail: map[string]interface{}{
			"message_id": result.AssistantMessage.PublicID,
			"model":      req.Model,
			"status":     result.AssistantMessage.Status,
		},
	})
}

// handleSendMessageError 处理发送消息错误的公共方法。
func handleSendMessageError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, appconversation.ErrConversationNotFound):
		response.Error(c, http.StatusNotFound, "conversation not found")
	case errors.Is(err, appconversation.ErrInteractionNotFound):
		response.Error(c, http.StatusNotFound, "interaction not found")
	case errors.Is(err, appconversation.ErrInvalidExecutionTarget), errors.Is(err, appconversation.ErrInvalidInteraction):
		response.Error(c, http.StatusBadRequest, "invalid execution request")
	case errors.Is(err, appconversation.ErrExecutionBindingNotFound):
		response.Error(c, http.StatusNotFound, "execution binding not found")
	case errors.Is(err, appconversation.ErrExecutionConflict):
		response.Error(c, http.StatusConflict, "execution state conflict")
	case errors.Is(err, appconversation.ErrExecutionUnavailable):
		response.Error(c, http.StatusServiceUnavailable, "execution adapter unavailable")
	case errors.Is(err, appconversation.ErrInvalidFileReference):
		response.Error(c, http.StatusBadRequest, "invalid file reference")
	case errors.Is(err, appconversation.ErrFileNotFound):
		response.Error(c, http.StatusNotFound, "file not found")
	case errors.Is(err, appconversation.ErrFileTooLarge):
		response.Error(c, http.StatusRequestEntityTooLarge, "file too large")
	case errors.Is(err, appconversation.ErrTooManyMessageFiles):
		response.Error(c, http.StatusBadRequest, "too many files in one message")
	case errors.Is(err, appconversation.ErrTooManySelectedTools):
		response.Error(c, http.StatusBadRequest, "too many selected tools")
	case errors.Is(err, appconversation.ErrMultipleImageAttachmentProcessors):
		response.Error(c, http.StatusBadRequest, "multiple image attachment processors selected")
	case errors.Is(err, appconversation.ErrImageAttachmentProcessingFailed):
		response.Error(c, http.StatusBadGateway, "image attachment processing failed")
	case errors.Is(err, appconversation.ErrTooManySelectedSkills):
		response.Error(c, http.StatusBadRequest, "too many selected skills")
	case errors.Is(err, appconversation.ErrSkillNotFound):
		response.Error(c, http.StatusNotFound, "skill not found")
	case errors.Is(err, appconversation.ErrInvalidSkillUse):
		response.Error(c, http.StatusBadRequest, "invalid skill use")
	case errors.Is(err, appconversation.ErrConversationPluginUnavailable):
		response.Error(c, http.StatusNotFound, "conversation plugin unavailable")
	case errors.Is(err, appconversation.ErrConversationPluginUnsupported):
		response.Error(c, http.StatusBadRequest, "conversation plugin unsupported by model route")
	case errors.Is(err, appconversation.ErrInvalidMessageBranch):
		response.Error(c, http.StatusBadRequest, "invalid message branch")
	case errors.Is(err, appconversation.ErrFileProcessingNotReady):
		response.Error(c, http.StatusBadRequest, "file processing not ready")
	case errors.Is(err, appconversation.ErrFileTooLargeForFullContext):
		response.Error(c, http.StatusBadRequest, "file too large for full context")
	case errors.Is(err, appconversation.ErrEmbeddingUnavailable):
		response.Error(c, http.StatusBadRequest, "embedding unavailable for current file capability")
	case errors.Is(err, appconversation.ErrInvalidKnowledgeBaseReference):
		response.ErrorWithCode(c, http.StatusBadRequest, appconversation.MessageErrorCodeKnowledgeBaseInvalidReference, "invalid knowledge base reference")
	case errors.Is(err, appconversation.ErrKnowledgeBaseUnavailable):
		response.ErrorWithCode(c, http.StatusServiceUnavailable, appconversation.MessageErrorCodeKnowledgeBaseUnavailable, "knowledge base retrieval is unavailable")
	case errors.Is(err, appconversation.ErrKnowledgeBaseNotReady):
		response.ErrorWithCode(c, http.StatusConflict, appconversation.MessageErrorCodeKnowledgeBaseNotReady, "selected knowledge base has no ready files")
	case errors.Is(err, appconversation.ErrModelRouteNotConfigured):
		response.Error(c, http.StatusServiceUnavailable, "model route not configured")
	case errors.Is(err, appconversation.ErrInvalidKeyBinding):
		response.Error(c, http.StatusBadRequest, "invalid key binding")
	case errors.Is(err, appconversation.ErrKeyBindingUnavailable):
		response.Error(c, http.StatusBadGateway, "key binding unavailable")
	case errors.Is(err, appconversation.ErrGeneratedMediaArtifactUnavailable):
		response.ErrorWithCode(c, http.StatusBadGateway, appconversation.MessageErrorCode(err), "generated media artifact is temporarily unavailable")
	case errors.Is(err, appconversation.ErrUpstreamEmptyResponse):
		response.Error(c, http.StatusBadGateway, "model returned empty response")
	case errors.Is(err, appconversation.ErrUpstreamRequestFailed):
		if code := appconversation.MessageErrorCode(err); code != "" {
			response.ErrorWithCode(c, http.StatusBadGateway, code, mapClientErrorMessage(err))
			return
		}
		response.Error(c, http.StatusBadGateway, mapClientErrorMessage(err))
	default:
		response.Error(c, http.StatusInternalServerError, "send message failed")
	}
}

// SendMessage godoc
// @Summary 发送消息
// @Description 在会话中发送消息，支持文件/图片等多模态附件
// @Tags chat
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "会话 public_id"
// @Param body body SendMessageRequest true "消息参数"
// @Success 200 {object} SendMessageResponseDoc
// @Failure 400 {object} ErrorDoc
// @Failure 404 {object} ErrorDoc
// @Failure 500 {object} ErrorDoc
// @Router /conversations/{id}/turns [post]
// StartTurn starts one conversation turn through its bound execution adapter.
func (h *Handler) StartTurn(c *gin.Context) {
	input, conversation, req, err := h.parseSendMessageInput(c)
	if err != nil {
		return
	}
	result, err := h.service.ExecuteTurn(c.Request.Context(), input)
	if err != nil {
		if result != nil {
			h.recordSendMessageAudit(c, conversation, req, result, "send_message")
			response.Success(c, toSendMessageResponse(result))
			return
		}
		handleSendMessageError(c, err)
		return
	}
	h.recordSendMessageAudit(c, conversation, req, result, "send_message")
	response.Success(c, toSendMessageResponse(result))
}

// StreamTurn godoc
// @Summary 流式发送消息
// @Description 在会话中发送消息并以 NDJSON 流式返回 assistant 增量文本
// @Tags chat
// @Accept json
// @Produce application/x-ndjson
// @Security BearerAuth
// @Param id path string true "会话 public_id"
// @Param body body SendMessageRequest true "消息参数"
// @Success 200 {string} string "NDJSON stream"
// @Failure 400 {object} ErrorDoc
// @Failure 404 {object} ErrorDoc
// @Failure 500 {object} ErrorDoc
// @Router /conversations/{id}/turns/stream [post]
func (h *Handler) StreamTurn(c *gin.Context) {
	input, conversation, req, err := h.parseSendMessageInput(c)
	if err != nil {
		return
	}
	c.Header("Content-Type", "application/x-ndjson; charset=utf-8")
	c.Header("Cache-Control", "no-cache, no-transform")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	// Gateway events are already persisted and published by the execution
	// projector before awaitGatewayTurn reads them. Republishing those events
	// from this HTTP subscriber feeds them back into the same stream.
	persistStreamEvents := shouldPersistStreamEvents(conversation.ExecutionType)
	var clientDisconnected atomic.Bool
	flushStreamEvent := func(payload map[string]interface{}) error {
		if persistStreamEvents {
			payload = h.service.PublishMessageGenerationEvent(input.ClientRunID, payload)
		}
		if clientDisconnected.Load() {
			return nil
		}
		encoded, marshalErr := json.Marshal(payload)
		if marshalErr != nil {
			return marshalErr
		}
		if _, writeErr := c.Writer.Write(append(encoded, '\n')); writeErr != nil {
			clientDisconnected.Store(true)
			return writeErr
		}
		c.Writer.Flush()
		return nil
	}

	// 有附件时先推送文件处理事件，提升用户体验感知。
	if len(req.FileIDs) > 0 {
		_ = flushStreamEvent(map[string]interface{}{
			"type":    "file_proc",
			"message": "正在处理附件…",
		})
	}

	// 将中间事件（rag_search 等）通过 NDJSON 推送给客户端。
	input.OnEvent = func(eventType string, payload map[string]interface{}) error {
		_ = flushStreamEvent(normalizeStreamEventPayload(eventType, payload))
		return nil
	}

	result, err := h.service.StreamTurn(c.Request.Context(), input, func(delta string) error {
		_ = flushStreamEvent(map[string]interface{}{
			"type":  "delta",
			"delta": delta,
		})
		return nil
	})
	if err != nil {
		if result != nil {
			payload := streamErrorPayload(err)
			payload["data"] = toSendMessageResponse(result)
			if debug := appconversation.MessageErrorDebug(err); debug != nil {
				payload["debug"] = debug
			}
			_ = flushStreamEvent(payload)
			h.service.FinishMessageGeneration(input.ClientRunID)
			h.recordStreamSendMessageAuditAsync(c, conversation, req, result, "stream_message")
			return
		}
		payload := streamErrorPayload(err)
		if debug := appconversation.MessageErrorDebug(err); debug != nil {
			payload["debug"] = debug
		}
		_ = flushStreamEvent(payload)
		h.service.FinishMessageGeneration(input.ClientRunID)
		return
	}

	_ = flushStreamEvent(map[string]interface{}{
		"type": "completed",
		"data": toSendMessageResponse(result),
	})
	h.service.FinishMessageGeneration(input.ClientRunID)
	h.recordStreamSendMessageAuditAsync(c, conversation, req, result, "stream_message")
}

// InterruptTurn godoc
// @Summary 取消流式生成
// @Description 仅在用户显式点击暂停时取消对应 run；浏览器刷新或断开连接不会调用此接口
// @Tags chat
// @Produce json
// @Security BearerAuth
// @Param run_id path string true "运行 ID"
// @Success 200 {object} response.SuccessDoc
// @Failure 400 {object} ErrorDoc
// @Router /conversation-runs/{run_id}/interrupt [post]
func (h *Handler) InterruptTurn(c *gin.Context) {
	runID, err := stringParam(c, "run_id")
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid run id")
		return
	}
	canceled := h.service.CancelMessageGeneration(c.Request.Context(), middleware.MustUserID(c), runID)
	response.Success(c, CancelMessageGenerationResponse{Canceled: canceled})
}

// SteerTurn godoc
// @Summary Adjust the active Gateway turn
// @Description Sends guidance to the active Gateway turn without interrupting it or creating another run
// @Tags chat
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param run_id path string true "Run ID"
// @Param Idempotency-Key header string true "Idempotency key"
// @Param body body SteerConversationRunRequest true "Turn guidance"
// @Success 200 {object} response.SuccessDoc
// @Failure 400,404,409 {object} ErrorDoc
// @Router /conversation-runs/{run_id}/steer [post]
func (h *Handler) SteerTurn(c *gin.Context) {
	runID, err := stringParam(c, "run_id")
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid run id")
		return
	}
	var request SteerConversationRunRequest
	if err = bindConversationJSON(c, &request, 1024*1024+1024); err != nil {
		response.InvalidRequestBody(c, err)
		return
	}
	err = h.service.SteerConversationRun(
		c.Request.Context(), middleware.MustUserID(c), runID, request.Content, c.GetHeader("Idempotency-Key"),
	)
	if err != nil {
		handleSendMessageError(c, err)
		return
	}
	response.Success(c, map[string]bool{"accepted": true})
}

// StreamActiveMessageGenerations godoc
// @Summary Stream active conversation generations
// @Description Sends an authoritative snapshot followed by live user-scoped run state events; the snapshot is re-sent periodically for client-side reconciliation
// @Tags chat
// @Produce text/event-stream
// @Security BearerAuth
// @Success 200 {object} ActiveMessageGenerationEventResponse
// @Failure 500 {object} ErrorDoc
// @Router /conversation-runs/stream [get]
func (h *Handler) StreamActiveMessageGenerations(c *gin.Context) {
	userID := middleware.MustUserID(c)
	snapshot, events, unsubscribe, err := h.service.SubscribeActiveMessageGenerations(c.Request.Context(), userID)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "failed to subscribe to active conversation generations")
		return
	}
	defer unsubscribe()

	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("Cache-Control", "no-cache, no-transform")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	writeEvent := func(payload ActiveMessageGenerationEventResponse) bool {
		encoded, marshalErr := json.Marshal(payload)
		if marshalErr != nil {
			return true
		}
		if _, writeErr := c.Writer.Write([]byte("data: ")); writeErr != nil {
			return false
		}
		if _, writeErr := c.Writer.Write(encoded); writeErr != nil {
			return false
		}
		if _, writeErr := c.Writer.Write([]byte("\n\n")); writeErr != nil {
			return false
		}
		c.Writer.Flush()
		return true
	}
	writeSnapshot := func(items []appconversation.ActiveMessageGeneration) bool {
		runs := make([]ActiveMessageGenerationResponse, 0, len(items))
		for _, item := range items {
			runs = append(runs, ActiveMessageGenerationResponse{
				RunID:                item.RunID,
				ConversationPublicID: item.ConversationPublicID,
			})
		}
		return writeEvent(ActiveMessageGenerationEventResponse{Type: "snapshot", Runs: runs})
	}

	if !writeSnapshot(snapshot) {
		return
	}

	snapshotTicker := time.NewTicker(20 * time.Second)
	defer snapshotTicker.Stop()
	for {
		select {
		case <-c.Request.Context().Done():
			return
		case <-h.shutdown.Done():
			return
		case <-snapshotTicker.C:
			latest, listErr := h.service.ListActiveMessageGenerations(c.Request.Context(), userID)
			if listErr != nil {
				if _, writeErr := c.Writer.Write([]byte(": keepalive\n\n")); writeErr != nil {
					return
				}
				c.Writer.Flush()
				continue
			}
			if !writeSnapshot(latest) {
				return
			}
		case event, ok := <-events:
			if !ok {
				return
			}
			if !writeEvent(ActiveMessageGenerationEventResponse{
				Type:                 event.Type,
				RunID:                event.RunID,
				ConversationPublicID: event.ConversationPublicID,
			}) {
				return
			}
		}
	}
}

// ResumeMessageGenerationStream godoc
// @Summary 恢复流式生成订阅
// @Description 页面刷新后按 run_id 重新订阅仍在运行的生成流，返回 NDJSON 事件
// @Tags chat
// @Produce application/x-ndjson
// @Security BearerAuth
// @Param run_id path string true "运行 ID"
// @Param after query int false "已接收的最后事件序号"
// @Success 200 {string} string "NDJSON stream"
// @Failure 404 {object} ErrorDoc
// @Router /conversation-runs/{run_id}/stream [get]
func (h *Handler) ResumeMessageGenerationStream(c *gin.Context) {
	runID, err := stringParam(c, "run_id")
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid run id")
		return
	}
	afterSeq, _ := strconv.ParseInt(strings.TrimSpace(c.Query("after")), 10, 64)
	if afterSeq < 0 {
		afterSeq = 0
	}
	userID := middleware.MustUserID(c)
	replay, events, unsubscribe, ok := h.service.SubscribeMessageGeneration(
		c.Request.Context(),
		userID,
		runID,
		afterSeq,
		true,
	)
	if !ok {
		h.service.ReconcileInterruptedMessageGeneration(c.Request.Context(), userID, runID)
		response.Error(c, http.StatusNotFound, "generation stream not found")
		return
	}
	defer unsubscribe()

	c.Header("Content-Type", "application/x-ndjson; charset=utf-8")
	c.Header("Cache-Control", "no-cache, no-transform")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	isTerminal := func(payload map[string]interface{}) bool {
		eventType, _ := payload["type"].(string)
		return eventType == "completed" || eventType == "error"
	}
	terminalWritten := false
	writeEvent := func(payload map[string]interface{}) bool {
		if eventType, _ := payload["type"].(string); eventType == "gateway_completed" {
			result, resultErr := h.service.GetTurnResult(c.Request.Context(), userID, runID)
			if resultErr != nil {
				payload = streamErrorPayload(resultErr)
				if result != nil {
					payload["data"] = toSendMessageResponse(result)
				}
			} else {
				payload = map[string]interface{}{"type": "completed", "data": toSendMessageResponse(result)}
			}
		}
		encoded, marshalErr := json.Marshal(payload)
		if marshalErr != nil {
			return true
		}
		if _, writeErr := c.Writer.Write(append(encoded, '\n')); writeErr != nil {
			return false
		}
		c.Writer.Flush()
		if isTerminal(payload) {
			terminalWritten = true
		}
		return true
	}

	for _, event := range replay {
		if !writeEvent(event.Payload) {
			return
		}
	}
	if terminalWritten {
		return
	}

	isActive := func() bool {
		return h.service.HasActiveMessageGeneration(c.Request.Context(), runID)
	}
	writeInterrupted := func() {
		if h.service.ReconcileInterruptedMessageGeneration(c.Request.Context(), userID, runID) {
			_ = writeEvent(map[string]interface{}{"type": "gateway_completed"})
			return
		}
		_ = writeEvent(streamErrorPayloadWithCode("conversation_run.stream_interrupted", "generation stream was interrupted; retry this message"))
	}
	if !isActive() {
		writeInterrupted()
		return
	}
	activeTicker := time.NewTicker(resumeActiveCheckInterval)
	defer func() {
		activeTicker.Stop()
	}()

	for {
		select {
		case <-c.Request.Context().Done():
			return
		case <-h.shutdown.Done():
			return
		case <-activeTicker.C:
			if !isActive() {
				writeInterrupted()
				return
			}
		case event, ok := <-events:
			if !ok {
				if !terminalWritten && !isActive() {
					writeInterrupted()
				}
				return
			}
			if !writeEvent(event.Payload) {
				return
			}
			if terminalWritten {
				return
			}
		}
	}
}
