package conversation

import (
	"context"
	"testing"
	"time"

	model "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/conversation"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
)

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
	if boundaryPayload["kind"] != "summary_part_added" || boundaryPayload["summaryIndex"] != float64(1) {
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
	settings, err := gatewayTurnSettings(map[string]interface{}{"reasoningEffort": "high"})
	if err != nil || string(settings) != `{"reasoningEffort":"high"}` {
		t.Fatalf("settings = %s, %v", settings, err)
	}
	if _, err := gatewayTurnSettings(map[string]interface{}{"temperature": 0.5}); err == nil {
		t.Fatal("unknown gateway option was accepted")
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
