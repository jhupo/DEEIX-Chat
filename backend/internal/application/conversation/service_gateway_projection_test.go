package conversation

import (
	"testing"
	"time"

	model "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/conversation"
)

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
