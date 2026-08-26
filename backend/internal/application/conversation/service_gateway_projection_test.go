package conversation

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

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
	steerErr                               error
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
	return nil
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

	terminal, err := normalizeGatewayExecutionEvent(GatewayExecutionEvent{
		SourceKey: "agent:agev_2", UserID: 7, ConversationID: 11, RunID: "run_1",
		Kind: "turn/completed", Payload: []byte(`{"turn":{"status":"failed","durationMs":42,"error":{"message":"boom"}}}`), OccurredAt: now,
	})
	if err != nil || terminal.TerminalStatus != "failed" || terminal.ErrorMessage != "boom" || terminal.LatencyMS != 42 {
		t.Fatalf("terminal projection = %#v, %v", terminal, err)
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

func TestAttachReasoningSummaryTrace(t *testing.T) {
	updatedAt := time.Now().UTC()
	message := model.Message{
		Role: "assistant", Status: "success", ReasoningContent: "checked configuration", UpdatedAt: updatedAt,
	}
	attachReasoningSummaryTrace(&message)
	if message.ProcessTrace == nil || message.ProcessTrace.UpstreamThink == nil ||
		message.ProcessTrace.UpstreamThink.ContentMarkdown != "checked configuration" ||
		message.ProcessTrace.UpstreamThink.Status != messageTraceStatusCompleted ||
		!message.ProcessTrace.UpstreamThink.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("reasoning trace = %#v", message.ProcessTrace)
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

func TestDurableCloudEventSkipsStreamingDeltas(t *testing.T) {
	for _, kind := range []string{"upstream_think_delta", "output_text.delta", "usage", "process_update"} {
		if durableCloudEvent(kind) {
			t.Fatalf("%s should remain a live-only event", kind)
		}
	}
	for _, kind := range []string{"rag_search", "compact_done", "turn.completed"} {
		if !durableCloudEvent(kind) {
			t.Fatalf("%s should be durable", kind)
		}
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
