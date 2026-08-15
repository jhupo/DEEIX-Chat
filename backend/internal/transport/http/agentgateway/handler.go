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
	hub := newBridgeHub()
	service.SetNotifier(hub.notifyUser)
	return &Handler{service: service, hub: hub}
}

type enrollmentChallengeRequest struct {
	UserPublicID string `json:"userPublicID"`
	Name         string `json:"name"`
	Platform     string `json:"platform"`
	PublicKey    string `json:"publicKey"`
}
type enrollmentChallengeResponse struct {
	ChallengeID string `json:"challengeId"`
	Canonical   string `json:"canonical"`
	ExpiresAt   string `json:"expiresAt"`
}
type completeEnrollmentRequest struct {
	ChallengeID string `json:"challengeId"`
	Proof       string `json:"proof"`
	Signature   string `json:"signature"`
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
	Online     bool    `json:"online"`
	CreatedAt  string  `json:"createdAt"`
	UpdatedAt  string  `json:"updatedAt"`
	LastSeenAt *string `json:"lastSeenAt"`
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
type createArtifactRequest struct {
	FileID string `json:"fileId"`
}
type registerWorkspaceRequest struct {
	ProfileID string `json:"profileId"`
	Path      string `json:"path"`
	Create    bool   `json:"create"`
}

const (
	smallJSONBodyLimit = int64(8 * 1024)
	agentJSONBodyLimit = int64(1024*1024 + 128*1024)
	timeLayout         = "2006-01-02T15:04:05.000Z07:00"
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

// BeginEnrollment godoc
// @Summary Create a gateway device enrollment challenge
// @Tags agent-gateway
// @Success 200 {object} EnrollmentChallengeResponseDoc
// @Router /agent/bridge/enrollment-challenges [post]
func (h *Handler) BeginEnrollment(c *gin.Context) {
	var request enrollmentChallengeRequest
	if err := bindStrictJSON(c, &request, smallJSONBodyLimit); err != nil {
		response.InvalidRequestBody(c, err)
		return
	}
	result, err := h.service.BeginEnrollment(c.Request.Context(), appagent.BeginEnrollmentInput{
		UserPublicID: request.UserPublicID, Name: request.Name,
		Platform: request.Platform, PublicKey: request.PublicKey,
	})
	if err != nil {
		writeError(c, err, "create enrollment challenge failed")
		return
	}
	response.Success(c, enrollmentChallengeResponse{
		ChallengeID: result.ChallengeID, Canonical: result.Canonical,
		ExpiresAt: result.ExpiresAt.Format(timeLayout),
	})
}

func (h *Handler) CompleteEnrollment(c *gin.Context) {
	var request completeEnrollmentRequest
	if err := bindStrictJSON(c, &request, smallJSONBodyLimit); err != nil {
		response.InvalidRequestBody(c, err)
		return
	}
	result, err := h.service.CompleteEnrollment(c.Request.Context(), appagent.CompleteEnrollmentInput{
		ChallengeID: request.ChallengeID, Proof: request.Proof, Signature: request.Signature,
	})
	if err != nil {
		writeError(c, err, "enroll device failed")
		return
	}
	response.Success(c, enrollDeviceResponse{DeviceID: result.DeviceID, Status: result.Status})
}

// ListDevices godoc
// @Summary List gateway devices
// @Tags agent-gateway
// @Security BearerAuth
// @Success 200 {object} DeviceListResponseDoc
// @Router /agent/devices [get]
func (h *Handler) ListDevices(c *gin.Context) {
	items, err := h.service.ListDevices(c.Request.Context(), middleware.MustUserID(c), middleware.MustUserPublicID(c))
	if err != nil {
		writeError(c, err, "load devices failed")
		return
	}
	result := make([]deviceResponse, 0, len(items))
	for _, item := range items {
		result = append(result, h.toDeviceResponse(item))
	}
	response.Success(c, result)
}

// GetDevice godoc
// @Summary Get a gateway device
// @Tags agent-gateway
// @Security BearerAuth
// @Param device_id path string true "Device public ID"
// @Success 200 {object} DeviceResponseDoc
// @Router /agent/devices/{device_id} [get]
func (h *Handler) GetDevice(c *gin.Context) {
	item, err := h.service.GetDevice(c.Request.Context(), middleware.MustUserID(c), middleware.MustUserPublicID(c), c.Param("device_id"))
	if err != nil {
		writeError(c, err, "load device failed")
		return
	}
	response.Success(c, h.toDeviceResponse(*item))
}

// RenameDevice godoc
// @Summary Rename a gateway device
// @Tags agent-gateway
// @Accept json
// @Security BearerAuth
// @Param device_id path string true "Device public ID"
// @Param body body renameDeviceRequest true "Device name"
// @Success 200 {object} DeviceResponseDoc
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
	response.Success(c, h.toDeviceResponse(*item))
}

// RevokeDevice godoc
// @Summary Revoke a gateway device
// @Tags agent-gateway
// @Security BearerAuth
// @Param device_id path string true "Device public ID"
// @Success 200 {object} DeviceRevokeResponseDoc
// @Router /agent/devices/{device_id} [delete]
func (h *Handler) RevokeDevice(c *gin.Context) {
	deviceID := c.Param("device_id")
	if err := h.service.RevokeDevice(c.Request.Context(), middleware.MustUserID(c), deviceID); err != nil {
		writeError(c, err, "revoke device failed")
		return
	}
	h.hub.disconnect(deviceID)
	response.Success(c, gin.H{"revoked": true})
}

// ListRuntimeProfiles godoc
// @Summary List device runtime profiles
// @Tags agent-gateway
// @Security BearerAuth
// @Param device_id path string true "Device public ID"
// @Success 200 {object} RuntimeProfileListResponseDoc
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
// @Summary List device workspaces
// @Tags agent-gateway
// @Security BearerAuth
// @Param device_id path string true "Device public ID"
// @Success 200 {object} WorkspaceListResponseDoc
// @Router /agent/devices/{device_id}/workspaces [get]
func (h *Handler) ListWorkspaces(c *gin.Context) {
	items, err := h.service.ListWorkspaces(c.Request.Context(), middleware.MustUserID(c), c.Param("device_id"))
	if err != nil {
		writeError(c, err, "load agent workspaces failed")
		return
	}
	response.Success(c, toWorkspaceDocs(items))
}

// RegisterWorkspace queues local directory validation and registration on the selected device.
func (h *Handler) RegisterWorkspace(c *gin.Context) {
	var request registerWorkspaceRequest
	if err := bindStrictJSON(c, &request, smallJSONBodyLimit); err != nil {
		response.InvalidRequestBody(c, err)
		return
	}
	result, err := h.service.RegisterWorkspace(c.Request.Context(), middleware.MustUserID(c), appagent.RegisterWorkspaceInput{
		DeviceID: c.Param("device_id"), ProfileID: request.ProfileID, Path: request.Path, Create: request.Create,
		IdempotencyKey: strings.TrimSpace(c.GetHeader("Idempotency-Key")),
	})
	if err != nil {
		writeError(c, err, "register agent workspace failed")
		return
	}
	response.Success(c, CommandDoc{CommandID: result.CommandID, Status: result.Status})
}

func (h *Handler) GetCommand(c *gin.Context) {
	result, err := h.service.GetCommand(c.Request.Context(), middleware.MustUserID(c), c.Param("command_id"))
	if err != nil {
		writeError(c, err, "load agent command failed")
		return
	}
	response.Success(c, CommandDoc{CommandID: result.CommandID, Status: result.Status, ErrorMessage: result.ErrorMessage})
}

// QueueProfileResourceRefresh godoc
// @Summary Refresh a runtime profile resource
// @Tags agent-gateway
// @Security BearerAuth
// @Param device_id path string true "Device public ID"
// @Param profile_id path string true "Profile ID"
// @Param resource path string true "Resource name"
// @Param Idempotency-Key header string true "Idempotency key"
// @Success 200 {object} CommandResponseDoc
// @Router /agent/devices/{device_id}/profiles/{profile_id}/resources/{resource}/refresh [post]
func (h *Handler) QueueProfileResourceRefresh(c *gin.Context) {
	h.queueResourceRefresh(c, c.Param("profile_id"), "")
}

// GetProfileResource godoc
// @Summary Get a runtime profile resource
// @Tags agent-gateway
// @Security BearerAuth
// @Param device_id path string true "Device public ID"
// @Param profile_id path string true "Profile ID"
// @Param resource path string true "Resource name"
// @Success 200 {object} ResourceSnapshotResponseDoc
// @Router /agent/devices/{device_id}/profiles/{profile_id}/resources/{resource} [get]
func (h *Handler) GetProfileResource(c *gin.Context) { h.getResource(c, c.Param("profile_id"), "") }

// QueueWorkspaceResourceRefresh godoc
// @Summary Refresh a workspace resource
// @Tags agent-gateway
// @Security BearerAuth
// @Param device_id path string true "Device public ID"
// @Param workspace_id path string true "Workspace ID"
// @Param resource path string true "Resource name"
// @Param Idempotency-Key header string true "Idempotency key"
// @Success 200 {object} CommandResponseDoc
// @Router /agent/devices/{device_id}/workspaces/{workspace_id}/resources/{resource}/refresh [post]
func (h *Handler) QueueWorkspaceResourceRefresh(c *gin.Context) {
	h.queueResourceRefresh(c, "", c.Param("workspace_id"))
}

// GetWorkspaceResource godoc
// @Summary Get a workspace resource
// @Tags agent-gateway
// @Security BearerAuth
// @Param device_id path string true "Device public ID"
// @Param workspace_id path string true "Workspace ID"
// @Param resource path string true "Resource name"
// @Success 200 {object} ResourceSnapshotResponseDoc
// @Router /agent/devices/{device_id}/workspaces/{workspace_id}/resources/{resource} [get]
func (h *Handler) GetWorkspaceResource(c *gin.Context) { h.getResource(c, "", c.Param("workspace_id")) }

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

// CreateArtifact godoc
// @Summary Bind an uploaded file to a gateway workspace
// @Tags agent-gateway
// @Accept json
// @Security BearerAuth
// @Param workspace_id path string true "Workspace ID"
// @Param body body createArtifactRequest true "File reference"
// @Success 200 {object} ArtifactResponseDoc
// @Router /agent/workspaces/{workspace_id}/artifacts [post]
func (h *Handler) CreateArtifact(c *gin.Context) {
	var request createArtifactRequest
	if err := bindStrictJSON(c, &request, smallJSONBodyLimit); err != nil {
		response.InvalidRequestBody(c, err)
		return
	}
	item, err := h.service.CreateArtifact(c.Request.Context(), middleware.MustUserID(c), c.Param("workspace_id"), request.FileID)
	if err != nil {
		writeError(c, err, "create agent artifact failed")
		return
	}
	response.Success(c, ArtifactDoc{ArtifactID: item.ArtifactID, WorkspaceID: item.WorkspaceID, FileName: item.FileName, MimeType: item.MimeType, SizeBytes: item.SizeBytes, SHA256: item.SHA256})
}

func (h *Handler) GetArtifactContent(c *gin.Context) {
	authorization := strings.TrimSpace(c.GetHeader("Authorization"))
	grant := strings.TrimPrefix(authorization, "Bearer deeix_artifact_")
	if grant == authorization {
		grant = ""
	}
	content, err := h.service.OpenArtifact(c.Request.Context(), c.Param("artifact_id"), strings.TrimSpace(c.Query("command")), strings.TrimSpace(c.Query("expires")), grant)
	if err != nil {
		writeError(c, err, "open agent artifact failed")
		return
	}
	defer content.Reader.Close() //nolint:errcheck
	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Length", strconv.FormatInt(content.SizeBytes, 10))
	c.Header("Cache-Control", "no-store")
	c.Status(http.StatusOK)
	_, _ = io.Copy(c.Writer, content.Reader)
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
	response.Success(c, challengeResponse{ChallengeID: result.ChallengeID, Challenge: result.Challenge, ExpiresAt: result.ExpiresAt.Format(timeLayout)})
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

func (h *Handler) ConnectBridge(c *gin.Context) { h.connect(c.Writer, c.Request) }

func (h *Handler) toDeviceResponse(item appagent.DeviceView) deviceResponse {
	var lastSeenAt *string
	if item.LastSeenAt != nil {
		value := item.LastSeenAt.Format(timeLayout)
		lastSeenAt = &value
	}
	return deviceResponse{
		DeviceID: item.DeviceID, UserID: item.UserID, Name: item.Name, Platform: item.Platform,
		Status: item.Status, Online: item.Online, LastSeenAt: lastSeenAt,
		CreatedAt: item.CreatedAt.Format(timeLayout), UpdatedAt: item.UpdatedAt.Format(timeLayout),
	}
}

func toRuntimeProfileDocs(items []appagent.RuntimeProfileView) []RuntimeProfileDoc {
	result := make([]RuntimeProfileDoc, 0, len(items))
	for _, item := range items {
		manifest := ProviderManifestDoc{}
		_ = json.Unmarshal(item.Manifest, &manifest)
		result = append(result, RuntimeProfileDoc{ProfileID: item.ProfileID, DeviceID: item.DeviceID, Provider: item.Provider, Status: item.Status, VerifiedAt: item.VerifiedAt, LeaseExpiresAt: item.LeaseExpiresAt, Manifest: manifest})
	}
	return result
}
func toWorkspaceDocs(items []appagent.WorkspaceView) []WorkspaceDoc {
	result := make([]WorkspaceDoc, 0, len(items))
	for _, item := range items {
		result = append(result, WorkspaceDoc{WorkspaceID: item.WorkspaceID, DeviceID: item.DeviceID, ProfileID: item.ProfileID, Name: item.Name, Status: item.Status, LastSeenAt: item.LastSeenAt})
	}
	return result
}
func toResourceCommandDoc(item appagent.ResourceRefreshView) CommandDoc {
	return CommandDoc{CommandID: item.CommandID, Status: item.Status}
}
func toResourceSnapshotDoc(item appagent.ResourceSnapshotView) ResourceSnapshotDoc {
	return ResourceSnapshotDoc{Resource: item.Resource, Scope: item.Scope, DeviceID: item.DeviceID, ProfileID: item.ProfileID, WorkspaceID: item.WorkspaceID, Data: item.Data, RefreshedAt: item.RefreshedAt}
}

func writeError(c *gin.Context, err error, fallback string) {
	switch {
	case errors.Is(err, appagent.ErrInvalidInput):
		response.Error(c, http.StatusBadRequest, "invalid agent gateway input")
	case errors.Is(err, appagent.ErrDeviceNotFound), errors.Is(err, appagent.ErrResourceNotFound):
		response.Error(c, http.StatusNotFound, "agent resource not found")
	case errors.Is(err, appagent.ErrStateConflict), errors.Is(err, appagent.ErrDeviceRevoked):
		response.Error(c, http.StatusConflict, "agent resource state conflicts with request")
	case errors.Is(err, appagent.ErrCredential), errors.Is(err, appagent.ErrInvalidSignature):
		response.Error(c, http.StatusUnauthorized, "invalid device credential")
	case errors.Is(err, appagent.ErrRuntimeAuth):
		response.ErrorWithCode(c, http.StatusUnauthorized, "agent.runtime_key_invalid", "local Codex API key is not active for this DEEIX account")
	default:
		response.Error(c, http.StatusInternalServerError, strings.TrimSpace(fallback))
	}
}
