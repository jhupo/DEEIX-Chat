package nativetool

import "testing"

func TestResolveConversationPluginRefsRejectsUnknownAndDisabled(t *testing.T) {
	for _, refs := range [][]string{
		{"plugin:unknown"},
		{"plugin:image_generation"},
		{"skill:review"},
	} {
		if _, err := ResolveConversationPluginRefs(refs, `["web_search"]`); err == nil {
			t.Fatalf("expected refs %#v to be rejected", refs)
		}
	}
}

func TestConversationPluginToolResolvesCanonicalWebSearch(t *testing.T) {
	_, payload, ok := ConversationPluginTool("openai_responses", "web_search")
	if !ok {
		t.Fatal("expected OpenAI Responses web_search Plugin to be supported")
	}
	if len(payload) != 1 || payload["type"] != "web_search" {
		t.Fatalf("expected canonical web_search payload, got %#v", payload)
	}
}

func TestParseConversationPluginKeysRejectsDuplicate(t *testing.T) {
	if _, err := ParseConversationPluginKeysJSON(`["web_search","web_search"]`); err == nil {
		t.Fatal("expected duplicate plugin key to be rejected")
	}
}
