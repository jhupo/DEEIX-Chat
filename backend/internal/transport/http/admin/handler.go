package admin

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	appadmin "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/admin"
	auditapp "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/audit"
	appconversation "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/conversation"
	applogcleanup "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/logcleanup"
	systemeventapp "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/systemevent"
	appuser "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/user"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/userview"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/response"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/transport/http/middleware"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/update"
	"github.com/gin-gonic/gin"
)

type conversationExporter interface{}
type Handler struct {
	service              *appadmin.Service
	updater              *update.Updater
	conversationExporter conversationExporter
}

func NewHandler(service *appadmin.Service) *Handler   { return &Handler{service: service} }
func (h *Handler) SetUpdater(updater *update.Updater) { h.updater = updater }
func (h *Handler) SetConversationExporter(exporter conversationExporter) {
	h.conversationExporter = exporter
}
func adminID(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 0)
	if err != nil || id == 0 {
		response.Error(c, http.StatusBadRequest, "invalid user id")
		return 0, false
	}
	return uint(id), true
}

// @Summary List principals
// @Tags admin
// @Security BearerAuth
// @Success 200 {object} UserListResponseDoc
// @Router /admin/users [get]
func (h *Handler) ListUsers(c *gin.Context) {
	items, total, err := h.service.ListUsers(c.Request.Context(), 1, 50, appadmin.UserListFilter{Query: c.Query("query")})
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "list users failed")
		return
	}
	results := make([]UserResponse, 0, len(items))
	for _, item := range items {
		results = append(results, toUserResponse(item))
	}
	response.SuccessPage(c, total, results)
}

// @Summary Patch a principal profile
// @Tags admin
// @Security BearerAuth
// @Param id path int true "User ID"
// @Param body body PatchUserRequest true "Profile fields"
// @Success 200 {object} UserDataResponseDoc
// @Router /admin/users/{id} [patch]
func (h *Handler) PatchUser(c *gin.Context) {
	id, ok := adminID(c)
	if !ok {
		return
	}
	var req PatchUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.InvalidRequestBody(c, err)
		return
	}
	item, err := h.service.PatchUserByAdmin(c.Request.Context(), middleware.MustRequestID(c), middleware.MustUserID(c), id, toAppPatchUserInput(req), c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		if errors.Is(err, appuser.ErrUserNotFound) {
			response.Error(c, http.StatusNotFound, "user not found")
		} else {
			response.ErrorFrom(c, http.StatusBadRequest, err)
		}
		return
	}
	response.Success(c, UserDataResponse{User: toUserResponse(userview.FromUser(*item))})
}

// @Summary List audit logs
// @Tags admin
// @Security BearerAuth
// @Success 200 {object} AuditLogListResponseDoc
// @Router /admin/audit-logs [get]
func (h *Handler) ListAuditLogs(c *gin.Context) {
	page, size := pageParams(c)
	actorID, ok := optionalUint(c, "actor_user_id")
	if !ok {
		return
	}
	from, ok := optionalTime(c, "created_from")
	if !ok {
		return
	}
	to, ok := optionalTime(c, "created_to")
	if !ok {
		return
	}
	items, total, err := h.service.ListAuditLogs(c, page, size, auditapp.ListFilter{Query: c.Query("query"), Resource: c.Query("resource"), Action: c.Query("action"), ActorUserID: actorID, CreatedFrom: from, CreatedTo: to, Sort: c.Query("sort")})
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "list audit logs failed")
		return
	}
	labels := h.service.ResolveUserLabels(c, auditActorIDs(items))
	results := make([]AuditLogResponse, 0, len(items))
	for _, item := range items {
		results = append(results, toAuditLogResponse(item, labels[item.ActorUserID]))
	}
	response.SuccessPage(c, total, results)
}

// @Summary Remove nonfinancial logs before a timestamp
// @Tags admin
// @Security BearerAuth
// @Param body body CleanupLogsRequest true "Cleanup request"
// @Success 200 {object} CleanupLogsResponseDoc
// @Router /admin/logs/cleanup [post]
func (h *Handler) CleanupLogs(c *gin.Context) {
	var req CleanupLogsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.InvalidRequestBody(c, err)
		return
	}
	before, err := time.Parse(time.RFC3339, req.Before)
	if err != nil || !nonfinancialLogType(req.Type) {
		response.Error(c, http.StatusBadRequest, "invalid log cleanup request")
		return
	}
	result, err := h.service.CleanupLogs(c, applogcleanup.Input{Type: req.Type, Before: before, RequestID: middleware.MustRequestID(c), ActorUserID: middleware.MustUserID(c), IP: c.ClientIP(), UserAgent: c.Request.UserAgent()})
	if err != nil {
		response.Error(c, http.StatusBadRequest, "cleanup logs failed")
		return
	}
	response.Success(c, CleanupLogsResponse{Type: result.Type, Before: result.Before, DeletedCount: result.DeletedCount})
}

// @Summary Remove conversation event runs
// @Tags admin
// @Security BearerAuth
// @Param body body CleanupConversationRunsRequest true "Cleanup request"
// @Success 200 {object} CleanupConversationRunsResponseDoc
// @Router /admin/conversation-events/cleanup [post]
func (h *Handler) CleanupConversationRuns(c *gin.Context) {
	var req CleanupConversationRunsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.InvalidRequestBody(c, err)
		return
	}
	result, err := h.service.CleanupConversationRuns(c, applogcleanup.ConversationRunInput{RunIDs: req.RunIDs, RequestID: middleware.MustRequestID(c), ActorUserID: middleware.MustUserID(c), IP: c.ClientIP(), UserAgent: c.Request.UserAgent()})
	if err != nil {
		response.Error(c, http.StatusBadRequest, "cleanup conversation runs failed")
		return
	}
	response.Success(c, CleanupConversationRunsResponse{RunCount: result.RunCount, DeletedCount: result.DeletedCount})
}

// @Summary List conversation events
// @Tags admin
// @Security BearerAuth
// @Success 200 {object} ConversationEventListResponseDoc
// @Router /admin/conversation-events [get]
func (h *Handler) ListConversationEvents(c *gin.Context) {
	page, size := pageParams(c)
	userID, ok := optionalUint(c, "user_id")
	if !ok {
		return
	}
	conversationID, ok := optionalUint(c, "conversation_id")
	if !ok {
		return
	}
	from, ok := optionalTime(c, "created_from")
	if !ok {
		return
	}
	to, ok := optionalTime(c, "created_to")
	if !ok {
		return
	}
	items, total, err := h.service.ListConversationEventLogs(c, page, size, appconversation.EventLogListFilter{Query: c.Query("query"), EventScope: c.Query("event_scope"), EventType: c.Query("event_type"), Status: c.Query("status"), UserID: userID, ConversationID: conversationID, CreatedFrom: from, CreatedTo: to, Sort: c.Query("sort")})
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "list conversation events failed")
		return
	}
	labels := h.service.ResolveUserLabels(c, conversationUserIDs(items))
	results := make([]ConversationEventResponse, 0, len(items))
	for _, item := range items {
		results = append(results, toConversationEventResponse(item, labels[item.UserID]))
	}
	response.SuccessPage(c, total, results)
}

// @Summary Get a conversation event
// @Tags admin
// @Security BearerAuth
// @Param id path int true "Event ID"
// @Success 200 {object} ConversationEventDetailResponseDoc
// @Router /admin/conversation-events/{id} [get]
func (h *Handler) GetConversationEvent(c *gin.Context) {
	id, ok := adminID(c)
	if !ok {
		return
	}
	item, err := h.service.GetConversationEventLog(c, id)
	if err != nil {
		response.Error(c, http.StatusNotFound, "conversation event not found")
		return
	}
	label := h.service.ResolveUserLabels(c, []uint{item.UserID})[item.UserID]
	response.Success(c, toConversationEventResponse(*item, label))
}

// @Summary List system events
// @Tags admin
// @Security BearerAuth
// @Success 200 {object} SystemEventListResponseDoc
// @Router /admin/system-events [get]
func (h *Handler) ListSystemEvents(c *gin.Context) {
	page, size := pageParams(c)
	from, ok := optionalTime(c, "created_from")
	if !ok {
		return
	}
	to, ok := optionalTime(c, "created_to")
	if !ok {
		return
	}
	items, total, err := h.service.ListSystemEvents(c, page, size, systemeventapp.ListFilter{Query: c.Query("query"), Level: c.Query("level"), Source: c.Query("source"), Event: c.Query("event"), CreatedFrom: from, CreatedTo: to, Sort: c.Query("sort")})
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "list system events failed")
		return
	}
	results := make([]SystemEventResponse, 0, len(items))
	for _, item := range items {
		results = append(results, toSystemEventResponse(item))
	}
	response.SuccessPage(c, total, results)
}

// @Summary Revoke local sessions
// @Tags admin
// @Security BearerAuth
// @Param id path int true "User ID"
// @Success 200 {object} RevokeUserSessionsResponseDoc
// @Router /admin/users/{id}/revoke-sessions [post]
func (h *Handler) RevokeUserSessions(c *gin.Context) {
	id, ok := adminID(c)
	if !ok {
		return
	}
	result, err := h.service.RevokeUserSessionsByAdmin(c.Request.Context(), middleware.MustRequestID(c), middleware.MustUserID(c), id, c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		response.ErrorFrom(c, http.StatusBadRequest, err)
		return
	}
	response.Success(c, RevokeUserSessionsResponse{Revoked: result.Revoked})
}

// @Summary List authentication events
// @Tags admin
// @Security BearerAuth
// @Success 200 {object} AuthEventListResponseDoc
// @Router /admin/user-auth-events [get]
func (h *Handler) ListUserAuthEvents(c *gin.Context) {
	page, size := pageParams(c)
	userID, ok := optionalUint(c, "user_id")
	if !ok {
		return
	}
	items, total, err := h.service.ListUserAuthEventsByAdmin(c, userID, c.Query("event_type"), c.Query("result"), page, size)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "list auth events failed")
		return
	}
	labels := h.service.ResolveUserLabels(c, authUserIDs(items))
	results := make([]AuthEventResponse, 0, len(items))
	for _, item := range items {
		results = append(results, toAuthEventResponse(item, labels[item.UserID]))
	}
	response.SuccessPage(c, total, results)
}

func pageParams(c *gin.Context) (int, int) {
	page, size := 1, 20
	if v, err := strconv.Atoi(c.Query("page")); err == nil && v > 0 {
		page = v
	}
	if v, err := strconv.Atoi(c.Query("page_size")); err == nil && v > 0 {
		if v > 1000 {
			v = 1000
		}
		size = v
	}
	return page, size
}
func optionalUint(c *gin.Context, key string) (uint, bool) {
	raw := c.Query(key)
	if raw == "" {
		return 0, true
	}
	v, err := strconv.ParseUint(raw, 10, 0)
	if err != nil || v == 0 {
		response.Error(c, http.StatusBadRequest, "invalid "+key)
		return 0, false
	}
	return uint(v), true
}
func optionalTime(c *gin.Context, key string) (*time.Time, bool) {
	raw := c.Query(key)
	if raw == "" {
		return nil, true
	}
	v, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid "+key)
		return nil, false
	}
	return &v, true
}
func nonfinancialLogType(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case applogcleanup.TypeAudit, applogcleanup.TypeAuth, applogcleanup.TypeConversation, applogcleanup.TypeSystem:
		return true
	}
	return false
}
