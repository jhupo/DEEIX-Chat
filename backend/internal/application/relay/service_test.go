package relay

import "testing"

func TestNormalizeConnectorRejectsInvalidModelOrigin(t *testing.T) {
	_, err := normalizeConnector(ConnectorInput{
		Name: "relay", Protocol: "sub2api", AccountBaseURL: "https://account.example.com",
		ModelBaseURL: "https://model.example.com/v1",
	})
	if err != ErrInvalidConnector {
		t.Fatalf("normalizeConnector() error = %v, want %v", err, ErrInvalidConnector)
	}
}

func TestNormalizeConnectorDefaultsBlankModelOrigin(t *testing.T) {
	item, err := normalizeConnector(ConnectorInput{
		Name: "relay", Protocol: "sub2api", AccountBaseURL: "https://account.example.com",
	})
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
	})
	if err != ErrInvalidConnector {
		t.Fatalf("normalizeConnector() error = %v, want %v", err, ErrInvalidConnector)
	}
}

func TestNormalizeHostnameRejectsURLSyntax(t *testing.T) {
	for _, value := range []string{"https://chat.example.com", "chat.example.com/path", "chat.example.com?x=1"} {
		if _, err := normalizeHostname(value); err != ErrInvalidRoute {
			t.Fatalf("normalizeHostname(%q) error = %v, want %v", value, err, ErrInvalidRoute)
		}
	}
}
