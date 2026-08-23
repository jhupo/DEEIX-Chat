package nativetool

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	ConversationPluginRefPrefix       = "plugin:"
	DefaultConversationPluginKeysJSON = `["web_search","image_generation","code_interpreter"]`
)

// ConversationPlugin describes one trusted provider-native capability exposed in chat.
type ConversationPlugin struct {
	Key         string
	Label       string
	Description string
	Enabled     bool
	toolKeys    map[string]string
}

var conversationPlugins = []ConversationPlugin{
	{
		Key:         "web_search",
		Label:       "Web Search",
		Description: "Search the web for current information and sources.",
		toolKeys: map[string]string{
			"openai_chat_completions": "openai.web_search",
			"openai_responses":        "openai.web_search",
			"openrouter_responses":    "openai.web_search",
			"anthropic_messages":      "anthropic.web_search_20260209",
			"xai_responses":           "xai.web_search",
			"gemini_generate_content": "google.google_search",
		},
	},
	{
		Key:         "image_generation",
		Label:       "Image Generation",
		Description: "Create or edit images in the current conversation.",
		toolKeys: map[string]string{
			"openai_responses":     "openai.image_generation",
			"openrouter_responses": "openai.image_generation",
		},
	},
	{
		Key:         "code_interpreter",
		Label:       "Code Interpreter",
		Description: "Run code in a provider-managed sandbox.",
		toolKeys: map[string]string{
			"openai_responses":        "openai.code_interpreter",
			"openrouter_responses":    "openai.code_interpreter",
			"anthropic_messages":      "anthropic.code_execution_20260120",
			"xai_responses":           "xai.code_interpreter",
			"gemini_generate_content": "google.code_execution",
		},
	},
}

// ConversationPlugins returns the trusted catalog with the configured enabled state.
func ConversationPlugins(enabledKeysJSON string) ([]ConversationPlugin, error) {
	enabled, err := ParseConversationPluginKeysJSON(enabledKeysJSON)
	if err != nil {
		return nil, err
	}
	items := make([]ConversationPlugin, 0, len(conversationPlugins))
	for _, item := range conversationPlugins {
		clone := item
		clone.Enabled = enabled[item.Key]
		clone.toolKeys = cloneStringMap(item.toolKeys)
		items = append(items, clone)
	}
	return items, nil
}

// ParseConversationPluginKeysJSON validates the configured enabled plugin keys.
func ParseConversationPluginKeysJSON(raw string) (map[string]bool, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		value = DefaultConversationPluginKeysJSON
	}
	var keys []string
	if err := json.Unmarshal([]byte(value), &keys); err != nil {
		return nil, fmt.Errorf("conversation plugin keys must be a JSON array: %w", err)
	}
	known := make(map[string]struct{}, len(conversationPlugins))
	for _, item := range conversationPlugins {
		known[item.Key] = struct{}{}
	}
	result := make(map[string]bool, len(keys))
	for _, rawKey := range keys {
		key := strings.TrimSpace(rawKey)
		if _, ok := known[key]; !ok {
			return nil, fmt.Errorf("unsupported conversation plugin key: %s", key)
		}
		if result[key] {
			return nil, fmt.Errorf("duplicate conversation plugin key: %s", key)
		}
		result[key] = true
	}
	return result, nil
}

// ResolveConversationPluginRefs validates browser-provided semantic resource references.
func ResolveConversationPluginRefs(refs []string, enabledKeysJSON string) ([]string, error) {
	enabled, err := ParseConversationPluginKeysJSON(enabledKeysJSON)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(refs))
	seen := make(map[string]struct{}, len(refs))
	for _, rawRef := range refs {
		ref := strings.TrimSpace(rawRef)
		if !strings.HasPrefix(ref, ConversationPluginRefPrefix) {
			return nil, fmt.Errorf("invalid conversation plugin reference")
		}
		key := strings.TrimSpace(strings.TrimPrefix(ref, ConversationPluginRefPrefix))
		if !enabled[key] {
			return nil, fmt.Errorf("conversation plugin is unavailable: %s", key)
		}
		if _, duplicate := seen[key]; duplicate {
			return nil, fmt.Errorf("duplicate conversation plugin: %s", key)
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	return keys, nil
}

// ConversationPluginTool resolves one semantic plugin to a canonical provider tool.
func ConversationPluginTool(protocol string, pluginKey string) (Definition, map[string]interface{}, bool) {
	protocol = strings.TrimSpace(protocol)
	pluginKey = strings.TrimSpace(pluginKey)
	for _, plugin := range conversationPlugins {
		if plugin.Key != pluginKey {
			continue
		}
		toolKey := plugin.toolKeys[protocol]
		if toolKey == "" {
			return Definition{}, nil, false
		}
		for _, definition := range definitions {
			if definition.Key != toolKey {
				continue
			}
			if definition.Protocol != protocol && !(protocol == "openrouter_responses" && definition.Protocol == "openai_responses") {
				continue
			}
			return cloneDefinition(definition), cloneMap(definition.Payload), true
		}
	}
	return Definition{}, nil, false
}

func cloneStringMap(source map[string]string) map[string]string {
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}
