package conversation

import (
	"context"
	"errors"
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

	terminal, err := normalizeGatewayExecutionEvent(GatewayExecutionEvent{
		SourceKey: "agent:agev_2", UserID: 7, ConversationID: 11, RunID: "run_1",
		Kind: "turn/completed", Payload: []byte(`{"turn":{"status":"failed","durationMs":42,"error":{"message":"boom"}}}`), OccurredAt: now,
	})
	if err != nil || terminal.TerminalStatus != "failed" || terminal.ErrorMessage != "boom" || terminal.LatencyMS != 42 {
		t.Fatalf("terminal projection = %#v, %v", terminal, err)
	}
}

func TestGatewayReasoningStreamPayloadPreservesEventKindAndIndex(t *testing.T) {
	event, err := normalizeGatewayExecutionEvent(GatewayExecutionEvent{
		SourceKey: "agent:reasoning_1", UserID: 7, ConversationID: 11, RunID: "run_1",
		Kind:    "item/reasoning/textDelta",
		Payload: []byte(`{"delta":"raw thought","contentIndex":2}`), OccurredAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("reasoning event normalization failed: %v", err)
	}
	payload := gatewayStreamPayload(event)
	if payload["kind"] != "content_text" || payload["delta"] != "raw thought" || payload["contentIndex"] != float64(2) {
		t.Fatalf("reasoning stream payload = %#v", payload)
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
	if boundary.ReasoningDelta != "\n\n" || boundaryPayload["kind"] != "summary_part_added" ||
		boundaryPayload["summaryIndex"] != float64(1) || boundaryPayload["delta"] != nil {
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
