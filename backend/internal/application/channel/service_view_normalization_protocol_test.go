package channel

import (
	"encoding/json"
	"testing"
)

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

func TestResolveModelDisplayCapabilitiesAddsAdapterReasoningControls(t *testing.T) {
	raw := `{"nativeTools":[{"key":"openai.image_generation"}]}`
	resolved := resolveModelDisplayCapabilitiesJSON(
		"gpt-5.6-sol",
		`["openai_responses"]`,
		raw,
	)

	var capabilities map[string]interface{}
	if err := json.Unmarshal([]byte(resolved), &capabilities); err != nil {
		t.Fatalf("unmarshal capabilities: %v", err)
	}
	defaults := capabilities["defaultOptions"].(map[string]interface{})
	reasoning := defaults["reasoning"].(map[string]interface{})
	if reasoning["effort"] != "medium" {
		t.Fatalf("expected adapter reasoning default, got %#v", defaults)
	}
	controls := capabilities["optionControls"].([]interface{})
	if len(controls) != 1 || controls[0].(map[string]interface{})["path"] != "reasoning.effort" {
		t.Fatalf("expected Responses reasoning control, got %#v", controls)
	}
	if len(capabilities["nativeTools"].([]interface{})) != 1 {
		t.Fatalf("expected stored capabilities to be preserved, got %#v", capabilities)
	}
}

func TestResolveModelDisplayCapabilitiesLeavesUnknownModelsUntouched(t *testing.T) {
	raw := `{"nativeTools":[{"key":"openai.web_search"}]}`
	if resolved := resolveModelDisplayCapabilitiesJSON("gpt-4o", `["openai_responses"]`, raw); resolved != raw {
		t.Fatalf("expected unknown reasoning support to remain unmodified, got %s", resolved)
	}
}
