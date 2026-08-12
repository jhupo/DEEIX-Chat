package conversation

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/channel"
	appsub2key "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/sub2key"
	model "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/conversation"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/config"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/llm"
)

type textTaskRouteResolverStub struct {
	routes       map[string]*channel.ResolvedRoute
	defaultRoute *channel.ResolvedRoute
	fail         map[string]error
	chatModel    *channel.ModelView
	chatModelErr error
}

func (r *textTaskRouteResolverStub) ResolveRoute(_ context.Context, input channel.ResolveRouteInput) (*channel.ResolvedRoute, error) {
	if err := r.fail[input.PlatformModelName]; err != nil {
		return nil, err
	}
	route := r.routes[input.PlatformModelName]
	if route == nil {
		return nil, errors.New("route not found")
	}
	return route, nil
}

func (r *textTaskRouteResolverStub) ResolveChatModel(_ context.Context, _ uint, name string) (*channel.ModelView, error) {
	if r.chatModelErr != nil {
		return nil, r.chatModelErr
	}
	if r.chatModel != nil {
		model := *r.chatModel
		return &model, nil
	}
	if strings.TrimSpace(name) == "" {
		return nil, channel.ErrModelNotFound
	}
	return &channel.ModelView{PlatformModelName: name}, nil
}

type sub2ExecutionResolverStub struct {
	calls     int
	execution *appsub2key.Execution
	err       error
}

func (r *sub2ExecutionResolverStub) ResolveBinding(context.Context, uint, string) (*appsub2key.Execution, error) {
	r.calls++
	return r.execution, r.err
}

func TestResolveSub2ChatRouteChecksCatalogBeforeBinding(t *testing.T) {
	binding := &sub2ExecutionResolverStub{execution: &appsub2key.Execution{APIKey: "secret"}}
	service := &Service{
		cfg:           config.NewRuntime(config.Config{Sub2BaseURL: "https://sub2.example.test"}),
		routeResolver: &textTaskRouteResolverStub{chatModelErr: channel.ErrModelAccessDenied},
		sub2Resolver:  binding,
	}

	if _, _, err := service.resolveSub2ChatRoute(t.Context(), 7, "admin-only-model", "sub2_binding"); !errors.Is(err, ErrModelAccessDenied) {
		t.Fatalf("resolveSub2ChatRoute() error = %v", err)
	}
	if binding.calls != 0 {
		t.Fatalf("binding resolved before model authorization: %d calls", binding.calls)
	}
}

func TestResolveSub2ChatRoutePinsAdministratorCatalogIdentity(t *testing.T) {
	binding := &sub2ExecutionResolverStub{execution: &appsub2key.Execution{
		BindingPublicID: "sub2_0123456789abcdef0123456789abcdef",
		BindingVersion:  3,
		RemoteKeyID:     42,
		APIKey:          "secret",
		GroupPlatform:   "anthropic",
	}}
	service := &Service{
		cfg: config.NewRuntime(config.Config{Sub2BaseURL: "https://sub2.example.test"}),
		routeResolver: &textTaskRouteResolverStub{chatModel: &channel.ModelView{
			ID:                9,
			PlatformModelName: "catalog-model",
			Vendor:            "anthropic",
			Icon:              "anthropic",
			KindsJSON:         `["chat"]`,
			ProtocolsJSON:     `["anthropic_messages"]`,
		}},
		sub2Resolver: binding,
	}

	route, execution, err := service.resolveSub2ChatRoute(t.Context(), 7, "catalog-model", binding.execution.BindingPublicID)
	if err != nil {
		t.Fatal(err)
	}
	run := &model.Run{}
	pinSub2ChatRouteToRun(run, route, execution)
	if route.PlatformModelID != 9 || route.PlatformModelName != "catalog-model" || route.UpstreamModel != "catalog-model" {
		t.Fatalf("route did not use administrator catalog identity: %#v", route)
	}
	if route.Protocol != llm.AdapterAnthropicMessages {
		t.Fatalf("route protocol = %q, want Anthropic Messages", route.Protocol)
	}
	if run.PlatformModelName != "catalog-model" || run.RoutedBindingCode != binding.execution.BindingPublicID || run.KeyBindingVersion != 3 || run.RemoteKeyID != 42 {
		t.Fatalf("run did not pin catalog route and binding: %#v", run)
	}
}

func TestResolveSub2ChatRouteRejectsNonChatModel(t *testing.T) {
	binding := &sub2ExecutionResolverStub{execution: &appsub2key.Execution{
		BindingPublicID: "sub2_0123456789abcdef0123456789abcdef",
		APIKey:          "secret",
		GroupPlatform:   "anthropic",
	}}
	service := &Service{
		cfg: config.NewRuntime(config.Config{Sub2BaseURL: "https://sub2.example.test"}),
		routeResolver: &textTaskRouteResolverStub{chatModel: &channel.ModelView{
			PlatformModelName: "openai-only-model",
			KindsJSON:         `["image_gen"]`,
			ProtocolsJSON:     `["openai_responses"]`,
		}},
		sub2Resolver: binding,
	}

	if _, _, err := service.resolveSub2ChatRoute(t.Context(), 7, "openai-only-model", binding.execution.BindingPublicID); !errors.Is(err, ErrModelAccessDenied) {
		t.Fatalf("resolveSub2ChatRoute() error = %v, want ErrModelAccessDenied", err)
	}
}

func TestResolveSub2ChatRouteRejectsModelWithoutKeyGroupProtocol(t *testing.T) {
	binding := &sub2ExecutionResolverStub{execution: &appsub2key.Execution{
		BindingPublicID: "sub2_0123456789abcdef0123456789abcdef",
		APIKey:          "secret",
	}}
	service := &Service{
		cfg: config.NewRuntime(config.Config{Sub2BaseURL: "https://sub2.example.test"}),
		routeResolver: &textTaskRouteResolverStub{chatModel: &channel.ModelView{
			PlatformModelName: "messages-only-model",
			KindsJSON:         `["chat"]`,
			ProtocolsJSON:     `["openai_responses"]`,
		}},
		sub2Resolver: binding,
	}

	if _, _, err := service.resolveSub2ChatRoute(t.Context(), 7, "messages-only-model", binding.execution.BindingPublicID); !errors.Is(err, ErrModelAccessDenied) {
		t.Fatalf("resolveSub2ChatRoute() error = %v, want ErrModelAccessDenied", err)
	}
}

func TestResolveSub2ChatProtocolUsesAdministratorModelProtocolForKeyGroup(t *testing.T) {
	tests := []struct {
		name, platform, protocols, want string
	}{
		{name: "OpenAI prefers Responses", platform: "openai", protocols: `["openai_chat_completions","openai_responses"]`, want: llm.AdapterOpenAIResponses},
		{name: "OpenAI accepts Completions", platform: "openai", protocols: `["openai_chat_completions"]`, want: llm.AdapterOpenAIChatCompletions},
		{name: "Anthropic uses Messages", platform: "anthropic", protocols: `["anthropic_messages"]`, want: llm.AdapterAnthropicMessages},
		{name: "Grok uses native Responses", platform: "grok", protocols: `["xai_responses"]`, want: llm.AdapterXAIResponses},
		{name: "Gemini uses Generate Content", platform: "gemini", protocols: `["google_generate_content"]`, want: llm.AdapterGoogleGenerateContent},
		{name: "Composite accepts OpenRouter", platform: "composite", protocols: `["openrouter_responses"]`, want: llm.AdapterOpenRouterResponses},
		{name: "Composite prefers Responses", platform: "composite", protocols: `["anthropic_messages","openai_responses"]`, want: llm.AdapterOpenAIResponses},
		{name: "Missing group", platform: "", protocols: `["openai_responses"]`, want: ""},
		{name: "No overlap", platform: "anthropic", protocols: `["openai_responses"]`, want: ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := resolveSub2ChatProtocol(test.platform, test.protocols); got != test.want {
				t.Fatalf("resolveSub2ChatProtocol(%q, %s) = %q, want %q", test.platform, test.protocols, got, test.want)
			}
		})
	}
}

func (r *textTaskRouteResolverStub) ResolveDefaultRoute(context.Context, channel.ResolveRouteInput) (*channel.ResolvedRoute, error) {
	if r.defaultRoute == nil {
		return nil, errors.New("default route not found")
	}
	return r.defaultRoute, nil
}

func (r *textTaskRouteResolverStub) MarkRouteFailure(context.Context, *channel.ResolvedRoute, error) {
}

func (r *textTaskRouteResolverStub) MarkRouteSuccess(context.Context, *channel.ResolvedRoute) {}

func TestResolveTextTaskRouteCandidatesFollowUsesCurrentThenDefault(t *testing.T) {
	service := &Service{routeResolver: &textTaskRouteResolverStub{
		routes: map[string]*channel.ResolvedRoute{
			"grok-4.3": {PlatformModelName: "grok-4.3", BindingCode: "current", Protocol: "xai_responses", UpstreamModel: "grok-4.3"},
		},
		defaultRoute: &channel.ResolvedRoute{PlatformModelName: "gpt-5-mini", BindingCode: "default", Protocol: "openai_responses", UpstreamModel: "gpt-5-mini"},
	}}

	routes, err := service.resolveTextTaskRouteCandidates(context.Background(), textTaskFollowModel, "grok-4.3", 1, 2, "")
	if err != nil {
		t.Fatalf("resolve candidates: %v", err)
	}
	if len(routes) != 2 {
		t.Fatalf("expected current and default routes, got %#v", routes)
	}
	if routes[0].BindingCode != "current" || routes[1].BindingCode != "default" {
		t.Fatalf("unexpected route order: %#v", routes)
	}
}

func TestResolveTextTaskRouteCandidatesSpecifiedModelDoesNotAddDefault(t *testing.T) {
	service := &Service{routeResolver: &textTaskRouteResolverStub{
		routes: map[string]*channel.ResolvedRoute{
			"gpt-5-mini": {PlatformModelName: "gpt-5-mini", BindingCode: "specified", Protocol: "openai_responses", UpstreamModel: "gpt-5-mini"},
		},
		defaultRoute: &channel.ResolvedRoute{PlatformModelName: "fallback", BindingCode: "default", Protocol: "openai_responses", UpstreamModel: "fallback"},
	}}

	routes, err := service.resolveTextTaskRouteCandidates(context.Background(), "gpt-5-mini", "grok-4.3", 1, 2, "")
	if err != nil {
		t.Fatalf("resolve candidates: %v", err)
	}
	if len(routes) != 1 || routes[0].BindingCode != "specified" {
		t.Fatalf("expected only specified route, got %#v", routes)
	}
}

func TestResolveTextTaskRouteCandidatesFollowFallsBackWhenCurrentRouteFails(t *testing.T) {
	service := &Service{routeResolver: &textTaskRouteResolverStub{
		routes: map[string]*channel.ResolvedRoute{},
		fail: map[string]error{
			"grok-4.3": errors.New("current route unavailable"),
		},
		defaultRoute: &channel.ResolvedRoute{PlatformModelName: "gpt-5-mini", BindingCode: "default", Protocol: "openai_responses", UpstreamModel: "gpt-5-mini"},
	}}

	routes, err := service.resolveTextTaskRouteCandidates(context.Background(), textTaskFollowModel, "grok-4.3", 1, 2, "")
	if err != nil {
		t.Fatalf("resolve candidates: %v", err)
	}
	if len(routes) != 1 || routes[0].BindingCode != "default" {
		t.Fatalf("expected default route after current route failure, got %#v", routes)
	}
}

func TestBuildTextTaskGenerateInputAppliesDefaultsAndInstructions(t *testing.T) {
	route := &channel.ResolvedRoute{
		Protocol:              llm.AdapterOpenAIResponses,
		BaseURL:               "https://api.openai.com/v1",
		ModelCapabilitiesJSON: `{"defaultOptions":{"reasoning":{"effort":"medium"}}}`,
	}
	input := buildTextTaskGenerateInput(route, config.Config{
		ModelOptionPolicyMode: modelOptionPolicyAllowlist,
		ModelOptionAllowedPaths: `{
			"openai_responses": ["reasoning.effort"]
		}`,
		ModelOptionDeniedPaths: config.DefaultModelOptionDeniedPathsJSON(),
	}, []llm.Message{
		{Role: "system", Content: "summarize carefully"},
		{Role: "user", Content: "hello"},
	})

	if input.Instructions != "summarize carefully" {
		t.Fatalf("expected official Responses instructions, got %q", input.Instructions)
	}
	if len(input.Messages) != 1 || input.Messages[0].Role != "user" {
		t.Fatalf("expected system message to be removed from input, got %#v", input.Messages)
	}
	reasoning := input.Options["reasoning"].(map[string]interface{})
	if reasoning["effort"] != "medium" {
		t.Fatalf("expected default reasoning effort, got %#v", input.Options)
	}
}

func TestBuildTextTaskGenerateInputInlinesSystemWhenCapabilitiesDisableSystemPrompt(t *testing.T) {
	route := &channel.ResolvedRoute{
		Protocol:              llm.AdapterOpenAIResponses,
		BaseURL:               "https://api.openai.com/v1",
		ModelCapabilitiesJSON: `{"supportsSystemPrompt":false}`,
	}
	input := buildTextTaskGenerateInput(route, config.Config{}, []llm.Message{
		{Role: "system", Content: "title only"},
		{Role: "user", Content: "hello"},
	})

	if input.Instructions != "" {
		t.Fatalf("expected no native instructions for inline-user capability, got %q", input.Instructions)
	}
	if len(input.Messages) != 1 || input.Messages[0].Role != "user" {
		t.Fatalf("expected one inlined user message, got %#v", input.Messages)
	}
	content := input.Messages[0].Content
	if !strings.Contains(content, "<system_instructions>") || !strings.Contains(content, "title only") || !strings.Contains(content, "hello") {
		t.Fatalf("expected system prompt to be inlined into user message, got %q", content)
	}
}
