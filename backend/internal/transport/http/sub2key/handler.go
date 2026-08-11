package sub2key

import (
	"errors"
	"net/http"
	"strings"
	"time"

	app "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/sub2key"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/response"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/transport/http/middleware"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type Handler struct{ service *app.Service }

type bindRequest struct {
	RemoteKeyID int64 `json:"remoteKeyID"`
}

type createRemoteKeyRequest struct {
	Name    string `json:"name" binding:"required,max=100"`
	GroupID int64  `json:"groupID" binding:"required,gt=0"`
}

type bindingResponse struct {
	PublicID        string     `json:"publicID"`
	RemoteKeyID     int64      `json:"remoteKeyID"`
	Label           string     `json:"label"`
	MaskedKey       string     `json:"maskedKey"`
	GroupID         *int64     `json:"groupID,omitempty"`
	GroupName       string     `json:"groupName,omitempty"`
	GroupPlatform   string     `json:"groupPlatform,omitempty"`
	Status          string     `json:"status"`
	Quota           float64    `json:"quota"`
	UsedQuota       float64    `json:"quotaUsed"`
	ExpiresAt       *time.Time `json:"expiresAt"`
	Version         uint       `json:"version"`
	LastValidatedAt *time.Time `json:"lastValidatedAt,omitempty"`
}

type remoteKeyResponse struct {
	RemoteKeyID     int64      `json:"remoteKeyID"`
	Label           string     `json:"label"`
	MaskedKey       string     `json:"maskedKey"`
	GroupID         *int64     `json:"groupID"`
	GroupName       string     `json:"groupName"`
	GroupPlatform   string     `json:"groupPlatform"`
	Status          string     `json:"status"`
	Quota           float64    `json:"quota"`
	UsedQuota       float64    `json:"quotaUsed"`
	ExpiresAt       *time.Time `json:"expiresAt"`
	Bound           bool       `json:"bound"`
	BindingPublicID *string    `json:"bindingPublicID"`
}

type groupResponse struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Platform    string `json:"platform"`
}

func NewHandler(service *app.Service) *Handler { return &Handler{service: service} }
func noStore(c *gin.Context)                   { c.Header("Cache-Control", "no-store") }
func (h *Handler) ListRemote(c *gin.Context) {
	noStore(c)
	items, err := h.service.ListRemote(c, middleware.MustUserID(c), middleware.MustSessionID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.Success(c, remoteKeyResponses(items))
}
func (h *Handler) ListGroups(c *gin.Context) {
	noStore(c)
	items, err := h.service.ListGroups(c, middleware.MustUserID(c), middleware.MustSessionID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	result := make([]groupResponse, 0, len(items))
	for _, item := range items {
		result = append(result, groupResponse{ID: item.ID, Name: item.Name, Description: item.Description, Platform: item.Platform})
	}
	response.Success(c, result)
}
func (h *Handler) CreateRemote(c *gin.Context) {
	noStore(c)
	var req createRemoteKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.InvalidRequestBody(c, err)
		return
	}
	idempotencyKey := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	parsed, err := uuid.Parse(idempotencyKey)
	if err != nil || parsed.String() != strings.ToLower(idempotencyKey) {
		response.Error(c, http.StatusBadRequest, "idempotency key must be a UUID")
		return
	}
	item, err := h.service.CreateRemote(c, middleware.MustUserID(c), middleware.MustSessionID(c), req.Name, req.GroupID, idempotencyKey)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.Success(c, toRemoteKeyResponse(*item))
}
func (h *Handler) ListBindings(c *gin.Context) {
	noStore(c)
	items, err := h.service.List(c, middleware.MustUserID(c), middleware.MustSessionID(c))
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.Success(c, bindingResponses(items))
}
func (h *Handler) Bind(c *gin.Context) {
	noStore(c)
	var req bindRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.InvalidRequestBody(c, err)
		return
	}
	idempotencyKey := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	parsed, err := uuid.Parse(idempotencyKey)
	if err != nil || parsed.String() != strings.ToLower(idempotencyKey) {
		response.Error(c, http.StatusBadRequest, "idempotency key must be a UUID")
		return
	}
	item, err := h.service.Bind(c, middleware.MustUserID(c), middleware.MustSessionID(c), req.RemoteKeyID, idempotencyKey)
	if err != nil {
		h.writeError(c, err)
		return
	}
	response.Success(c, toBindingResponse(*item))
}
func (h *Handler) Delete(c *gin.Context) {
	noStore(c)
	err := h.service.Delete(c, middleware.MustUserID(c), c.Param("public_id"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
func (h *Handler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, app.ErrBindingUnavailable):
		response.Error(c, http.StatusNotFound, "key binding not found")
	case errors.Is(err, app.ErrInvalidBinding):
		response.Error(c, http.StatusBadRequest, "invalid key binding")
	case errors.Is(err, app.ErrIdempotencyConflict):
		response.Error(c, http.StatusConflict, "idempotency key conflicts with request")
	default:
		response.Error(c, http.StatusBadGateway, "Sub2 service unavailable")
	}
}

func toBindingResponse(item app.BindingView) bindingResponse {
	return bindingResponse{PublicID: item.PublicID, RemoteKeyID: item.RemoteKeyID, Label: item.Label, MaskedKey: item.MaskedKey, GroupID: item.GroupID, GroupName: item.GroupName, GroupPlatform: item.GroupPlatform, Status: item.Status, Quota: item.Quota, UsedQuota: item.UsedQuota, ExpiresAt: item.ExpiresAt, Version: item.Version, LastValidatedAt: item.LastValidatedAt}
}

func bindingResponses(items []app.BindingView) []bindingResponse {
	out := make([]bindingResponse, 0, len(items))
	for _, item := range items {
		out = append(out, toBindingResponse(item))
	}
	return out
}

func remoteKeyResponses(items []app.RemoteKeyView) []remoteKeyResponse {
	out := make([]remoteKeyResponse, 0, len(items))
	for _, item := range items {
		out = append(out, toRemoteKeyResponse(item))
	}
	return out
}

func toRemoteKeyResponse(item app.RemoteKeyView) remoteKeyResponse {
	return remoteKeyResponse{RemoteKeyID: item.RemoteKeyID, Label: item.Label, MaskedKey: item.MaskedKey, GroupID: item.GroupID, GroupName: item.GroupName, GroupPlatform: item.GroupPlatform, Status: item.Status, Quota: item.Quota, UsedQuota: item.UsedQuota, ExpiresAt: item.ExpiresAt, Bound: item.Bound, BindingPublicID: item.BindingPublicID}
}
