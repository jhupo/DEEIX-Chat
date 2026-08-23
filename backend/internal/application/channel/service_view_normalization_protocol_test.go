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

func TestResolveModelDisplayCapabilitiesPrefersResponsesReasoningForDualProtocolModel(t *testing.T) {
	resolved := resolveModelDisplayCapabilitiesJSON(
		"gpt-5.6-sol",
		`["openai_chat_completions","openai_responses"]`,
		`{"defaultOptions":{"reasoning_effort":"high"},"optionControls":[{"path":"reasoning_effort","type":"select"}],"lockedOptionPaths":["reasoning_effort"]}`,
	)

	var capabilities map[string]interface{}
	if err := json.Unmarshal([]byte(resolved), &capabilities); err != nil {
		t.Fatalf("unmarshal capabilities: %v", err)
	}
	defaults := capabilities["defaultOptions"].(map[string]interface{})
	if _, exists := defaults["reasoning_effort"]; exists {
		t.Fatalf("did not expect duplicate Chat Completions reasoning default, got %#v", defaults)
	}
	reasoning := defaults["reasoning"].(map[string]interface{})
	if reasoning["effort"] != "medium" {
		t.Fatalf("expected Responses reasoning default, got %#v", defaults)
	}
	controls := capabilities["optionControls"].([]interface{})
	if len(controls) != 1 || controls[0].(map[string]interface{})["path"] != "reasoning.effort" {
		t.Fatalf("expected only the Responses reasoning control, got %#v", controls)
	}
	if _, exists := capabilities["lockedOptionPaths"]; exists {
		t.Fatalf("did not expect stale Chat Completions lock, got %#v", capabilities)
	}
}

func TestResolveModelDisplayCapabilitiesUsesFlatReasoningForChatCompletions(t *testing.T) {
	resolved := resolveModelDisplayCapabilitiesJSON(
		"gpt-5.6-sol",
		`["openai_chat_completions"]`,
		`{"defaultOptions":{"reasoning":{"effort":"high","summary":"auto"}},"optionControls":[{"path":"reasoning.effort","type":"select"}]}`,
	)

	var capabilities map[string]interface{}
	if err := json.Unmarshal([]byte(resolved), &capabilities); err != nil {
		t.Fatalf("unmarshal capabilities: %v", err)
	}
	defaults := capabilities["defaultOptions"].(map[string]interface{})
	if defaults["reasoning_effort"] != "medium" {
		t.Fatalf("expected Chat Completions reasoning default, got %#v", defaults)
	}
	reasoning := defaults["reasoning"].(map[string]interface{})
	if _, exists := reasoning["effort"]; exists || reasoning["summary"] != "auto" {
		t.Fatalf("expected only incompatible nested effort to be removed, got %#v", reasoning)
	}
	controls := capabilities["optionControls"].([]interface{})
	if len(controls) != 1 || controls[0].(map[string]interface{})["path"] != "reasoning_effort" {
		t.Fatalf("expected only the Chat Completions reasoning control, got %#v", controls)
	}
}

func TestResolveModelDisplayCapabilitiesLeavesUnknownModelsUntouched(t *testing.T) {
	raw := `{"nativeTools":[{"key":"openai.web_search"}]}`
	if resolved := resolveModelDisplayCapabilitiesJSON("gpt-4o", `["openai_responses"]`, raw); resolved != raw {
		t.Fatalf("expected unknown reasoning support to remain unmodified, got %s", resolved)
	}
}
