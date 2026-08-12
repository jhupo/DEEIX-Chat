package agentgateway

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	appagent "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/agentgateway"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/response"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/transport/http/middleware"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *appagent.Service
	hub     *bridgeHub
}

func NewHandler(service *appagent.Service) *Handler {
	return &Handler{service: service, hub: newBridgeHub()}
}

type enrollmentResponse struct {
	EnrollmentCode string `json:"enrollmentCode"`
	ExpiresAt      string `json:"expiresAt"`
}

type enrollDeviceRequest struct {
	EnrollmentCode string `json:"enrollmentCode"`
	Name           string `json:"name"`
	Platform       string `json:"platform"`
	PublicKey      string `json:"publicKey"`
}

type enrollDeviceResponse struct {
	DeviceID string `json:"deviceId"`
	Status   string `json:"status"`
}

type renameDeviceRequest struct {
	Name string `json:"name"`
}

type deviceResponse struct {
	DeviceID   string  `json:"deviceId"`
	UserID     string  `json:"userId"`
	Name       string  `json:"name"`
	Platform   string  `json:"platform"`
	Status     string  `json:"status"`
	LastSeenAt *string `json:"lastSeenAt"`
	CreatedAt  string  `json:"createdAt"`
	UpdatedAt  string  `json:"updatedAt"`
}

type challengeRequest struct {
	DeviceID string `json:"deviceId"`
}

type challengeResponse struct {
	ChallengeID string `json:"challengeId"`
	Challenge   string `json:"challenge"`
	ExpiresAt   string `json:"expiresAt"`
}

type connectionRequest struct {
	DeviceID    string `json:"deviceId"`
	ChallengeID string `json:"challengeId"`
	Signature   string `json:"signature"`
}

type connectionResponse struct {
	ConnectionToken string `json:"connectionToken"`
	ExpiresAt       string `json:"expiresAt"`
}

type startThreadRequest struct {
	DeviceID    string          `json:"deviceId"`
	ProfileID   string          `json:"profileId"`
	WorkspaceID string          `json:"workspaceId"`
	Title       string          `json:"title"`
	Settings    json.RawMessage `json:"settings"`
	Input       json.RawMessage `json:"input"`
}

type startTurnRequest struct {
	Input    json.RawMessage `json:"input"`
	Settings json.RawMessage `json:"settings"`
}

type respondInteractionRequest struct {
	Response json.RawMessage `json:"response"`
}

type renameThreadRequest struct {
	Name string `json:"name"`
}

type startReviewRequest struct {
	Target reviewTargetRequest `json:"target"`
}

type reviewTargetRequest struct {
	Kind   string `json:"kind"`
	Branch string `json:"branch"`
	SHA    string `json:"sha"`
}

type steerTurnRequest struct {
	Input json.RawMessage `json:"input"`
}

const (
	smallJSONBodyLimit = int64(8 * 1024)
	agentJSONBodyLimit = int64(1024*1024 + 128*1024)
)

func bindStrictJSON(c *gin.Context, destination any, limit int64) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("request body must contain exactly one JSON value")
		}
		return err
	}
	return nil
}

// CreateEnrollment godoc
// @Summary 创建本地 Agent 设备配对码
// @Tags agent
// @Security BearerAuth
// @Success 200 {object} EnrollmentResponseDoc
// @Failure 400,500 {object} ErrorDoc
// @Router /agent/devices/enrollments [post]
func (h *Handler) CreateEnrollment(c *gin.Context) {
	result, err := h.service.CreateEnrollment(c.Request.Context(), middleware.MustUserID(c))
	if err != nil {
		writeError(c, err, "create enrollment failed")
		return
	}
	response.Success(c, enrollmentResponse{EnrollmentCode: result.EnrollmentCode, ExpiresAt: result.ExpiresAt.Format(timeLayout)})
}

func (h *Handler) EnrollDevice(c *gin.Context) {
	var request enrollDeviceRequest
	if err := bindStrictJSON(c, &request, smallJSONBodyLimit); err != nil {
		response.InvalidRequestBody(c, err)
		return
	}
	result, err := h.service.EnrollDevice(c.Request.Context(), appagent.EnrollDeviceInput{
		EnrollmentCode: request.EnrollmentCode, Name: request.Name,
		Platform: request.Platform, PublicKey: request.PublicKey,
	})
	if err != nil {
		writeError(c, err, "enroll device failed")
		return
	}
	response.Success(c, enrollDeviceResponse{DeviceID: result.DeviceID, Status: result.Status})
}

// ListDevices godoc
// @Summary 获取当前用户的 Agent 设备
// @Tags agent
// @Security BearerAuth
// @Success 200 {object} DeviceListResponseDoc
// @Failure 400,500 {object} ErrorDoc
// @Router /agent/devices [get]
func (h *Handler) ListDevices(c *gin.Context) {
	items, err := h.service.ListDevices(c.Request.Context(), middleware.MustUserID(c), middleware.MustUserPublicID(c))
	if err != nil {
		writeError(c, err, "load devices failed")
		return
	}
	result := make([]deviceResponse, 0, len(items))
	for i := range items {
		result = append(result, toDeviceResponse(items[i]))
	}
	response.Success(c, result)
}

// GetDevice godoc
// @Summary 获取 Agent 设备
// @Tags agent
// @Security BearerAuth
// @Param device_id path string true "设备公开 ID"
// @Success 200 {object} DeviceResponseDoc
// @Failure 400,404,500 {object} ErrorDoc
// @Router /agent/devices/{device_id} [get]
func (h *Handler) GetDevice(c *gin.Context) {
	item, err := h.service.GetDevice(c.Request.Context(), middleware.MustUserID(c), middleware.MustUserPublicID(c), c.Param("device_id"))
	if err != nil {
		writeError(c, err, "load device failed")
		return
	}
	response.Success(c, toDeviceResponse(*item))
}

// RenameDevice godoc
// @Summary 重命名 Agent 设备
// @Tags agent
// @Security BearerAuth
// @Param device_id path string true "设备公开 ID"
// @Param body body renameDeviceRequest true "设备名称"
// @Success 200 {object} DeviceResponseDoc
// @Failure 400,404,500 {object} ErrorDoc
// @Router /agent/devices/{device_id} [patch]
func (h *Handler) RenameDevice(c *gin.Context) {
	var request renameDeviceRequest
	if err := bindStrictJSON(c, &request, smallJSONBodyLimit); err != nil {
		response.InvalidRequestBody(c, err)
		return
	}
	item, err := h.service.RenameDevice(c.Request.Context(), middleware.MustUserID(c), middleware.MustUserPublicID(c), c.Param("device_id"), request.Name)
	if err != nil {
		writeError(c, err, "rename device failed")
		return
	}
	response.Success(c, toDeviceResponse(*item))
}

// RevokeDevice godoc
// @Summary 撤销 Agent 设备
// @Tags agent
// @Security BearerAuth
// @Param device_id path string true "设备公开 ID"
// @Success 200 {object} DeviceRevokeResponseDoc
// @Failure 400,404,500 {object} ErrorDoc
// @Router /agent/devices/{device_id} [delete]
func (h *Handler) RevokeDevice(c *gin.Context) {
	if err := h.service.RevokeDevice(c.Request.Context(), middleware.MustUserID(c), c.Param("device_id")); err != nil {
		writeError(c, err, "revoke device failed")
		return
	}
	response.Success(c, gin.H{"revoked": true})
}

// ListRuntimeProfiles godoc
// @Summary 获取设备 Runtime Profile
// @Tags agent
// @Security BearerAuth
// @Param device_id path string true "设备公开 ID"
// @Success 200 {object} RuntimeProfileListResponseDoc
// @Failure 400,404,500 {object} ErrorDoc
// @Router /agent/devices/{device_id}/profiles [get]
func (h *Handler) ListRuntimeProfiles(c *gin.Context) {
	items, err := h.service.ListRuntimeProfiles(c.Request.Context(), middleware.MustUserID(c), c.Param("device_id"))
	if err != nil {
		writeError(c, err, "load runtime profiles failed")
		return
	}
	response.Success(c, toRuntimeProfileDocs(items))
}

// ListWorkspaces godoc
// @Summary 获取设备工作区
// @Tags agent
// @Security BearerAuth
// @Param device_id path string true "设备公开 ID"
// @Success 200 {object} WorkspaceListResponseDoc
// @Failure 400,404,500 {object} ErrorDoc
// @Router /agent/devices/{device_id}/workspaces [get]
func (h *Handler) ListWorkspaces(c *gin.Context) {
	items, err := h.service.ListWorkspaces(c.Request.Context(), middleware.MustUserID(c), c.Param("device_id"))
	if err != nil {
		writeError(c, err, "load agent workspaces failed")
		return
	}
	response.Success(c, toWorkspaceDocs(items))
}

// QueueProfileResourceRefresh godoc
// @Summary 刷新 Profile 本地资源快照
// @Tags agent
// @Security BearerAuth
// @Param device_id path string true "设备公开 ID"
// @Param profile_id path string true "Profile ID"
// @Param resource path string true "资源" Enums(models,permission-profiles,apps,mcp,plugins,auth-status)
// @Param Idempotency-Key header string true "UUID"
// @Success 200 {object} CommandResponseDoc
// @Failure 400,404,409,500 {object} ErrorDoc
// @Router /agent/devices/{device_id}/profiles/{profile_id}/resources/{resource}/refresh [post]
func (h *Handler) QueueProfileResourceRefresh(c *gin.Context) {
	h.queueResourceRefresh(c, c.Param("profile_id"), "")
}

// GetProfileResource godoc
// @Summary 获取 Profile 本地资源快照
// @Tags agent
// @Security BearerAuth
// @Param device_id path string true "设备公开 ID"
// @Param profile_id path string true "Profile ID"
// @Param resource path string true "资源" Enums(models,permission-profiles,apps,mcp,plugins,auth-status)
// @Success 200 {object} ResourceSnapshotResponseDoc
// @Failure 400,404,500 {object} ErrorDoc
// @Router /agent/devices/{device_id}/profiles/{profile_id}/resources/{resource} [get]
func (h *Handler) GetProfileResource(c *gin.Context) {
	h.getResource(c, c.Param("profile_id"), "")
}

// QueueWorkspaceResourceRefresh godoc
// @Summary 刷新 Workspace 本地资源快照
// @Tags agent
// @Security BearerAuth
// @Param device_id path string true "设备公开 ID"
// @Param workspace_id path string true "Workspace ID"
// @Param resource path string true "资源" Enums(sessions,skills,hooks)
// @Param Idempotency-Key header string true "UUID"
// @Success 200 {object} CommandResponseDoc
// @Failure 400,404,409,500 {object} ErrorDoc
// @Router /agent/devices/{device_id}/workspaces/{workspace_id}/resources/{resource}/refresh [post]
func (h *Handler) QueueWorkspaceResourceRefresh(c *gin.Context) {
	h.queueResourceRefresh(c, "", c.Param("workspace_id"))
}

// GetWorkspaceResource godoc
// @Summary 获取 Workspace 本地资源快照
// @Tags agent
// @Security BearerAuth
// @Param device_id path string true "设备公开 ID"
// @Param workspace_id path string true "Workspace ID"
// @Param resource path string true "资源" Enums(sessions,skills,hooks)
// @Success 200 {object} ResourceSnapshotResponseDoc
// @Failure 400,404,500 {object} ErrorDoc
// @Router /agent/devices/{device_id}/workspaces/{workspace_id}/resources/{resource} [get]
func (h *Handler) GetWorkspaceResource(c *gin.Context) {
	h.getResource(c, "", c.Param("workspace_id"))
}

func (h *Handler) queueResourceRefresh(c *gin.Context, profileID, workspaceID string) {
	result, err := h.service.QueueResourceRefresh(
		c.Request.Context(), middleware.MustUserID(c), c.Param("device_id"), profileID, workspaceID,
		c.Param("resource"), strings.TrimSpace(c.GetHeader("Idempotency-Key")),
	)
	if err != nil {
		writeError(c, err, "queue agent resource refresh failed")
		return
	}
	response.Success(c, toResourceCommandDoc(*result))
}

func (h *Handler) getResource(c *gin.Context, profileID, workspaceID string) {
	result, err := h.service.GetResourceSnapshot(
		c.Request.Context(), middleware.MustUserID(c), c.Param("device_id"), profileID, workspaceID, c.Param("resource"),
	)
	if err != nil {
		writeError(c, err, "load agent resource failed")
		return
	}
	response.Success(c, toResourceSnapshotDoc(*result))
}

// StartThread godoc
// @Summary 创建 Agent Thread
// @Tags agent
// @Security BearerAuth
// @Param Idempotency-Key header string true "UUID"
// @Param body body StartThreadRequestDoc true "Thread 参数"
// @Success 200 {object} StartThreadResponseDoc
// @Failure 400,404,409,500 {object} ErrorDoc
// @Router /agent/threads [post]
func (h *Handler) StartThread(c *gin.Context) {
	var request startThreadRequest
	if err := bindStrictJSON(c, &request, agentJSONBodyLimit); err != nil {
		response.InvalidRequestBody(c, err)
		return
	}
	result, err := h.service.StartThread(c.Request.Context(), middleware.MustUserID(c), appagent.StartThreadInput{
		DeviceID: request.DeviceID, ProfileID: request.ProfileID, WorkspaceID: request.WorkspaceID,
		Title: request.Title, Settings: request.Settings, InitialInput: request.Input,
		IdempotencyKey: strings.TrimSpace(c.GetHeader("Idempotency-Key")),
	})
	if err != nil {
		writeError(c, err, "start agent thread failed")
		return
	}
	response.Success(c, toStartThreadDataDoc(*result))
}

// ListThreads godoc
// @Summary 获取 Agent Thread
// @Tags agent
// @Security BearerAuth
// @Success 200 {object} ThreadListResponseDoc
// @Failure 500 {object} ErrorDoc
// @Router /agent/threads [get]
func (h *Handler) ListThreads(c *gin.Context) {
	items, err := h.service.ListThreads(c.Request.Context(), middleware.MustUserID(c))
	if err != nil {
		writeError(c, err, "load agent threads failed")
		return
	}
	response.Success(c, toThreadDocs(items))
}

// GetThread godoc
// @Summary 获取 Agent Thread 详情
// @Tags agent
// @Security BearerAuth
// @Param thread_id path string true "Thread ID"
// @Success 200 {object} ThreadResponseDoc
// @Failure 400,404,500 {object} ErrorDoc
// @Router /agent/threads/{thread_id} [get]
func (h *Handler) GetThread(c *gin.Context) {
	item, err := h.service.GetThread(c.Request.Context(), middleware.MustUserID(c), c.Param("thread_id"))
	if err != nil {
		writeError(c, err, "load agent thread failed")
		return
	}
	response.Success(c, toThreadDoc(*item))
}

// RenameThread godoc
// @Summary 重命名 Agent Thread
// @Tags agent
// @Security BearerAuth
// @Param thread_id path string true "Thread ID"
// @Param Idempotency-Key header string true "UUID"
// @Param body body RenameThreadRequestDoc true "新名称"
// @Success 200 {object} CommandResponseDoc
// @Failure 400,404,409,500 {object} ErrorDoc
// @Router /agent/threads/{thread_id}/name [patch]
func (h *Handler) RenameThread(c *gin.Context) {
	var request renameThreadRequest
	if err := bindStrictJSON(c, &request, smallJSONBodyLimit); err != nil {
		response.InvalidRequestBody(c, err)
		return
	}
	result, err := h.service.RenameThread(c.Request.Context(), middleware.MustUserID(c), c.Param("thread_id"), request.Name, idempotencyKey(c))
	if err != nil {
		writeError(c, err, "rename agent thread failed")
		return
	}
	response.Success(c, toCommandDoc(*result))
}

// ArchiveThread godoc
// @Summary 归档 Agent Thread
// @Tags agent
// @Security BearerAuth
// @Param thread_id path string true "Thread ID"
// @Param Idempotency-Key header string true "UUID"
// @Success 200 {object} CommandResponseDoc
// @Failure 400,404,409,500 {object} ErrorDoc
// @Router /agent/threads/{thread_id}/archive [post]
func (h *Handler) ArchiveThread(c *gin.Context) {
	h.changeThreadLifecycle(c, "archive")
}

// ForkThread godoc
// @Summary Fork Agent Thread
// @Tags agent
// @Security BearerAuth
// @Param thread_id path string true "Thread ID"
// @Param Idempotency-Key header string true "UUID"
// @Success 200 {object} ThreadResponseDoc
// @Failure 400,404,409,500 {object} ErrorDoc
// @Router /agent/threads/{thread_id}/fork [post]
func (h *Handler) ForkThread(c *gin.Context) {
	result, err := h.service.ForkThread(c.Request.Context(), middleware.MustUserID(c), c.Param("thread_id"), idempotencyKey(c))
	if err != nil {
		writeError(c, err, "fork agent thread failed")
		return
	}
	response.Success(c, toThreadDoc(*result))
}

// UnarchiveThread godoc
// @Summary 取消归档 Agent Thread
// @Tags agent
// @Security BearerAuth
// @Param thread_id path string true "Thread ID"
// @Param Idempotency-Key header string true "UUID"
// @Success 200 {object} CommandResponseDoc
// @Failure 400,404,409,500 {object} ErrorDoc
// @Router /agent/threads/{thread_id}/unarchive [post]
func (h *Handler) UnarchiveThread(c *gin.Context) {
	h.changeThreadLifecycle(c, "unarchive")
}

// ResumeThread godoc
// @Summary 恢复 Agent Thread
// @Tags agent
// @Security BearerAuth
// @Param thread_id path string true "Thread ID"
// @Param Idempotency-Key header string true "UUID"
// @Success 200 {object} CommandResponseDoc
// @Failure 400,404,409,500 {object} ErrorDoc
// @Router /agent/threads/{thread_id}/resume [post]
func (h *Handler) ResumeThread(c *gin.Context) {
	h.changeThreadLifecycle(c, "resume")
}

// DeleteThread godoc
// @Summary 删除 Agent Thread
// @Tags agent
// @Security BearerAuth
// @Param thread_id path string true "Thread ID"
// @Param Idempotency-Key header string true "UUID"
// @Success 200 {object} CommandResponseDoc
// @Failure 400,404,409,500 {object} ErrorDoc
// @Router /agent/threads/{thread_id} [delete]
func (h *Handler) DeleteThread(c *gin.Context) {
	h.changeThreadLifecycle(c, "delete")
}

func (h *Handler) changeThreadLifecycle(c *gin.Context, action string) {
	result, err := h.service.ChangeThreadLifecycle(c.Request.Context(), middleware.MustUserID(c), c.Param("thread_id"), action, idempotencyKey(c))
	if err != nil {
		writeError(c, err, action+" agent thread failed")
		return
	}
	response.Success(c, toCommandDoc(*result))
}

// CompactThread godoc
// @Summary 压缩 Agent Thread 上下文
// @Tags agent
// @Security BearerAuth
// @Param thread_id path string true "Thread ID"
// @Param Idempotency-Key header string true "UUID"
// @Success 200 {object} CommandResponseDoc
// @Failure 400,404,409,500 {object} ErrorDoc
// @Router /agent/threads/{thread_id}/compact [post]
func (h *Handler) CompactThread(c *gin.Context) {
	result, err := h.service.CompactThread(c.Request.Context(), middleware.MustUserID(c), c.Param("thread_id"), idempotencyKey(c))
	if err != nil {
		writeError(c, err, "compact agent thread failed")
		return
	}
	response.Success(c, toCommandDoc(*result))
}

// StartReview godoc
// @Summary 启动 Agent 代码审查
// @Tags agent
// @Security BearerAuth
// @Param thread_id path string true "Thread ID"
// @Param Idempotency-Key header string true "UUID"
// @Param body body StartReviewRequestDoc true "审查目标"
// @Success 200 {object} CommandResponseDoc
// @Failure 400,404,409,500 {object} ErrorDoc
// @Router /agent/threads/{thread_id}/reviews [post]
func (h *Handler) StartReview(c *gin.Context) {
	var request startReviewRequest
	if err := bindStrictJSON(c, &request, smallJSONBodyLimit); err != nil {
		response.InvalidRequestBody(c, err)
		return
	}
	result, err := h.service.StartReview(c.Request.Context(), middleware.MustUserID(c), c.Param("thread_id"), appagent.ReviewTargetInput{
		Kind: request.Target.Kind, Branch: request.Target.Branch, SHA: request.Target.SHA,
	}, idempotencyKey(c))
	if err != nil {
		writeError(c, err, "start agent review failed")
		return
	}
	response.Success(c, toCommandDoc(*result))
}

// StartTurn godoc
// @Summary 启动 Agent Turn
// @Tags agent
// @Security BearerAuth
// @Param thread_id path string true "Thread ID"
// @Param Idempotency-Key header string true "UUID"
// @Param body body StartTurnRequestDoc true "Turn 参数"
// @Success 200 {object} TurnResponseDoc
// @Failure 400,404,409,500 {object} ErrorDoc
// @Router /agent/threads/{thread_id}/turns [post]
func (h *Handler) StartTurn(c *gin.Context) {
	var request startTurnRequest
	if err := bindStrictJSON(c, &request, agentJSONBodyLimit); err != nil {
		response.InvalidRequestBody(c, err)
		return
	}
	item, err := h.service.StartTurn(c.Request.Context(), middleware.MustUserID(c), appagent.StartTurnInput{
		ThreadID: c.Param("thread_id"), Input: request.Input, Settings: request.Settings,
		IdempotencyKey: strings.TrimSpace(c.GetHeader("Idempotency-Key")),
	})
	if err != nil {
		writeError(c, err, "start agent turn failed")
		return
	}
	response.Success(c, toTurnDoc(*item))
}

// SteerTurn godoc
// @Summary 追加 Agent Turn 输入
// @Tags agent
// @Security BearerAuth
// @Param turn_id path string true "Turn ID"
// @Param Idempotency-Key header string true "UUID"
// @Param body body SteerTurnRequestDoc true "追加输入"
// @Success 200 {object} CommandResponseDoc
// @Failure 400,404,409,500 {object} ErrorDoc
// @Router /agent/turns/{turn_id}/steer [post]
func (h *Handler) SteerTurn(c *gin.Context) {
	var request steerTurnRequest
	if err := bindStrictJSON(c, &request, agentJSONBodyLimit); err != nil {
		response.InvalidRequestBody(c, err)
		return
	}
	result, err := h.service.SteerTurn(c.Request.Context(), middleware.MustUserID(c), c.Param("turn_id"), request.Input, idempotencyKey(c))
	if err != nil {
		writeError(c, err, "steer agent turn failed")
		return
	}
	response.Success(c, toCommandDoc(*result))
}

// InterruptTurn godoc
// @Summary 中断 Agent Turn
// @Tags agent
// @Security BearerAuth
// @Param turn_id path string true "Turn ID"
// @Param Idempotency-Key header string true "UUID"
// @Success 200 {object} CommandResponseDoc
// @Failure 400,404,409,500 {object} ErrorDoc
// @Router /agent/turns/{turn_id}/interrupt [post]
func (h *Handler) InterruptTurn(c *gin.Context) {
	result, err := h.service.InterruptTurn(c.Request.Context(), middleware.MustUserID(c), c.Param("turn_id"), idempotencyKey(c))
	if err != nil {
		writeError(c, err, "interrupt agent turn failed")
		return
	}
	response.Success(c, toCommandDoc(*result))
}

// ListTurns godoc
// @Summary 获取 Agent Turn
// @Tags agent
// @Security BearerAuth
// @Param thread_id path string true "Thread ID"
// @Success 200 {object} TurnListResponseDoc
// @Failure 400,404,500 {object} ErrorDoc
// @Router /agent/threads/{thread_id}/turns [get]
func (h *Handler) ListTurns(c *gin.Context) {
	items, err := h.service.ListTurns(c.Request.Context(), middleware.MustUserID(c), c.Param("thread_id"))
	if err != nil {
		writeError(c, err, "load agent turns failed")
		return
	}
	response.Success(c, toTurnDocs(items))
}

// ListEvents godoc
// @Summary 重放 Agent Thread 事件
// @Tags agent
// @Security BearerAuth
// @Param thread_id path string true "Thread ID"
// @Param after_seq query int false "已消费的 Thread 事件序号"
// @Success 200 {object} EventListResponseDoc
// @Failure 400,404,500 {object} ErrorDoc
// @Router /agent/threads/{thread_id}/events [get]
func (h *Handler) ListEvents(c *gin.Context) {
	after := uint64(0)
	if value := strings.TrimSpace(c.Query("after_seq")); value != "" {
		parsed, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			response.Error(c, http.StatusBadRequest, "invalid after_seq")
			return
		}
		after = parsed
	}
	items, err := h.service.ListEvents(c.Request.Context(), middleware.MustUserID(c), c.Param("thread_id"), after)
	if err != nil {
		writeError(c, err, "load agent events failed")
		return
	}
	response.Success(c, toEventDocs(items))
}

// ListInteractions godoc
// @Summary 获取 Agent 交互请求
// @Tags agent
// @Security BearerAuth
// @Param thread_id path string true "Thread ID"
// @Param status query string false "状态" Enums(pending,responding,resolved,failed)
// @Success 200 {object} InteractionListResponseDoc
// @Failure 400,404,500 {object} ErrorDoc
// @Router /agent/threads/{thread_id}/interactions [get]
func (h *Handler) ListInteractions(c *gin.Context) {
	items, err := h.service.ListInteractions(c.Request.Context(), middleware.MustUserID(c), c.Param("thread_id"), c.Query("status"))
	if err != nil {
		writeError(c, err, "load agent interactions failed")
		return
	}
	response.Success(c, toInteractionDocs(items))
}

// RespondInteraction godoc
// @Summary 响应 Agent 交互请求
// @Tags agent
// @Security BearerAuth
// @Param interaction_id path string true "Interaction ID"
// @Param Idempotency-Key header string true "UUID"
// @Param body body RespondInteractionRequestDoc true "类型化响应"
// @Success 200 {object} InteractionResponseDoc
// @Failure 400,404,409,500 {object} ErrorDoc
// @Router /agent/interactions/{interaction_id}/respond [post]
func (h *Handler) RespondInteraction(c *gin.Context) {
	var request respondInteractionRequest
	if err := bindStrictJSON(c, &request, agentJSONBodyLimit); err != nil {
		response.InvalidRequestBody(c, err)
		return
	}
	item, err := h.service.RespondInteraction(c.Request.Context(), middleware.MustUserID(c), appagent.RespondInteractionInput{
		InteractionID: c.Param("interaction_id"), Response: request.Response,
		IdempotencyKey: strings.TrimSpace(c.GetHeader("Idempotency-Key")),
	})
	if err != nil {
		writeError(c, err, "respond to agent interaction failed")
		return
	}
	response.Success(c, toInteractionDoc(*item))
}

func (h *Handler) CreateChallenge(c *gin.Context) {
	var request challengeRequest
	if err := bindStrictJSON(c, &request, smallJSONBodyLimit); err != nil {
		response.InvalidRequestBody(c, err)
		return
	}
	result, err := h.service.CreateChallenge(c.Request.Context(), request.DeviceID)
	if err != nil {
		writeError(c, err, "create challenge failed")
		return
	}
	response.Success(c, challengeResponse{
		ChallengeID: result.ChallengeID, Challenge: result.Challenge, ExpiresAt: result.ExpiresAt.Format(timeLayout),
	})
}

func (h *Handler) IssueConnection(c *gin.Context) {
	var request connectionRequest
	if err := bindStrictJSON(c, &request, smallJSONBodyLimit); err != nil {
		response.InvalidRequestBody(c, err)
		return
	}
	result, err := h.service.IssueConnection(c.Request.Context(), request.DeviceID, request.ChallengeID, request.Signature)
	if err != nil {
		writeError(c, err, "issue connection token failed")
		return
	}
	response.Success(c, connectionResponse{ConnectionToken: result.ConnectionToken, ExpiresAt: result.ExpiresAt.Format(timeLayout)})
}

func (h *Handler) ConnectBridge(c *gin.Context) {
	h.connect(c.Writer, c.Request)
}

func idempotencyKey(c *gin.Context) string {
	return strings.TrimSpace(c.GetHeader("Idempotency-Key"))
}

const timeLayout = "2006-01-02T15:04:05.000Z07:00"

func toDeviceResponse(item appagent.DeviceView) deviceResponse {
	var lastSeenAt *string
	if item.LastSeenAt != nil {
		value := item.LastSeenAt.Format(timeLayout)
		lastSeenAt = &value
	}
	return deviceResponse{
		DeviceID: item.DeviceID, UserID: item.UserID, Name: item.Name,
		Platform: item.Platform, Status: item.Status, LastSeenAt: lastSeenAt,
		CreatedAt: item.CreatedAt.Format(timeLayout), UpdatedAt: item.UpdatedAt.Format(timeLayout),
	}
}

func toRuntimeProfileDocs(items []appagent.RuntimeProfileView) []RuntimeProfileDoc {
	result := make([]RuntimeProfileDoc, 0, len(items))
	for _, item := range items {
		result = append(result, RuntimeProfileDoc{
			ProfileID: item.ProfileID, DeviceID: item.DeviceID, Provider: item.Provider, Status: item.Status,
			VerifiedAt: item.VerifiedAt, LeaseExpiresAt: item.LeaseExpiresAt,
		})
	}
	return result
}

func toWorkspaceDocs(items []appagent.WorkspaceView) []WorkspaceDoc {
	result := make([]WorkspaceDoc, 0, len(items))
	for _, item := range items {
		result = append(result, WorkspaceDoc{
			WorkspaceID: item.WorkspaceID, DeviceID: item.DeviceID, ProfileID: item.ProfileID,
			Name: item.Name, Status: item.Status, LastSeenAt: item.LastSeenAt,
		})
	}
	return result
}

func toResourceCommandDoc(item appagent.ResourceRefreshView) CommandDoc {
	return CommandDoc{CommandID: item.CommandID, Status: item.Status}
}

func toCommandDoc(item appagent.CommandView) CommandDoc {
	return CommandDoc{CommandID: item.CommandID, Status: item.Status}
}

func toResourceSnapshotDoc(item appagent.ResourceSnapshotView) ResourceSnapshotDoc {
	return ResourceSnapshotDoc{
		Resource: item.Resource, Scope: item.Scope, DeviceID: item.DeviceID, ProfileID: item.ProfileID,
		WorkspaceID: item.WorkspaceID, Data: item.Data, RefreshedAt: item.RefreshedAt,
	}
}

func toThreadDoc(item appagent.ThreadView) ThreadDoc {
	return ThreadDoc{
		ThreadID: item.ThreadID, DeviceID: item.DeviceID, ProfileID: item.ProfileID, WorkspaceID: item.WorkspaceID,
		Title: item.Title, Status: item.Status, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
	}
}

func toThreadDocs(items []appagent.ThreadView) []ThreadDoc {
	result := make([]ThreadDoc, 0, len(items))
	for _, item := range items {
		result = append(result, toThreadDoc(item))
	}
	return result
}

func toTurnDoc(item appagent.TurnView) TurnDoc {
	return TurnDoc{
		TurnID: item.TurnID, ThreadID: item.ThreadID, Status: item.Status,
		CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
	}
}

func toTurnDocs(items []appagent.TurnView) []TurnDoc {
	result := make([]TurnDoc, 0, len(items))
	for _, item := range items {
		result = append(result, toTurnDoc(item))
	}
	return result
}

func toStartThreadDataDoc(item appagent.StartThreadResult) StartThreadDataDoc {
	result := StartThreadDataDoc{Thread: toThreadDoc(item.Thread)}
	if item.Turn != nil {
		turn := toTurnDoc(*item.Turn)
		result.Turn = &turn
	}
	return result
}

func toEventDocs(items []appagent.EventView) []EventDoc {
	result := make([]EventDoc, 0, len(items))
	for _, item := range items {
		result = append(result, EventDoc{
			EventID: item.EventID, ThreadID: item.ThreadID, TurnID: item.TurnID, Seq: item.Seq,
			Kind: item.Kind, Payload: item.Payload, OccurredAt: item.OccurredAt,
		})
	}
	return result
}

func toInteractionDoc(item appagent.InteractionView) InteractionDoc {
	return InteractionDoc{
		InteractionID: item.InteractionID, ThreadID: item.ThreadID, TurnID: item.TurnID,
		Kind: item.Kind, Status: item.Status, Request: item.Request, CreatedAt: item.CreatedAt,
	}
}

func toInteractionDocs(items []appagent.InteractionView) []InteractionDoc {
	result := make([]InteractionDoc, 0, len(items))
	for _, item := range items {
		result = append(result, toInteractionDoc(item))
	}
	return result
}

func writeError(c *gin.Context, err error, fallback string) {
	switch {
	case errors.Is(err, appagent.ErrInvalidInput):
		response.Error(c, http.StatusBadRequest, "invalid agent gateway input")
	case errors.Is(err, appagent.ErrDeviceNotFound):
		response.Error(c, http.StatusNotFound, "agent device not found")
	case errors.Is(err, appagent.ErrResourceNotFound):
		response.Error(c, http.StatusNotFound, "agent resource not found")
	case errors.Is(err, appagent.ErrStateConflict):
		response.Error(c, http.StatusConflict, "agent resource state conflicts with request")
	case errors.Is(err, appagent.ErrCredential), errors.Is(err, appagent.ErrInvalidSignature):
		response.Error(c, http.StatusUnauthorized, "invalid device credential")
	case errors.Is(err, appagent.ErrDeviceRevoked):
		response.Error(c, http.StatusConflict, "agent device revoked")
	default:
		response.Error(c, http.StatusInternalServerError, strings.TrimSpace(fallback))
	}
}
