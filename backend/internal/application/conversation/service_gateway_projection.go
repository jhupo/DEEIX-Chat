package conversation

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	model "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/conversation"
)

func (s *Service) ProjectGatewayEvent(ctx context.Context, input GatewayExecutionEvent) error {
	event, err := normalizeGatewayExecutionEvent(input)
	if err != nil {
		return err
	}
	applied, err := s.repo.ProjectExecutionEvent(ctx, event)
	if err != nil {
		return err
	}
	// Repository projection is idempotent by SourceKey. Only the first
	// application may advance the live stream; replayed bridge events are
	// durable no-ops unless an active subscriber still needs its terminal.
	if !applied {
		if event.TerminalStatus != "" && s.HasActiveMessageGeneration(ctx, event.RunID) {
			if err := s.publishMessageGenerationEventReliable(event.RunID, map[string]interface{}{
				"type": "gateway_completed", "status": event.TerminalStatus,
			}); err != nil {
				return err
			}
			s.FinishMessageGeneration(event.RunID)
		}
		return nil
	}
	payload := gatewayStreamPayload(event)
	if payload != nil {
		if err := s.publishMessageGenerationEventReliable(event.RunID, payload); err != nil {
			return err
		}
	}
	if event.TerminalStatus != "" {
		if err := s.publishMessageGenerationEventReliable(event.RunID, map[string]interface{}{
			"type": "gateway_completed", "status": event.TerminalStatus,
		}); err != nil {
			return err
		}
		s.FinishMessageGeneration(event.RunID)
	}
	return nil
}

func normalizeGatewayExecutionEvent(input GatewayExecutionEvent) (*model.ExecutionEvent, error) {
	event := &model.ExecutionEvent{
		ConversationID: input.ConversationID, UserID: input.UserID, RunID: strings.TrimSpace(input.RunID),
		SourceKey: strings.TrimSpace(input.SourceKey), Kind: strings.TrimSpace(input.Kind),
		PayloadJSON: string(input.Payload), OccurredAt: input.OccurredAt,
	}
	var payload map[string]interface{}
	if json.Unmarshal(input.Payload, &payload) != nil {
		return nil, ErrInvalidExecutionTarget
	}
	switch event.Kind {
	case "item/agentMessage/delta":
		phase, _ := payload["phase"].(string)
		if strings.TrimSpace(phase) != "commentary" {
			event.TextDelta, _ = payload["delta"].(string)
		}
	case "item/reasoning/summaryTextDelta":
		event.ReasoningDelta, _ = payload["delta"].(string)
	case "item/reasoning/summaryPartAdded":
		if index, ok := payload["summaryIndex"].(float64); ok && index > 0 {
			event.ReasoningDelta = "\n\n"
		}
	case "turn/completed":
		normalizeGatewayTerminal(event, payload)
	}
	return event, nil
}

func normalizeGatewayTerminal(event *model.ExecutionEvent, payload map[string]interface{}) {
	turn, _ := payload["turn"].(map[string]interface{})
	status, _ := turn["status"].(string)
	switch status {
	case "completed":
		event.TerminalStatus = "completed"
	case "interrupted":
		event.TerminalStatus = "interrupted"
		event.ErrorCode = "gateway_interrupted"
		event.ErrorMessage = "local execution was interrupted"
	default:
		event.TerminalStatus = "failed"
		event.ErrorCode = "gateway_failed"
		event.ErrorMessage = "local execution failed"
		if failure, ok := turn["error"].(map[string]interface{}); ok {
			if message, _ := failure["message"].(string); strings.TrimSpace(message) != "" {
				event.ErrorMessage = message
			}
		}
	}
	if duration, ok := turn["durationMs"].(float64); ok && duration > 0 {
		event.LatencyMS = int64(duration)
	}
}

func gatewayStreamPayload(event *model.ExecutionEvent) map[string]interface{} {
	switch {
	case event.TextDelta != "":
		return map[string]interface{}{"type": "delta", "delta": event.TextDelta}
	default:
		var payload interface{}
		if json.Unmarshal([]byte(event.PayloadJSON), &payload) != nil {
			return nil
		}
		return map[string]interface{}{
			"type": "execution_event", "executionSeq": event.Seq, "runID": event.RunID,
			"kind": event.Kind, "payload": payload, "occurredAt": event.OccurredAt.UTC().Format(time.RFC3339Nano),
		}
	}
}
