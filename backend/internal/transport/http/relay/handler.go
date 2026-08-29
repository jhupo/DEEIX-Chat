package relay

import (
	"errors"
	"net/http"
	"strconv"

	apprelay "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/relay"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/response"
	"github.com/gin-gonic/gin"
)

type Handler struct{ service *apprelay.Service }

func NewHandler(service *apprelay.Service) *Handler { return &Handler{service: service} }

// ListConnectors godoc
// @Summary List relay connectors
// @Description Lists database-backed relay protocol connectors.
// @Tags admin-relays
// @Produce json
// @Security BearerAuth
// @Success 200 {object} ConnectorListResponseDoc
// @Failure 500 {object} response.SuccessDoc
// @Router /admin/relays/connectors [get]
func (h *Handler) ListConnectors(c *gin.Context) {
	items, err := h.service.ListConnectors(c.Request.Context())
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "list relay connectors failed")
		return
	}
	out := make([]ConnectorResponse, 0, len(items))
	for _, item := range items {
		out = append(out, toConnectorResponse(item))
	}
	response.Success(c, out)
}

// CreateConnector godoc
// @Summary Create a relay connector
// @Tags admin-relays
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body ConnectorRequest true "Relay connector"
// @Success 200 {object} ConnectorDataResponseDoc
// @Failure 400 {object} response.SuccessDoc
// @Failure 409 {object} response.SuccessDoc
// @Failure 500 {object} response.SuccessDoc
// @Router /admin/relays/connectors [post]
func (h *Handler) CreateConnector(c *gin.Context) {
	var req ConnectorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.InvalidRequestBody(c, err)
		return
	}
	item, err := h.service.CreateConnector(c.Request.Context(), toConnectorInput(req))
	if err != nil {
		writeRelayError(c, err)
		return
	}
	response.Success(c, toConnectorResponse(*item))
}

// UpdateConnector godoc
// @Summary Update a relay connector
// @Tags admin-relays
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Connector public ID"
// @Param body body ConnectorRequest true "Relay connector"
// @Success 200 {object} ConnectorDataResponseDoc
// @Failure 400 {object} response.SuccessDoc
// @Failure 404 {object} response.SuccessDoc
// @Failure 500 {object} response.SuccessDoc
// @Router /admin/relays/connectors/{id} [patch]
func (h *Handler) UpdateConnector(c *gin.Context) {
	var req ConnectorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.InvalidRequestBody(c, err)
		return
	}
	item, err := h.service.UpdateConnector(c.Request.Context(), c.Param("id"), toConnectorInput(req))
	if err != nil {
		writeRelayError(c, err)
		return
	}
	response.Success(c, toConnectorResponse(*item))
}

// DeleteConnector godoc
// @Summary Delete a relay connector
// @Description Deletes a connector that is not referenced by an inbound hostname route.
// @Tags admin-relays
// @Produce json
// @Security BearerAuth
// @Param id path string true "Connector public ID"
// @Success 200 {object} RelayDeleteResponseDoc
// @Failure 404 {object} response.SuccessDoc
// @Failure 409 {object} response.SuccessDoc
// @Failure 500 {object} response.SuccessDoc
// @Router /admin/relays/connectors/{id} [delete]
func (h *Handler) DeleteConnector(c *gin.Context) {
	if err := h.service.DeleteConnector(c.Request.Context(), c.Param("id")); err != nil {
		writeRelayError(c, err)
		return
	}
	response.Success(c, gin.H{"deleted": true})
}

// ListRoutes godoc
// @Summary List relay hostname routes
// @Tags admin-relays
// @Produce json
// @Security BearerAuth
// @Success 200 {object} RouteListResponseDoc
// @Failure 500 {object} response.SuccessDoc
// @Router /admin/relays/routes [get]
func (h *Handler) ListRoutes(c *gin.Context) {
	items, err := h.service.ListIngressRoutes(c.Request.Context())
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "list relay routes failed")
		return
	}
	out := make([]RouteResponse, 0, len(items))
	for _, item := range items {
		out = append(out, toRouteResponse(item))
	}
	response.Success(c, out)
}

// CreateRoute godoc
// @Summary Create a relay hostname route
// @Tags admin-relays
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body RouteRequest true "Inbound hostname route"
// @Success 200 {object} RouteDataResponseDoc
// @Failure 400 {object} response.SuccessDoc
// @Failure 404 {object} response.SuccessDoc
// @Failure 409 {object} response.SuccessDoc
// @Failure 500 {object} response.SuccessDoc
// @Router /admin/relays/routes [post]
func (h *Handler) CreateRoute(c *gin.Context) {
	var req RouteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.InvalidRequestBody(c, err)
		return
	}
	item, err := h.service.CreateIngressRoute(c.Request.Context(), apprelay.RouteInput{Hostname: req.Hostname, ConnectorID: req.ConnectorID, Enabled: req.Enabled})
	if err != nil {
		writeRelayError(c, err)
		return
	}
	response.Success(c, toRouteResponse(*item))
}

// UpdateRoute godoc
// @Summary Update a relay hostname route
// @Tags admin-relays
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "Route ID"
// @Param body body RouteRequest true "Inbound hostname route"
// @Success 200 {object} RouteDataResponseDoc
// @Failure 400 {object} response.SuccessDoc
// @Failure 404 {object} response.SuccessDoc
// @Failure 409 {object} response.SuccessDoc
// @Failure 500 {object} response.SuccessDoc
// @Router /admin/relays/routes/{id} [patch]
func (h *Handler) UpdateRoute(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		response.Error(c, http.StatusBadRequest, "invalid route id")
		return
	}
	var req RouteRequest
	if err = c.ShouldBindJSON(&req); err != nil {
		response.InvalidRequestBody(c, err)
		return
	}
	item, err := h.service.UpdateIngressRoute(c.Request.Context(), uint(id), apprelay.RouteInput{Hostname: req.Hostname, ConnectorID: req.ConnectorID, Enabled: req.Enabled})
	if err != nil {
		writeRelayError(c, err)
		return
	}
	response.Success(c, toRouteResponse(*item))
}

// DeleteRoute godoc
// @Summary Delete a relay hostname route
// @Tags admin-relays
// @Produce json
// @Security BearerAuth
// @Param id path int true "Route ID"
// @Success 200 {object} RelayDeleteResponseDoc
// @Failure 400 {object} response.SuccessDoc
// @Failure 404 {object} response.SuccessDoc
// @Failure 500 {object} response.SuccessDoc
// @Router /admin/relays/routes/{id} [delete]
func (h *Handler) DeleteRoute(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		response.Error(c, http.StatusBadRequest, "invalid route id")
		return
	}
	if err = h.service.DeleteIngressRoute(c.Request.Context(), uint(id)); err != nil {
		writeRelayError(c, err)
		return
	}
	response.Success(c, gin.H{"deleted": true})
}
func writeRelayError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, apprelay.ErrResourceNotFound):
		response.Error(c, http.StatusNotFound, err.Error())
	case errors.Is(err, apprelay.ErrResourceConflict):
		response.Error(c, http.StatusConflict, err.Error())
	case errors.Is(err, apprelay.ErrInvalidConnector), errors.Is(err, apprelay.ErrInvalidRoute):
		response.Error(c, http.StatusBadRequest, err.Error())
	default:
		response.Error(c, http.StatusInternalServerError, "relay operation failed")
	}
}
