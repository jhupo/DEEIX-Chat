package repository

import (
	"context"

	domainrelay "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/relay"
)

type RelayConnectorInput struct {
	PublicID       string
	Name           string
	Protocol       string
	AccountBaseURL string
	ModelBaseURL   string
	ConfigJSON     string
	Enabled        bool
}

type RelayRepository interface {
	ListConnectors(context.Context) ([]domainrelay.Connector, error)
	GetConnector(context.Context, string) (*domainrelay.Connector, error)
	GetConnectorByHostname(context.Context, string) (*domainrelay.Connector, error)
	CreateConnector(context.Context, RelayConnectorInput) (*domainrelay.Connector, error)
	UpdateConnector(context.Context, string, RelayConnectorInput) (*domainrelay.Connector, error)
	DeleteConnector(context.Context, string) error
	ListIngressRoutes(context.Context) ([]domainrelay.IngressRoute, error)
	CreateIngressRoute(context.Context, string, string, bool) (*domainrelay.IngressRoute, error)
	UpdateIngressRoute(context.Context, uint, string, string, bool) (*domainrelay.IngressRoute, error)
	DeleteIngressRoute(context.Context, uint) error
}
