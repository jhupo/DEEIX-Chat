package relay

import (
	"context"
	"errors"
	"testing"

	domainrelay "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/relay"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
)

type relayRepositoryStub struct {
	repository.RelayRepository
	connector    *domainrelay.Connector
	updateCalled bool
}

func (s *relayRepositoryStub) GetConnector(context.Context, string) (*domainrelay.Connector, error) {
	if s.connector == nil {
		return nil, repository.ErrNotFound
	}
	return s.connector, nil
}

func (s *relayRepositoryStub) UpdateConnector(context.Context, string, repository.RelayConnectorInput) (*domainrelay.Connector, error) {
	s.updateCalled = true
	return s.connector, nil
}

func TestNormalizeConnectorRejectsInvalidModelOrigin(t *testing.T) {
	_, err := normalizeConnector(ConnectorInput{
		Name: "relay", Protocol: "sub2api", AccountBaseURL: "https://account.example.com",
		ModelBaseURL: "https://model.example.com/v1",
	}, true)
	if err != ErrInvalidConnector {
		t.Fatalf("normalizeConnector() error = %v, want %v", err, ErrInvalidConnector)
	}
}

func TestNormalizeConnectorDefaultsBlankModelOrigin(t *testing.T) {
	item, err := normalizeConnector(ConnectorInput{
		Name: "relay", Protocol: "sub2api", AccountBaseURL: "https://account.example.com",
	}, true)
	if err != nil {
		t.Fatalf("normalizeConnector() error = %v", err)
	}
	if item.ModelBaseURL != item.AccountBaseURL {
		t.Fatalf("model origin = %q, want account origin %q", item.ModelBaseURL, item.AccountBaseURL)
	}
}

func TestNormalizeConnectorRejectsUnsupportedProtocol(t *testing.T) {
	_, err := normalizeConnector(ConnectorInput{
		Name: "relay", Protocol: "newapi", AccountBaseURL: "https://account.example.com",
	}, true)
	if err != ErrInvalidConnector {
		t.Fatalf("normalizeConnector() error = %v, want %v", err, ErrInvalidConnector)
	}
}

func TestNormalizeConnectorRejectsHTTPInProduction(t *testing.T) {
	_, err := normalizeConnector(ConnectorInput{
		Name: "relay", Protocol: "sub2api", AccountBaseURL: "http://account.example.com",
	}, true)
	if err != ErrInvalidConnector {
		t.Fatalf("normalizeConnector() error = %v, want %v", err, ErrInvalidConnector)
	}
}

func TestNormalizeConnectorAllowsHTTPInDevelopment(t *testing.T) {
	item, err := normalizeConnector(ConnectorInput{
		Name: "relay", Protocol: "sub2api", AccountBaseURL: "http://127.0.0.1:8080",
	}, false)
	if err != nil || item.AccountBaseURL != "http://127.0.0.1:8080" {
		t.Fatalf("normalizeConnector() = %#v, %v", item, err)
	}
}

func TestUpdateConnectorRejectsIdentityOriginChanges(t *testing.T) {
	for _, test := range []struct {
		name             string
		existingProtocol string
		input            ConnectorInput
	}{
		{
			name:             "protocol",
			existingProtocol: "legacy",
			input: ConnectorInput{
				Name: "relay", Protocol: "sub2api", AccountBaseURL: "https://account.example.com",
			},
		},
		{
			name:             "account origin",
			existingProtocol: "sub2api",
			input: ConnectorInput{
				Name: "relay", Protocol: "sub2api", AccountBaseURL: "https://other.example.com",
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := &relayRepositoryStub{connector: &domainrelay.Connector{
				PublicID: "relay-1", Protocol: test.existingProtocol, AccountBaseURL: "https://account.example.com",
			}}
			_, err := NewService(repo, true).UpdateConnector(context.Background(), "relay-1", test.input)
			if !errors.Is(err, ErrResourceConflict) {
				t.Fatalf("UpdateConnector() error = %v, want %v", err, ErrResourceConflict)
			}
			if repo.updateCalled {
				t.Fatal("UpdateConnector() called repository update after identity change")
			}
		})
	}
}

func TestNormalizeHostnameRejectsURLSyntax(t *testing.T) {
	for _, value := range []string{"https://chat.example.com", "chat.example.com/path", "chat.example.com?x=1"} {
		if _, err := normalizeHostname(value); err != ErrInvalidRoute {
			t.Fatalf("normalizeHostname(%q) error = %v, want %v", value, err, ErrInvalidRoute)
		}
	}
}
