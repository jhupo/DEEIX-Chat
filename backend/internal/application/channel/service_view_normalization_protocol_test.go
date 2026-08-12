package channel

import "testing"

func TestResolveModelProtocolsJSONFallsBackToVendorWithoutRoutes(t *testing.T) {
	tests := []struct {
		name     string
		routes   string
		vendor   string
		expected string
	}{
		{name: "openai", routes: "[]", vendor: "openai", expected: `["openai_chat_completions","openai_responses"]`},
		{name: "anthropic", routes: "", vendor: "anthropic", expected: `["anthropic_messages"]`},
		{name: "explicit route", routes: `["openai_responses"]`, vendor: "openai", expected: `["openai_responses"]`},
		{name: "unknown", routes: "[]", vendor: "unknown", expected: `[]`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if actual := resolveModelProtocolsJSON(test.routes, test.vendor); actual != test.expected {
				t.Fatalf("expected %s, got %s", test.expected, actual)
			}
		})
	}
}
