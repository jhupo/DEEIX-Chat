package conversation

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	model "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/conversation"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
)

type steerConversationRepoStub struct {
	repository.ConversationRepository
	executionType string
}

func (s *steerConversationRepoStub) GetConversationExecutionByRunID(context.Context, uint, string) (*model.Conversation, error) {
	return &model.Conversation{ExecutionType: s.executionType}, nil
}

type steerGatewayExecutorStub struct {
	gatewayExecutor
	steerCalls, startCalls, interruptCalls int
	steerErr, interruptErr                 error
	steerInput                             []byte
}

func (s *steerGatewayExecutorStub) SteerRun(_ context.Context, _ uint, _, _ string, input []byte) error {
	s.steerCalls++
	s.steerInput = append([]byte(nil), input...)
	return s.steerErr
}

func (s *steerGatewayExecutorStub) StartTurn(context.Context, uint, GatewayStartTurnInput) error {
	s.startCalls++
	return nil
}

func (s *steerGatewayExecutorStub) InterruptRun(context.Context, uint, string, string) error {
	s.interruptCalls++
	return s.interruptErr
}

func TestSteerConversationRunUsesOnlyActiveGatewaySteer(t *testing.T) {
	gateway := &steerGatewayExecutorStub{}
	service := &Service{
		repo: &steerConversationRepoStub{executionType: model.ExecutionTypeGateway}, gatewayExecutor: gateway,
	}
	if err := service.SteerConversationRun(context.Background(), 7, "run_active", "change direction", "idem-1"); err != nil {
		t.Fatal(err)
	}
	if gateway.steerCalls != 1 || gateway.startCalls != 0 || gateway.interruptCalls != 0 ||
		string(gateway.steerInput) != `[{"kind":"text","text":"change direction"}]` {
		t.Fatalf("gateway calls steer=%d start=%d interrupt=%d input=%s", gateway.steerCalls, gateway.startCalls, gateway.interruptCalls, gateway.steerInput)
	}

	nonGateway := &steerGatewayExecutorStub{}
	cloudService := &Service{
		repo: &steerConversationRepoStub{executionType: model.ExecutionTypeCloud}, gatewayExecutor: nonGateway,
	}
	if err := cloudService.SteerConversationRun(context.Background(), 7, "run_cloud", "guide", "idem-2"); !errors.Is(err, ErrExecutionConflict) || nonGateway.steerCalls != 0 {
		t.Fatalf("non-Gateway steer error=%v calls=%d", err, nonGateway.steerCalls)
	}

	terminal := &steerGatewayExecutorStub{steerErr: ErrExecutionConflict}
	terminalService := &Service{
		repo: &steerConversationRepoStub{executionType: model.ExecutionTypeGateway}, gatewayExecutor: terminal,
	}
	if err := terminalService.SteerConversationRun(context.Background(), 7, "run_done", "guide", "idem-3"); !errors.Is(err, ErrExecutionConflict) || terminal.steerCalls != 1 {
		t.Fatalf("terminal steer error=%v calls=%d", err, terminal.steerCalls)
	}
}

type gatewayProjectionRepoStub struct {
	repository.ConversationRepository
	applied bool
}

type cancelGatewayRepoStub struct {
	repository.ConversationRepository
	event model.ExecutionEvent
}

func (s *cancelGatewayRepoStub) GetConversationExecutionByRunID(context.Context, uint, string) (*model.Conversation, error) {
	return &model.Conversation{ID: 11, UserID: 7, ExecutionType: model.ExecutionTypeGateway}, nil
}

func (s *cancelGatewayRepoStub) ProjectExecutionEvent(_ context.Context, event *model.ExecutionEvent) (bool, error) {
	event.Seq = 1
	s.event = *event
	return true, nil
}

func TestCancelGatewayRunCommitsInterruptedStateWhenRemoteInterruptFails(t *testing.T) {
	repo := &cancelGatewayRepoStub{}
	gateway := &steerGatewayExecutorStub{interruptErr: errors.New("thread not found")}
	registry := newGenerationStreamRegistry(newTestGenerationStreamStore(), defaultGenerationStreamOptions())
	registry.register(context.Background(), "run_cancel", 7, "conv_cancel", func() {})
	service := &Service{repo: repo, gatewayExecutor: gateway, generationStreams: registry}

	if !service.CancelMessageGeneration(context.Background(), 7, "run_cancel") {
		t.Fatal("gateway cancellation was rejected after the remote thread disappeared")
	}
	if gateway.interruptCalls != 1 || repo.event.TerminalStatus != "interrupted" || repo.event.Kind != "turn/completed" {
		t.Fatalf("gateway cancellation = calls %d event %#v", gateway.interruptCalls, repo.event)
	}
	if service.HasActiveMessageGeneration(context.Background(), "run_cancel") {
		t.Fatal("interrupted generation retained an active lease")
	}
}

func (s *gatewayProjectionRepoStub) ProjectExecutionEvent(_ context.Context, _ *model.ExecutionEvent) (bool, error) {
	return s.applied, nil
}

func TestNormalizeGatewayExecutionEvent(t *testing.T) {
	now := time.Now().UTC()
	delta, err := normalizeGatewayExecutionEvent(GatewayExecutionEvent{
		SourceKey: "agent:agev_1", UserID: 7, ConversationID: 11, RunID: "run_1",
		Kind: "item/agentMessage/delta", Payload: []byte(`{"delta":"hello"}`), OccurredAt: now,
	})
	if err != nil || delta.TextDelta != "hello" || delta.TerminalStatus != "" {
		t.Fatalf("delta projection = %#v, %v", delta, err)
	}
	commentary, err := normalizeGatewayExecutionEvent(GatewayExecutionEvent{
		SourceKey: "agent:agev_commentary", UserID: 7, ConversationID: 11, RunID: "run_1",
		Kind: "item/agentMessage/delta", Payload: []byte(`{"delta":"checking files","phase":"commentary"}`), OccurredAt: now,
	})
	if err != nil || commentary.TextDelta != "" || gatewayStreamPayload(commentary)["type"] != "execution_event" {
		t.Fatalf("commentary projection = %#v, %v", commentary, err)
	}
	usage, err := normalizeGatewayExecutionEvent(GatewayExecutionEvent{
		SourceKey: "agent:agev_usage", UserID: 7, ConversationID: 11, RunID: "run_1",
		Kind:    "thread/tokenUsage/updated",
		Payload: []byte(`{"tokenUsage":{"last":{"inputTokens":120,"cachedInputTokens":80,"outputTokens":15,"reasoningTokens":4},"total":{"inputTokens":500}}}`), OccurredAt: now,
	})
	if err != nil || !usage.HasUsage || usage.InputTokens != 120 || usage.CacheReadTokens != 80 || usage.OutputTokens != 15 || usage.ReasoningTokens != 4 {
		t.Fatalf("usage projection = %#v, %v", usage, err)
	}

	terminal, err := normalizeGatewayExecutionEvent(GatewayExecutionEvent{
		SourceKey: "agent:agev_2", UserID: 7, ConversationID: 11, RunID: "run_1",
		Kind: "turn/completed", Payload: []byte(`{"turn":{"status":"failed","durationMs":42,"error":{"code":"conversation.thread_in_use","message":"boom"}}}`), OccurredAt: now,
	})
	if err != nil || terminal.TerminalStatus != "failed" || terminal.ErrorCode != "conversation.thread_in_use" || terminal.ErrorMessage != "boom" || terminal.LatencyMS != 42 {
		t.Fatalf("terminal projection = %#v, %v", terminal, err)
	}
}

func TestGatewayEventUsageRejectsInvalidCounts(t *testing.T) {
	hasUsage, input, output, cacheRead, reasoning := gatewayEventUsage(map[string]interface{}{
		"tokenUsage": map[string]interface{}{"last": map[string]interface{}{
			"inputTokens": -1.0, "outputTokens": 1.5, "cachedInputTokens": "10", "reasoningTokens": math.Inf(1),
		}},
	})
	if hasUsage || input != 0 || output != 0 || cacheRead != 0 || reasoning != 0 {
		t.Fatalf("invalid usage accepted: %v %d %d %d %d", hasUsage, input, output, cacheRead, reasoning)
	}
}

func TestGatewayReasoningSummaryUsesExecutionTimeline(t *testing.T) {
	summary, err := normalizeGatewayExecutionEvent(GatewayExecutionEvent{
		SourceKey: "agent:reasoning_1", UserID: 7, ConversationID: 11, RunID: "run_1",
		Kind:    "item/reasoning/summaryTextDelta",
		Payload: []byte(`{"delta":"Reviewed the repository","summaryIndex":2}`), OccurredAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("reasoning event normalization failed: %v", err)
	}
	payload := gatewayStreamPayload(summary)
	if summary.ReasoningDelta != "Reviewed the repository" || payload["type"] != "execution_event" || payload["kind"] != "item/reasoning/summaryTextDelta" {
		t.Fatalf("reasoning stream payload = %#v", payload)
	}
	raw, err := normalizeGatewayExecutionEvent(GatewayExecutionEvent{
		SourceKey: "agent:reasoning_raw", UserID: 7, ConversationID: 11, RunID: "run_1",
		Kind: "item/reasoning/textDelta", Payload: []byte(`{"delta":"raw thought","contentIndex":2}`), OccurredAt: time.Now().UTC(),
	})
	if err != nil || raw.ReasoningDelta != "" || gatewayStreamPayload(raw)["type"] != "execution_event" {
		t.Fatalf("raw reasoning projection = %#v, %v", raw, err)
	}

	boundary, err := normalizeGatewayExecutionEvent(GatewayExecutionEvent{
		SourceKey: "agent:reasoning_2", UserID: 7, ConversationID: 11, RunID: "run_1",
		Kind:    "item/reasoning/summaryPartAdded",
		Payload: []byte(`{"summaryIndex":1}`), OccurredAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("reasoning boundary normalization failed: %v", err)
	}
	boundaryPayload := gatewayStreamPayload(boundary)
	if boundary.ReasoningDelta != "\n\n" || boundaryPayload["type"] != "execution_event" ||
		boundaryPayload["kind"] != "item/reasoning/summaryPartAdded" {
		t.Fatalf("reasoning boundary payload = %#v", boundaryPayload)
	}
}

func TestProjectGatewayEventSkipsDuplicateLivePublication(t *testing.T) {
	service := &Service{repo: &gatewayProjectionRepoStub{applied: false}}
	err := service.ProjectGatewayEvent(context.Background(), GatewayExecutionEvent{
		SourceKey: "agent:duplicate", UserID: 7, ConversationID: 11, RunID: "run_1",
		Kind: "item/agentMessage/delta", Payload: []byte(`{"delta":"already projected"}`), OccurredAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("duplicate gateway event = %v", err)
	}
}

func TestGatewayTurnSettingsRejectsUnknownOptions(t *testing.T) {
	valid := map[string]interface{}{
		"reasoningEffort": "high", "approvalPolicy": "on-request",
		"approvalsReviewer": "auto_review", "sandboxPolicy": "workspace-write",
	}
	settings, err := gatewayTurnSettings("gpt-5.6-codex", valid)
	if err != nil || string(settings) != `{"approvalPolicy":"on-request","approvalsReviewer":"auto_review","model":"gpt-5.6-codex","reasoningEffort":"high","sandboxPolicy":"workspace-write"}` {
		t.Fatalf("settings = %s, %v", settings, err)
	}
	for _, invalid := range []map[string]interface{}{
		{"reasoningEffort": "high"},
		{"reasoningEffort": "high", "approvalPolicy": "never", "approvalsReviewer": "auto_review", "sandboxPolicy": "workspace-write"},
		{"reasoningEffort": "high", "approvalPolicy": "on-request", "approvalsReviewer": "user", "sandboxPolicy": "danger-full-access"},
		{"reasoningEffort": "high", "approvalPolicy": "on-request", "approvalsReviewer": "user", "temperature": 0.5},
	} {
		if _, err := gatewayTurnSettings("gpt-5.6-codex", invalid); err == nil {
			t.Fatalf("invalid gateway settings accepted: %#v", invalid)
		}
	}
}

type cloudProjectionRepoStub struct {
	repository.ConversationRepository
	events []model.ExecutionEvent
}

func (s *cloudProjectionRepoStub) ProjectExecutionEvent(_ context.Context, event *model.ExecutionEvent) (bool, error) {
	event.Seq = uint64(len(s.events) + 1)
	s.events = append(s.events, *event)
	return true, nil
}

func TestCloudExecutionProjectionUsesCanonicalReasoningEvents(t *testing.T) {
	repo := &cloudProjectionRepoStub{}
	streamKinds := []string{}
	projection := newCloudExecutionProjection(
		&Service{repo: repo},
		context.Background(),
		SendMessageInput{ConversationID: 11, UserID: 7, ClientRunID: "run_cloud"},
		func(kind string, _ map[string]interface{}) error {
			streamKinds = append(streamKinds, kind)
			return nil
		},
	)
	if err := projection.start(); err != nil {
		t.Fatal(err)
	}
	if err := projection.handle(cloudReasoningSummaryEvent, map[string]interface{}{
		"eventID": "legacy_reasoning", "kind": "summary_text", "status": "streaming", "delta": "must be ignored",
	}); err != nil {
		t.Fatal(err)
	}
	if len(repo.events) != 1 {
		t.Fatalf("legacy reasoning payload was projected: %#v", repo.events)
	}
	if err := projection.handle(cloudReasoningSummaryEvent, map[string]interface{}{
		"itemID": "reasoning_1", "kind": "summary_text", "status": "streaming", "delta": "Checked the repository",
	}); err != nil {
		t.Fatal(err)
	}
	if err := projection.handle(cloudReasoningSummaryEvent, map[string]interface{}{
		"itemID": "reasoning_1", "kind": "content_text", "status": "streaming", "delta": "raw reasoning",
	}); err != nil {
		t.Fatal(err)
	}
	if err := projection.handle(cloudReasoningSummaryEvent, map[string]interface{}{
		"itemID": "reasoning_1", "kind": "summary_text", "status": "completed",
	}); err != nil {
		t.Fatal(err)
	}
	if err := projection.complete("completed", nil, nil); err != nil {
		t.Fatal(err)
	}

	kinds := make([]string, 0, len(repo.events))
	for _, event := range repo.events {
		kinds = append(kinds, event.Kind)
		if strings.Contains(event.PayloadJSON, "raw reasoning") {
			t.Fatalf("raw reasoning leaked into execution event: %s", event.PayloadJSON)
		}
	}
	want := []string{
		"turn/started", "item/started", "item/reasoning/summaryTextDelta", "item/completed", "turn/completed",
	}
	if strings.Join(kinds, ",") != strings.Join(want, ",") {
		t.Fatalf("cloud event kinds = %v, want %v", kinds, want)
	}
	for _, kind := range streamKinds {
		if kind != "execution_event" {
			t.Fatalf("cloud stream kind = %q", kind)
		}
	}
}

func TestCloudExecutionProjectionInterruptsOpenTool(t *testing.T) {
	repo := &cloudProjectionRepoStub{}
	projection := newCloudExecutionProjection(
		&Service{repo: repo},
		context.Background(),
		SendMessageInput{ConversationID: 11, UserID: 7, ClientRunID: "run_cloud"},
		nil,
	)
	if err := projection.start(); err != nil {
		t.Fatal(err)
	}
	if err := projection.handle(cloudToolActivityEvent, map[string]interface{}{
		"tools": []model.ToolCall{{
			ToolCallID: "tool_1", ToolName: "web_search", ToolType: "builtin", Status: "running",
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := projection.complete("interrupted", errors.New("generation canceled"), nil); err != nil {
		t.Fatal(err)
	}

	kinds := make([]string, 0, len(repo.events))
	for _, event := range repo.events {
		kinds = append(kinds, event.Kind)
	}
	want := []string{"turn/started", "item/started", "item/completed", "turn/completed"}
	if strings.Join(kinds, ",") != strings.Join(want, ",") {
		t.Fatalf("cloud event kinds = %v, want %v", kinds, want)
	}
	var completed struct {
		ItemID string `json:"itemID"`
		Item   struct {
			Kind   string `json:"kind"`
			Status string `json:"status"`
			Tool   string `json:"tool"`
		} `json:"item"`
	}
	if err := json.Unmarshal([]byte(repo.events[2].PayloadJSON), &completed); err != nil {
		t.Fatal(err)
	}
	if completed.ItemID != "tool_1" || completed.Item.Kind != "dynamicToolCall" ||
		completed.Item.Status != "interrupted" || completed.Item.Tool != "web_search" {
		t.Fatalf("completed cloud tool = %#v", completed)
	}
}

func TestCompactExecutionHistoryMergesIncrementalText(t *testing.T) {
	events := []model.ExecutionEvent{
		{RunID: "run_test", Seq: 1, Kind: "turn/started", PayloadJSON: `{"turn":{"status":"running"}}`},
		{RunID: "run_test", Seq: 2, Kind: "item/started", PayloadJSON: `{"itemID":"command_1","item":{"itemID":"command_1","kind":"commandExecution","command":"go test"}}`},
	}
	for seq := uint64(3); seq < 103; seq++ {
		events = append(events, model.ExecutionEvent{RunID: "run_test", Seq: seq, Kind: "item/commandExecution/outputDelta", PayloadJSON: `{"itemID":"command_1","outputDelta":"x"}`})
	}
	events = append(events,
		model.ExecutionEvent{RunID: "run_test", Seq: 103, Kind: "item/completed", PayloadJSON: `{"itemID":"command_1","item":{"itemID":"command_1","kind":"commandExecution","status":"completed"}}`},
		model.ExecutionEvent{RunID: "run_test", Seq: 104, Kind: "item/started", PayloadJSON: `{"itemID":"reasoning_1","item":{"itemID":"reasoning_1","kind":"reasoning","summary":[]}}`},
		model.ExecutionEvent{RunID: "run_test", Seq: 105, Kind: "item/reasoning/summaryTextDelta", PayloadJSON: `{"itemID":"reasoning_1","delta":"first"}`},
		model.ExecutionEvent{RunID: "run_test", Seq: 106, Kind: "item/reasoning/summaryPartAdded", PayloadJSON: `{"itemID":"reasoning_1"}`},
		model.ExecutionEvent{RunID: "run_test", Seq: 107, Kind: "item/reasoning/summaryTextDelta", PayloadJSON: `{"itemID":"reasoning_1","delta":"second"}`},
		model.ExecutionEvent{RunID: "run_test", Seq: 108, Kind: "item/completed", PayloadJSON: `{"itemID":"reasoning_1","item":{"itemID":"reasoning_1","kind":"reasoning","status":"completed","summary":[]}}`},
		model.ExecutionEvent{RunID: "run_test", Seq: 109, Kind: "turn/completed", PayloadJSON: `{"turn":{"status":"completed"}}`},
	)

	compacted := compactExecutionHistory(events)
	if len(compacted) != 8 {
		t.Fatalf("compacted event count = %d, want 8", len(compacted))
	}
	var commandDelta, reasoningDelta struct {
		OutputDelta string `json:"outputDelta"`
		Delta       string `json:"delta"`
	}
	for _, event := range compacted {
		switch event.Kind {
		case "item/commandExecution/outputDelta":
			_ = json.Unmarshal([]byte(event.PayloadJSON), &commandDelta)
		case "item/reasoning/summaryTextDelta":
			_ = json.Unmarshal([]byte(event.PayloadJSON), &reasoningDelta)
		}
	}
	if commandDelta.OutputDelta != strings.Repeat("x", 100) || reasoningDelta.Delta != "first\n\nsecond" {
		t.Fatalf("compacted deltas = command %q reasoning %q", commandDelta.OutputDelta, reasoningDelta.Delta)
	}
}

func TestCompactExecutionHistoryKeepsLatestRunTokenUsageSnapshot(t *testing.T) {
	events := []model.ExecutionEvent{
		{RunID: "run_usage", Seq: 1, Kind: "thread/tokenUsage/updated", PayloadJSON: `{"tokenUsage":{"last":{"inputTokens":10,"cachedInputTokens":8,"outputTokens":2,"reasoningTokens":1,"totalTokens":12}}}`},
		{RunID: "run_usage", Seq: 2, Kind: "thread/tokenUsage/updated", PayloadJSON: `{"tokenUsage":{"last":{"inputTokens":20,"cachedInputTokens":16,"outputTokens":4,"reasoningTokens":2,"totalTokens":24}}}`},
	}
	compacted := compactExecutionHistory(events)
	if len(compacted) != 1 || compacted[0].Seq != 2 {
		t.Fatalf("compacted token events = %#v", compacted)
	}
	if compacted[0].PayloadJSON != events[1].PayloadJSON {
		t.Fatalf("compacted token usage = %s, want latest snapshot %s", compacted[0].PayloadJSON, events[1].PayloadJSON)
	}
}

func TestCompactExecutionHistoryDeduplicatesAndBoundsCompletedToolItems(t *testing.T) {
	largeResult := strings.Repeat("界", maxExecutionHistoryToolFieldBytes)
	completedPayload := func(status, result string) string {
		encoded, err := json.Marshal(map[string]any{
			"itemID": "tool_1",
			"item": map[string]any{
				"itemID": "tool_1", "kind": "mcpToolCall", "status": status,
				"tool": "search", "result": result,
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		return string(encoded)
	}
	events := []model.ExecutionEvent{
		{RunID: "run_tool", Seq: 1, Kind: "item/completed", PayloadJSON: completedPayload("failed", "old")},
		{RunID: "run_tool", Seq: 2, Kind: "item/completed", PayloadJSON: completedPayload("completed", largeResult)},
	}

	compacted := compactExecutionHistory(events)
	if len(compacted) != 1 || compacted[0].Seq != 2 {
		t.Fatalf("compacted tool events = %#v", compacted)
	}
	var payload struct {
		Item struct {
			Status    string `json:"status"`
			Result    string `json:"result"`
			Truncated bool   `json:"truncated"`
		} `json:"item"`
	}
	if err := json.Unmarshal([]byte(compacted[0].PayloadJSON), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Item.Status != "completed" || !payload.Item.Truncated ||
		len(payload.Item.Result) > maxExecutionHistoryToolFieldBytes || !utf8.ValidString(payload.Item.Result) {
		t.Fatalf("compacted tool payload = %#v", payload.Item)
	}
}

type executionPageRepoStub struct {
	repository.ConversationRepository
	conversation model.Conversation
	events       []model.ExecutionEvent
	history      []model.ExecutionEvent
}

func (s *executionPageRepoStub) GetConversationByPublicID(context.Context, string, uint) (*model.Conversation, error) {
	result := s.conversation
	return &result, nil
}

func (s *executionPageRepoStub) ListExecutionEvents(context.Context, uint, uint, uint64, []string, int) ([]model.ExecutionEvent, error) {
	return append([]model.ExecutionEvent(nil), s.events...), nil
}

func (s *executionPageRepoStub) ListExecutionEventHistory(_ context.Context, _ uint, _ uint, _ []string) ([]model.ExecutionEvent, error) {
	return append([]model.ExecutionEvent(nil), s.history...), nil
}

func TestListExecutionEventsCompactsLivePageWithoutMovingCursorBackward(t *testing.T) {
	repo := &executionPageRepoStub{
		conversation: model.Conversation{ID: 11, UserID: 7, ExecutionEventSeq: 4},
		events: []model.ExecutionEvent{
			{RunID: "run_live", Seq: 2, Kind: "item/commandExecution/outputDelta", PayloadJSON: `{"itemID":"command_1","outputDelta":"first"}`},
			{RunID: "run_live", Seq: 3, Kind: "item/commandExecution/outputDelta", PayloadJSON: `{"itemID":"command_1","outputDelta":" second"}`},
			{RunID: "run_live", Seq: 4, Kind: "thread/tokenUsage/updated", PayloadJSON: `{"tokenUsage":{"totalTokens":12}}`},
		},
	}
	page, err := (&Service{repo: repo}).ListExecutionEvents(context.Background(), 7, "conversation_live", 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if page.Cursor != 4 || page.HasMore || len(page.Events) != 2 {
		t.Fatalf("live page = cursor %d hasMore %v events %#v", page.Cursor, page.HasMore, page.Events)
	}
	var delta struct {
		Output string `json:"outputDelta"`
	}
	if err := json.Unmarshal([]byte(page.Events[0].PayloadJSON), &delta); err != nil || delta.Output != "first second" {
		t.Fatalf("compacted live delta = %#v, %v", delta, err)
	}
}

func TestListExecutionEventsUsesCompleteInitialHistory(t *testing.T) {
	repo := &executionPageRepoStub{
		conversation: model.Conversation{ID: 11, UserID: 7, ExecutionEventSeq: 900},
		history: []model.ExecutionEvent{
			{RunID: "run_history", Seq: 899, Kind: "item/completed", PayloadJSON: `{"itemID":"command_1","item":{"kind":"commandExecution","status":"completed"}}`},
			{RunID: "run_history", Seq: 901, Kind: "turn/completed", PayloadJSON: `{"turn":{"status":"completed"}}`},
		},
	}
	page, err := (&Service{repo: repo}).ListExecutionEvents(context.Background(), 7, "conversation_history", 0, []string{"run_history"})
	if err != nil {
		t.Fatal(err)
	}
	if page.Cursor != 901 || page.HasMore || len(page.Events) != 2 {
		t.Fatalf("history page = cursor %d hasMore %v events %#v", page.Cursor, page.HasMore, page.Events)
	}
}
