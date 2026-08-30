package sub2api

import (
	"context"
	"fmt"
	"testing"

	domainrelay "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/relay"
	port "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/ports/sub2api"
	sharedsecurity "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/security"
)

type registryConnectorResolverStub struct {
	connector *domainrelay.Connector
}

type countingConnectorResolver struct {
	calls int
}

func (s *countingConnectorResolver) GetConnectorByHostname(context.Context, string) (*domainrelay.Connector, error) {
	s.calls++
	return &domainrelay.Connector{
		PublicID: fmt.Sprintf("relay-%d", s.calls), Protocol: "sub2api",
		AccountBaseURL: fmt.Sprintf("https://account-%d.example.com", s.calls),
		ModelBaseURL:   fmt.Sprintf("https://model-%d.example.com", s.calls), Enabled: true,
	}, nil
}

type hostnameConnectorResolver struct{}

func (hostnameConnectorResolver) GetConnectorByHostname(_ context.Context, host string) (*domainrelay.Connector, error) {
	return &domainrelay.Connector{
		PublicID: host, Protocol: "sub2api", AccountBaseURL: "https://" + host + ".example.com",
		ModelBaseURL: "https://" + host + ".example.com", Enabled: true,
	}, nil
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

func TestRegistryPinsConnectorForRequest(t *testing.T) {
	resolver := &countingConnectorResolver{}
	registry := NewRegistry(resolver, sharedsecurity.NewStrictOutboundPolicy(false))
	ctx := port.WithRequestHost(context.Background(), "chat.example.com")

	if got := registry.InstanceIDForContext(ctx); got != "relay-1" {
		t.Fatalf("InstanceIDForContext() = %q, want relay-1", got)
	}
	if got := registry.ModelBaseURL(ctx); got != "https://model-1.example.com" {
		t.Fatalf("ModelBaseURL() = %q, want pinned model origin", got)
	}
	if resolver.calls != 1 {
		t.Fatalf("connector resolver calls = %d, want 1", resolver.calls)
	}
}

func TestRegistryBoundsClientCache(t *testing.T) {
	registry := NewRegistry(hostnameConnectorResolver{}, sharedsecurity.NewStrictOutboundPolicy(false))
	for index := 0; index <= maxCachedClients; index++ {
		ctx := port.WithRequestHost(context.Background(), fmt.Sprintf("relay-%d", index))
		if _, err := registry.client(ctx); err != nil {
			t.Fatalf("client(%d) error = %v", index, err)
		}
	}
	if got := len(registry.cache); got != maxCachedClients {
		t.Fatalf("client cache size = %d, want %d", got, maxCachedClients)
	}
}
