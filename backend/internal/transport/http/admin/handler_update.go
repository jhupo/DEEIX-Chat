package admin

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"time"

	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/response"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/transport/http/middleware"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/update"
	"github.com/gin-gonic/gin"
)

var idempotencyKeyPattern = regexp.MustCompile(`^[A-Za-z0-9._~-]{16,128}$`)

type updateInstallRequest struct {
	Version        string `json:"version" binding:"required,max=32"`
	ManifestDigest string `json:"manifestDigest" binding:"required,max=71"`
	Confirmation   string `json:"confirmation" binding:"required,max=128"`
}

func (h *Handler) updateContext(c *gin.Context) (context.Context, context.CancelFunc, bool) {
	if h.updater == nil {
		response.ErrorWithCode(c, http.StatusServiceUnavailable, "updater_unavailable", "updater unavailable")
		return nil, nil, false
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	return ctx, cancel, true
}

func updateError(c *gin.Context, err error) {
	status, code := mapUpdateError(err)
	response.ErrorWithCode(c, status, code, "updater unavailable")
}

func mapUpdateError(err error) (int, string) {
	if err == nil {
		return http.StatusBadGateway, "updater_unavailable"
	}
	status, code := http.StatusServiceUnavailable, "updater_unavailable"
	if errors.Is(err, context.DeadlineExceeded) {
		status, code = http.StatusGatewayTimeout, "updater_timeout"
	}
	var remote *update.HTTPError
	if errors.As(err, &remote) {
		switch remote.Status {
		case http.StatusBadRequest:
			status, code = http.StatusBadRequest, "update.invalid_request"
		case http.StatusNotFound:
			status, code = http.StatusNotFound, "update.not_found"
		case http.StatusConflict:
			status, code = http.StatusConflict, "update.conflict"
		default:
			status, code = http.StatusBadGateway, "updater_unavailable"
		}
	}
	return status, code
}

func (h *Handler) UpdateStatus(c *gin.Context) {
	ctx, cancel, ok := h.updateContext(c)
	if !ok {
		return
	}
	defer cancel()
	status, err := h.updater.Status(ctx)
	if err != nil {
		updateError(c, err)
		return
	}
	response.Success(c, status)
}
func (h *Handler) CheckUpdate(c *gin.Context) {
	ctx, cancel, ok := h.updateContext(c)
	if !ok {
		return
	}
	defer cancel()
	status, err := h.updater.Check(ctx)
	if err != nil {
		updateError(c, err)
		return
	}
	response.Success(c, status)
}
func (h *Handler) InstallUpdate(c *gin.Context) {
	key := c.GetHeader("Idempotency-Key")
	if !idempotencyKeyPattern.MatchString(key) {
		response.ErrorWithCode(c, http.StatusBadRequest, "invalid_idempotency_key", "invalid idempotency key")
		return
	}
	var req updateInstallRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.InvalidRequestBody(c, err)
		return
	}
	if !regexp.MustCompile(`^sha256:[0-9a-f]{64}$`).MatchString(req.ManifestDigest) || req.Confirmation != "install "+req.Version+" "+req.ManifestDigest {
		response.ErrorWithCode(c, http.StatusBadRequest, "invalid_update_request", "invalid update request")
		return
	}
	ctx, cancel, ok := h.updateContext(c)
	if !ok {
		return
	}
	defer cancel()
	job, err := h.updater.Install(ctx, update.InstallRequest{Version: req.Version, ManifestDigest: req.ManifestDigest, Confirmation: req.Confirmation, IdempotencyKey: key, ActorUserID: middleware.MustUserID(c), ActorUsername: middleware.MustUsername(c), RequestID: middleware.MustRequestID(c)})
	if err != nil {
		updateError(c, err)
		return
	}
	response.Success(c, job)
}
func (h *Handler) UpdateJob(c *gin.Context) {
	id := c.Param("job_id")
	if !regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`).MatchString(id) {
		response.ErrorWithCode(c, http.StatusBadRequest, "invalid_job_id", "invalid job id")
		return
	}
	ctx, cancel, ok := h.updateContext(c)
	if !ok {
		return
	}
	defer cancel()
	job, err := h.updater.Job(ctx, id)
	if err != nil {
		updateError(c, err)
		return
	}
	response.Success(c, job)
}
