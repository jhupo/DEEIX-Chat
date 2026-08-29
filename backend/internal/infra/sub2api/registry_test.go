package sub2api

import (
	"context"
	"testing"

	domainrelay "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/relay"
	port "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/ports/sub2api"
	sharedsecurity "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/security"
)

type registryConnectorResolverStub struct {
	connector *domainrelay.Connector
}

func (s registryConnectorResolverStub) GetConnectorByHostname(context.Context, string) (*domainrelay.Connector, error) {
	return s.connector, nil
}

func TestRegistryResolvesConfiguredConnectorFromRequestHost(t *testing.T) {
	registry := NewRegistry(registryConnectorResolverStub{connector: &domainrelay.Connector{
		PublicID: "relay_public_id", Protocol: "sub2api", AccountBaseURL: "https://account.example.com", ModelBaseURL: "https://model.example.com", Enabled: true,
	}}, sharedsecurity.NewStrictOutboundPolicy(false))
	ctx := port.WithRequestHost(context.Background(), "chat.example.com")

	if got := registry.InstanceIDForContext(ctx); got != "relay_public_id" {
		t.Fatalf("InstanceIDForContext() = %q, want relay_public_id", got)
	}
	if got := registry.ModelBaseURL(ctx); got != "https://model.example.com" {
		t.Fatalf("ModelBaseURL() = %q, want model origin", got)
	}
}

func TestRegistryRequiresRequestHost(t *testing.T) {
	registry := NewRegistry(registryConnectorResolverStub{connector: &domainrelay.Connector{Enabled: true}}, sharedsecurity.NewStrictOutboundPolicy(false))
	if got := registry.InstanceIDForContext(context.Background()); got != "" {
		t.Fatalf("InstanceIDForContext() without host = %q, want empty", got)
	}
	if got := registry.ModelBaseURL(context.Background()); got != "" {
		t.Fatalf("ModelBaseURL() without host = %q, want empty", got)
	}
}
