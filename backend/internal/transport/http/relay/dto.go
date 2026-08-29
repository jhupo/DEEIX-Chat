package relay

import (
	"time"

	apprelay "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/relay"
	domainrelay "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/relay"
)

type ConnectorRequest struct {
	Name           string `json:"name" binding:"required,max=128"`
	Protocol       string `json:"protocol" binding:"required,oneof=sub2api"`
	AccountBaseURL string `json:"accountBaseURL" binding:"required,max=512"`
	ModelBaseURL   string `json:"modelBaseURL,omitempty" binding:"omitempty,max=512"`
	ConfigJSON     string `json:"configJSON,omitempty" binding:"omitempty,max=65536"`
	Enabled        bool   `json:"enabled"`
}

type RouteRequest struct {
	Hostname    string `json:"hostname" binding:"required,max=255"`
	ConnectorID string `json:"connectorID" binding:"required,max=64"`
	Enabled     bool   `json:"enabled"`
}

type ConnectorResponse struct {
	ID             uint       `json:"id"`
	PublicID       string     `json:"publicID"`
	Name           string     `json:"name"`
	Protocol       string     `json:"protocol"`
	AccountBaseURL string     `json:"accountBaseURL"`
	ModelBaseURL   string     `json:"modelBaseURL"`
	Enabled        bool       `json:"enabled"`
	LastProbeAt    *time.Time `json:"lastProbeAt,omitempty"`
	LastProbeError string     `json:"lastProbeError,omitempty"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

type RouteResponse struct {
	ID          uint      `json:"id"`
	Hostname    string    `json:"hostname"`
	ConnectorID string    `json:"connectorID"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type ConnectorListResponseDoc struct {
	ErrorMsg string              `json:"errorMsg"`
	Data     []ConnectorResponse `json:"data"`
}

type ConnectorDataResponseDoc struct {
	ErrorMsg string            `json:"errorMsg"`
	Data     ConnectorResponse `json:"data"`
}

type RouteListResponseDoc struct {
	ErrorMsg string          `json:"errorMsg"`
	Data     []RouteResponse `json:"data"`
}

type RouteDataResponseDoc struct {
	ErrorMsg string        `json:"errorMsg"`
	Data     RouteResponse `json:"data"`
}

type RelayDeleteResponseDoc struct {
	ErrorMsg string `json:"errorMsg"`
	Data     struct {
		Deleted bool `json:"deleted"`
	} `json:"data"`
}

func toConnectorResponse(v domainrelay.Connector) ConnectorResponse {
	return ConnectorResponse{ID: v.ID, PublicID: v.PublicID, Name: v.Name, Protocol: v.Protocol, AccountBaseURL: v.AccountBaseURL, ModelBaseURL: v.ModelBaseURL, Enabled: v.Enabled, LastProbeAt: v.LastProbeAt, LastProbeError: v.LastProbeError, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt}
}
func toRouteResponse(v domainrelay.IngressRoute) RouteResponse {
	return RouteResponse{ID: v.ID, Hostname: v.Hostname, ConnectorID: v.ConnectorID, Enabled: v.Enabled, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt}
}
func toConnectorInput(v ConnectorRequest) apprelay.ConnectorInput {
	return apprelay.ConnectorInput{Name: v.Name, Protocol: v.Protocol, AccountBaseURL: v.AccountBaseURL, ModelBaseURL: v.ModelBaseURL, ConfigJSON: v.ConfigJSON, Enabled: v.Enabled}
}
