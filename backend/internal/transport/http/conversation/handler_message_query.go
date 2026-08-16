package conversation

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	appconversation "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/conversation"
	model "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/conversation"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/response"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/transport/http/middleware"
	"github.com/gin-gonic/gin"
)

// ListInputResources godoc
// @Summary List local input resources for a Work workspace
// @Tags chat
// @Produce json
// @Security BearerAuth
// @Param device query string true "Gateway device public ID"
// @Param workspace query string true "Gateway workspace public ID"
// @Param query query string false "Name or description filter"
// @Success 200 {object} ConversationInputResourceListResponseDoc
// @Failure 400,404 {object} ErrorDoc
// @Router /conversation-input-resources [get]
func (h *Handler) ListInputResources(c *gin.Context) {
	catalog, err := h.service.ListInputResources(
		c.Request.Context(), middleware.MustUserID(c), c.Query("device"), c.Query("workspace"), c.Query("query"),
	)
	if err != nil {
		handleSendMessageError(c, err)
		return
	}
	result := make([]ConversationInputResourceResponse, 0, len(catalog.Items))
	for _, item := range catalog.Items {
		result = append(result, ConversationInputResourceResponse{
			ResourceRef: item.ResourceRef, Kind: item.Kind, Name: item.Name, Description: item.Description,
		})
	}
	response.Success(c, ConversationInputResourceCatalogResponse{Ready: catalog.Ready, Items: result})
}

// ListExecutionEvents godoc
// @Summary List conversation execution events
// @Tags chat
// @Produce json
// @Security BearerAuth
// @Param id path string true "Conversation public ID"
// @Param after query int false "Last applied sequence"
// @Success 200 {object} ExecutionEventListResponseDoc
// @Failure 404 {object} ErrorDoc
// @Router /conversations/{id}/events [get]
func (h *Handler) ListExecutionEvents(c *gin.Context) {
	var after uint64
	rawAfter := strings.TrimSpace(c.Query("after"))
	if rawAfter != "" {
		parsed, err := strconv.ParseUint(rawAfter, 10, 64)
		if err != nil {
			response.Error(c, http.StatusBadRequest, "invalid event cursor")
			return
		}
		after = parsed
	}
	items, err := h.service.ListExecutionEvents(c.Request.Context(), middleware.MustUserID(c), c.Param("id"), after)
	if err != nil {
		if errors.Is(err, appconversation.ErrConversationNotFound) {
			response.Error(c, http.StatusNotFound, "conversation not found")
			return
		}
		response.Error(c, http.StatusInternalServerError, "load execution events failed")
		return
	}
	result := make([]ExecutionEventResponse, 0, len(items))
	for _, item := range items {
		var payload interface{}
		if json.Unmarshal([]byte(item.PayloadJSON), &payload) != nil {
			payload = map[string]interface{}{}
		}
		result = append(result, ExecutionEventResponse{
			RunID: item.RunID, Seq: item.Seq, Kind: item.Kind, Payload: payload, OccurredAt: item.OccurredAt,
		})
	}
	response.Success(c, result)
}

// ListInteractions godoc
// @Summary List pending and completed conversation interactions
// @Tags chat
// @Produce json
// @Security BearerAuth
// @Param id path string true "Conversation public ID"
// @Param status query string false "Interaction status"
// @Success 200 {object} InteractionListResponseDoc
// @Failure 400,404 {object} ErrorDoc
// @Router /conversations/{id}/interactions [get]
func (h *Handler) ListInteractions(c *gin.Context) {
	items, err := h.service.ListInteractions(c.Request.Context(), middleware.MustUserID(c), c.Param("id"), c.Query("status"))
	if err != nil {
		handleSendMessageError(c, err)
		return
	}
	result := make([]InteractionResponse, 0, len(items))
	for _, item := range items {
		result = append(result, toInteractionResponse(item))
	}
	response.Success(c, result)
}

// RespondInteraction godoc
// @Summary Respond to a conversation execution interaction
// @Tags chat
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param interaction_id path string true "Interaction public ID"
// @Param Idempotency-Key header string true "Idempotency key"
// @Param body body RespondInteractionRequest true "Interaction response"
// @Success 200 {object} InteractionResponseDoc
// @Failure 400,404,409 {object} ErrorDoc
// @Router /conversation-interactions/{interaction_id}/respond [post]
func (h *Handler) RespondInteraction(c *gin.Context) {
	var request RespondInteractionRequest
	if err := bindConversationJSON(c, &request, 1024*1024); err != nil {
		response.InvalidRequestBody(c, err)
		return
	}
	item, err := h.service.RespondInteraction(c.Request.Context(), middleware.MustUserID(c), appconversation.GatewayInteractionResponse{
		InteractionID:  strings.TrimSpace(c.Param("interaction_id")),
		IdempotencyKey: strings.TrimSpace(c.GetHeader("Idempotency-Key")), Response: request.Response,
	})
	if err != nil {
		handleSendMessageError(c, err)
		return
	}
	response.Success(c, toInteractionResponse(*item))
}

func toInteractionResponse(item appconversation.GatewayInteraction) InteractionResponse {
	var request interface{}
	if json.Unmarshal(item.Request, &request) != nil {
		request = map[string]interface{}{}
	}
	return InteractionResponse{
		InteractionID: item.InteractionID, RunID: item.RunID, Kind: item.Kind,
		Status: item.Status, Request: request, CreatedAt: item.CreatedAt,
	}
}

// UpdateMessage godoc
// @Summary 更新消息内容
// @Description 更新当前用户会话中的 assistant 消息内容，并标记为已编辑
// @Tags chat
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "消息 public_id"
// @Param body body UpdateMessageRequest true "消息内容"
// @Success 200 {object} MessageResponseDoc
// @Failure 400 {object} ErrorDoc
// @Failure 404 {object} ErrorDoc
// @Failure 500 {object} ErrorDoc
// @Router /messages/{id} [patch]
func (h *Handler) UpdateMessage(c *gin.Context) {
	userID := middleware.MustUserID(c)
	publicID, err := stringParam(c, "id")
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid message id")
		return
	}

	var req UpdateMessageRequest
	if err = c.ShouldBindJSON(&req); err != nil {
		response.InvalidRequestBody(c, err)
		return
	}

	item, err := h.service.UpdateAssistantMessageContent(c.Request.Context(), userID, publicID, req.Content)
	if err != nil {
		switch {
		case errors.Is(err, appconversation.ErrInvalidMessageContent):
			response.Error(c, http.StatusBadRequest, "invalid message content")
			return
		case errors.Is(err, appconversation.ErrMessageEditTargetInvalid):
			response.Error(c, http.StatusBadRequest, "message edit target invalid")
			return
		case errors.Is(err, appconversation.ErrMessageEditStateInvalid):
			response.Error(c, http.StatusBadRequest, "message edit state invalid")
			return
		case errors.Is(err, appconversation.ErrMessageNotFound):
			response.Error(c, http.StatusNotFound, "message not found")
			return
		default:
			response.Error(c, http.StatusInternalServerError, "update message failed")
			return
		}
	}

	h.recordAudit(c, "update_message",
		"message",
		item.PublicID,
		map[string]interface{}{
			"role": item.Role,
		},
	)

	run := model.Run{}
	runID := strings.TrimSpace(item.RunID)
	if runID != "" {
		runs, runErr := h.service.ListConversationRunsByRunIDs(c.Request.Context(), userID, item.ConversationID, []string{runID})
		if runErr == nil && len(runs) > 0 {
			run = runs[0]
		}
	}
	response.Success(c, toMessageResponseWithRun(*item, run))
}

// SetMessageFeedback godoc
// @Summary 设置消息反馈
// @Description 对 assistant 消息设置点赞/点踩，传空 feedback 表示取消反馈
// @Tags chat
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "消息 public_id"
// @Param body body SetMessageFeedbackRequest true "反馈参数"
// @Success 200 {object} MessageFeedbackResponseDoc
// @Failure 400 {object} ErrorDoc
// @Failure 404 {object} ErrorDoc
// @Failure 500 {object} ErrorDoc
// @Router /messages/{id}/feedback [put]
func (h *Handler) SetMessageFeedback(c *gin.Context) {
	userID := middleware.MustUserID(c)
	publicID, err := stringParam(c, "id")
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid message id")
		return
	}

	var req SetMessageFeedbackRequest
	if err = c.ShouldBindJSON(&req); err != nil {
		response.InvalidRequestBody(c, err)
		return
	}

	result, err := h.service.SetMessageFeedback(c.Request.Context(), userID, publicID, req.Feedback)
	if err != nil {
		switch {
		case errors.Is(err, appconversation.ErrInvalidMessageFeedback):
			response.Error(c, http.StatusBadRequest, "invalid message feedback")
			return
		case errors.Is(err, appconversation.ErrMessageFeedbackTargetInvalid):
			response.Error(c, http.StatusBadRequest, "message feedback target invalid")
			return
		case errors.Is(err, appconversation.ErrMessageNotFound):
			response.Error(c, http.StatusNotFound, "message not found")
			return
		default:
			response.Error(c, http.StatusInternalServerError, "set message feedback failed")
			return
		}
	}

	h.recordAudit(c, "set_message_feedback",
		"message",
		result.MessagePublicID,
		map[string]interface{}{
			"feedback": req.Feedback,
		},
	)

	response.Success(c, toMessageFeedbackResponse(result))
}

// ListMessages godoc
// @Summary 查询会话消息
// @Description 查询会话内消息列表
// @Tags chat
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "会话 public_id"
// @Param page query int false "页码"
// @Param page_size query int false "每页数量"
// @Success 200 {object} MessageListResponseDoc
// @Failure 400 {object} ErrorDoc
// @Failure 404 {object} ErrorDoc
// @Failure 500 {object} ErrorDoc
// @Router /conversations/{id}/messages [get]
// ListMessages 查询消息。
func (h *Handler) ListMessages(c *gin.Context) {
	userID := middleware.MustUserID(c)
	publicID, err := stringParam(c, "id")
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid conversation id")
		return
	}

	page, pageSize := messagePageParams(c)
	var beforeID uint
	if rawBeforeID := strings.TrimSpace(c.Query("before_id")); rawBeforeID != "" {
		parsed, parseErr := strconv.ParseUint(rawBeforeID, 10, strconv.IntSize)
		if parseErr != nil || parsed == 0 {
			response.Error(c, http.StatusBadRequest, "invalid before message id")
			return
		}
		beforeID = uint(parsed)
	}
	conversation, err := h.service.GetConversationByPublicID(c.Request.Context(), userID, publicID)
	if err != nil {
		if errors.Is(err, appconversation.ErrConversationNotFound) {
			response.Error(c, http.StatusNotFound, "conversation not found")
			return
		}
		response.Error(c, http.StatusInternalServerError, "load conversation failed")
		return
	}

	var items []model.Message
	var total int64
	if beforeID > 0 {
		items, total, err = h.service.ListMessagesBeforeID(c.Request.Context(), userID, conversation.ID, beforeID, pageSize)
	} else if c.Query("tail") == "true" {
		items, total, err = h.service.ListRecentMessages(c.Request.Context(), userID, conversation.ID, pageSize)
	} else {
		items, total, err = h.service.ListMessages(c.Request.Context(), userID, conversation.ID, page, pageSize)
	}
	if err != nil {
		if errors.Is(err, appconversation.ErrConversationNotFound) {
			response.Error(c, http.StatusNotFound, "conversation not found")
			return
		}
		response.Error(c, http.StatusInternalServerError, "list messages failed")
		return
	}
	runModels := map[string]model.Run{}
	runIDs := collectMessageRunIDs(items)
	if len(runIDs) > 0 {
		runs, runErr := h.service.ListConversationRunsByRunIDs(c.Request.Context(), userID, conversation.ID, runIDs)
		if runErr != nil {
			if errors.Is(runErr, appconversation.ErrConversationNotFound) {
				response.Error(c, http.StatusNotFound, "conversation not found")
				return
			}
			response.Error(c, http.StatusInternalServerError, "list conversation runs failed")
			return
		}
		for _, run := range runs {
			if runID := strings.TrimSpace(run.RunID); runID != "" {
				runModels[runID] = run
			}
		}
	}
	msgResults := make([]MessageResponse, 0, len(items))
	for _, m := range items {
		msgResults = append(msgResults, toMessageResponseWithRunAndFallback(m, runModels[strings.TrimSpace(m.RunID)], conversation.Model))
	}
	response.SuccessPage(c, total, msgResults)
}

// GetConversationHistory godoc
// @Summary 查询会话历史加载状态
// @Tags chat
// @Produce json
// @Security BearerAuth
// @Param id path string true "会话 public_id"
// @Success 200 {object} ConversationHistoryResponseDoc
// @Failure 400 {object} ErrorDoc
// @Failure 404 {object} ErrorDoc
// @Router /conversations/{id}/history [get]
func (h *Handler) GetConversationHistory(c *gin.Context) {
	h.conversationHistory(c, false)
}

// EnsureConversationHistory godoc
// @Summary 准备会话完整历史
// @Description 普通聊天立即就绪；本地工作会话按需排队读取对应 Codex thread。
// @Tags chat
// @Produce json
// @Security BearerAuth
// @Param id path string true "会话 public_id"
// @Success 200 {object} ConversationHistoryResponseDoc
// @Failure 400 {object} ErrorDoc
// @Failure 404 {object} ErrorDoc
// @Failure 409 {object} ErrorDoc
// @Router /conversations/{id}/history [post]
func (h *Handler) EnsureConversationHistory(c *gin.Context) {
	h.conversationHistory(c, true)
}

func (h *Handler) conversationHistory(c *gin.Context, ensure bool) {
	publicID, err := stringParam(c, "id")
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid conversation id")
		return
	}
	history, err := h.service.ConversationHistory(c.Request.Context(), middleware.MustUserID(c), publicID, ensure)
	if err != nil {
		switch {
		case errors.Is(err, appconversation.ErrConversationNotFound):
			response.Error(c, http.StatusNotFound, "conversation not found")
		case errors.Is(err, appconversation.ErrExecutionBindingNotFound), errors.Is(err, appconversation.ErrExecutionConflict), errors.Is(err, appconversation.ErrExecutionUnavailable):
			response.Error(c, http.StatusConflict, err.Error())
		default:
			response.Error(c, http.StatusInternalServerError, "prepare conversation history failed")
		}
		return
	}
	response.Success(c, ConversationHistoryResponse{Status: history.Status, Error: history.Error})
}

// ListConversationPreviewMessages godoc
// @Summary 查询会话预览消息
// @Description 返回当前用户会话最新分支最近 10 条用户或助手消息
// @Tags chat
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "会话 public_id"
// @Success 200 {object} ConversationPreviewMessageListResponseDoc
// @Failure 400 {object} ErrorDoc
// @Failure 404 {object} ErrorDoc
// @Failure 500 {object} ErrorDoc
// @Router /conversations/{id}/messages/preview [get]
// ListConversationPreviewMessages 查询搜索预览所需的轻量消息。
func (h *Handler) ListConversationPreviewMessages(c *gin.Context) {
	userID := middleware.MustUserID(c)
	publicID, err := stringParam(c, "id")
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid conversation id")
		return
	}
	items, err := h.service.ListConversationPreviewMessages(c.Request.Context(), userID, publicID)
	if err != nil {
		if errors.Is(err, appconversation.ErrConversationNotFound) {
			response.Error(c, http.StatusNotFound, "conversation not found")
			return
		}
		response.Error(c, http.StatusInternalServerError, "list conversation preview messages failed")
		return
	}
	results := make([]ConversationPreviewMessageResponse, 0, len(items))
	for _, item := range items {
		results = append(results, toConversationPreviewMessageResponse(item))
	}
	response.Success(c, results)
}

// collectMessageRunIDs 提取消息列表中的运行 ID，并保持首次出现顺序。
func collectMessageRunIDs(items []model.Message) []string {
	seen := make(map[string]struct{}, len(items))
	runIDs := make([]string, 0, len(items))
	for _, item := range items {
		runID := strings.TrimSpace(item.RunID)
		if runID == "" {
			continue
		}
		if _, ok := seen[runID]; ok {
			continue
		}
		seen[runID] = struct{}{}
		runIDs = append(runIDs, runID)
	}
	return runIDs
}

// ListConversationRuns godoc
// @Summary 查询会话运行日志
// @Description 查询会话内模型调用运行日志（tokens/时长/错误）
// @Tags chat
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "会话 public_id"
// @Param page query int false "页码"
// @Param page_size query int false "每页数量"
// @Success 200 {object} ConversationRunListResponseDoc
// @Failure 400 {object} ErrorDoc
// @Failure 404 {object} ErrorDoc
// @Failure 500 {object} ErrorDoc
// @Router /conversations/{id}/runs [get]
// ListConversationRuns 查询运行日志。
func (h *Handler) ListConversationRuns(c *gin.Context) {
	userID := middleware.MustUserID(c)
	publicID, err := stringParam(c, "id")
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid conversation id")
		return
	}

	page, pageSize := pageParams(c)
	conversation, err := h.service.GetConversationByPublicID(c.Request.Context(), userID, publicID)
	if err != nil {
		if errors.Is(err, appconversation.ErrConversationNotFound) {
			response.Error(c, http.StatusNotFound, "conversation not found")
			return
		}
		response.Error(c, http.StatusInternalServerError, "load conversation failed")
		return
	}

	items, total, err := h.service.ListConversationRuns(c.Request.Context(), userID, conversation.ID, page, pageSize)
	if err != nil {
		if errors.Is(err, appconversation.ErrConversationNotFound) {
			response.Error(c, http.StatusNotFound, "conversation not found")
			return
		}
		response.Error(c, http.StatusInternalServerError, "list conversation runs failed")
		return
	}
	runResults := make([]RunResponse, 0, len(items))
	for _, r := range items {
		runResults = append(runResults, toRunResponse(r))
	}
	response.SuccessPage(c, total, runResults)
}
