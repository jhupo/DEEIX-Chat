package conversation

import (
	"errors"
	"testing"

	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/config"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/llm"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/nativetool"
)

func TestCloudModelOptionsStripBrowserAndLegacyProviderTools(t *testing.T) {
	semanticOptions := cloneModelOptionMap(map[string]interface{}{
		"temperature": 0.4,
		"tools":       []interface{}{map[string]interface{}{"type": "web_search"}},
	})
	delete(semanticOptions, "tools")
	capabilities := modelCapabilitiesWithoutNativeTools(`{
		"nativeToolKeys":["openai.web_search"],
		"defaultOptions":{"tools":[{"type":"web_search"}],"text":{"verbosity":"low"}}
	}`)
	filtered := filterModelOptions(semanticOptions, llm.AdapterOpenAIResponses, modelOptionPolicyConfig{
		Mode:                  modelOptionPolicyAllowlist,
		AllowedPathsJSON:      config.DefaultModelOptionAllowedPathsJSON(),
		DeniedPathsJSON:       config.DefaultModelOptionDeniedPathsJSON(),
		ModelCapabilitiesJSON: capabilities,
	})
	if _, ok := filtered["tools"]; ok {
		t.Fatalf("expected provider tools to be stripped, got %#v", filtered)
	}
}

func TestWithConversationPluginToolsAddsOnlySelectedPlugin(t *testing.T) {
	options, err := withConversationPluginTools(
		map[string]interface{}{"temperature": 0.4},
		[]string{"plugin:web_search"},
		llm.AdapterOpenAIResponses,
		nativetool.DefaultConversationPluginKeysJSON,
	)
	if err != nil {
		t.Fatalf("resolve selected Plugin: %v", err)
	}
	tools, ok := options["tools"].([]map[string]interface{})
	if !ok || len(tools) != 1 || len(tools[0]) != 1 || tools[0]["type"] != "web_search" {
		t.Fatalf("expected only canonical web_search tool, got %#v", options["tools"])
	}
}

func TestWithConversationPluginToolsRejectsUnavailableAndUnsupported(t *testing.T) {
	if _, err := withConversationPluginTools(nil, []string{"plugin:image_generation"}, llm.AdapterOpenAIResponses, `["web_search"]`); !errors.Is(err, ErrConversationPluginUnavailable) {
		t.Fatalf("expected unavailable Plugin error, got %v", err)
	}
	if _, err := withConversationPluginTools(nil, []string{"plugin:image_generation"}, llm.AdapterAnthropicMessages, nativetool.DefaultConversationPluginKeysJSON); !errors.Is(err, ErrConversationPluginUnsupported) {
		t.Fatalf("expected unsupported route error, got %v", err)
	}
}

func TestWithConversationPluginToolsDoesNotAddToolsWithoutSelection(t *testing.T) {
	options, err := withConversationPluginTools(map[string]interface{}{"temperature": 0.2}, nil, llm.AdapterOpenAIResponses, nativetool.DefaultConversationPluginKeysJSON)
	if err != nil {
		t.Fatalf("resolve empty Plugin selection: %v", err)
	}
	if _, ok := options["tools"]; ok {
		t.Fatalf("expected no provider tools without a Plugin selection, got %#v", options)
	}
}
