package conversation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	model "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/conversation"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
	"github.com/google/uuid"
)

func (s *Service) ExecuteTurn(ctx context.Context, input SendMessageInput) (*SendMessageResult, error) {
	input.ClientRunID = EnsureMessageGenerationRunID(input.ClientRunID)
	conversation, err := s.executionConversation(ctx, input)
	if err != nil {
		return nil, err
	}
	if conversation.ExecutionType == model.ExecutionTypeCloud {
		if strings.TrimSpace(input.KeyBindingID) == "" {
			return nil, ErrInvalidKeyBinding
		}
		return s.executeCloudTurn(ctx, input, nil, false)
	}
	result, err := s.startGatewayTurn(context.WithoutCancel(ctx), input, conversation)
	if err != nil {
		return result, err
	}
	return s.awaitGatewayTurn(context.WithoutCancel(ctx), input, result, nil)
}

func (s *Service) StreamTurn(ctx context.Context, input SendMessageInput, onDelta func(string) error) (*SendMessageResult, error) {
	input.ClientRunID = EnsureMessageGenerationRunID(input.ClientRunID)
	conversation, err := s.executionConversation(ctx, input)
	if err != nil {
		return nil, err
	}
	if conversation.ExecutionType == model.ExecutionTypeCloud {
		if strings.TrimSpace(input.KeyBindingID) == "" {
			return nil, ErrInvalidKeyBinding
		}
		input.Cancelable = true
		return s.executeCloudTurn(context.WithoutCancel(ctx), input, onDelta, true)
	}
	result, err := s.startGatewayTurn(context.WithoutCancel(ctx), input, conversation)
	if err != nil {
		return result, err
	}
	return s.awaitGatewayTurn(context.WithoutCancel(ctx), input, result, onDelta)
}

func (s *Service) executeCloudTurn(ctx context.Context, input SendMessageInput, onDelta func(string) error, stream bool) (*SendMessageResult, error) {
	if len(input.InputResourceRefs) != 0 {
		return nil, ErrInvalidMessageContent
	}
	originalOnEvent := input.OnEvent
	input.OnEvent = func(kind string, payload map[string]interface{}) error {
		if durableCloudEvent(kind) {
			if err := s.recordCloudExecutionEvent(ctx, input, kind, payload); err != nil {
				return err
			}
		}
		if originalOnEvent != nil {
			return originalOnEvent(kind, payload)
		}
		return nil
	}
	wrappedDelta := func(delta string) error {
		if onDelta != nil {
			return onDelta(delta)
		}
		return nil
	}
	result, err := s.sendMessageInternal(ctx, input, wrappedDelta, stream)
	terminal := map[string]interface{}{"status": "completed"}
	if err != nil {
		terminal = map[string]interface{}{"status": "failed", "error": err.Error()}
	}
	terminalCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if eventErr := s.recordCloudExecutionEvent(terminalCtx, input, "turn.completed", terminal); err == nil && eventErr != nil {
		err = eventErr
	}
	return result, err
}

func durableCloudEvent(kind string) bool {
	kind = strings.TrimSpace(kind)
	return kind != "" && kind != "usage" && kind != "process_update" &&
		!strings.HasSuffix(kind, ".delta") && !strings.HasSuffix(kind, "_delta")
}

func (s *Service) recordCloudExecutionEvent(ctx context.Context, input SendMessageInput, kind string, payload map[string]interface{}) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = s.repo.ProjectExecutionEvent(ctx, &model.ExecutionEvent{
		ConversationID: input.ConversationID, UserID: input.UserID, RunID: normalizeRunID(input.ClientRunID),
		SourceKey: "cloud:" + strings.ReplaceAll(uuid.NewString(), "-", ""), Kind: kind,
		PayloadJSON: string(encoded), OccurredAt: time.Now().UTC(),
	})
	return err
}

func (s *Service) ListExecutionEvents(ctx context.Context, userID uint, conversationPublicID string, after uint64) ([]model.ExecutionEvent, error) {
	conversation, err := s.repo.GetConversationByPublicID(ctx, strings.TrimSpace(conversationPublicID), userID)
	if err != nil {
		return nil, ErrConversationNotFound
	}
	return s.repo.ListExecutionEvents(ctx, userID, conversation.ID, after, 500)
}

func (s *Service) executionConversation(ctx context.Context, input SendMessageInput) (*model.Conversation, error) {
	if input.UserID == 0 || input.ConversationID == 0 {
		return nil, ErrConversationNotFound
	}
	conversation, err := s.repo.GetConversationByUser(ctx, input.ConversationID, input.UserID)
	if err != nil {
		return nil, ErrConversationNotFound
	}
	if conversation.ExecutionType != model.ExecutionTypeCloud && conversation.ExecutionType != model.ExecutionTypeGateway {
		return nil, ErrInvalidExecutionTarget
	}
	return conversation, nil
}

func (s *Service) startGatewayTurn(ctx context.Context, input SendMessageInput, conversation *model.Conversation) (*SendMessageResult, error) {
	if s.gatewayExecutor == nil {
		return nil, ErrExecutionUnavailable
	}
	hasFiles := len(input.FileIDs) > 0
	validContentType := hasFiles && input.ContentType == "mixed" ||
		!hasFiles && (input.ContentType == "text" || input.ContentType == "markdown")
	if strings.TrimSpace(input.Content) == "" || !validContentType ||
		len(input.SelectedToolIDs) != 0 || len(input.SkillIDs) != 0 || len(input.InputResourceRefs) > 16 || input.HTMLVisualPromptEnabled ||
		strings.TrimSpace(input.ParentMessagePublicID) != "" || strings.TrimSpace(input.SourceMessagePublicID) != "" ||
		(strings.TrimSpace(input.BranchReason) != "" && strings.TrimSpace(input.BranchReason) != "default") {
		return nil, ErrInvalidMessageContent
	}
	runID := normalizeRunID(input.ClientRunID)
	if existingUser, existingAssistant, err := s.repo.GetMessagePairByRunID(ctx, input.UserID, runID); err == nil {
		return &SendMessageResult{UserMessage: *existingUser, AssistantMessage: *existingAssistant, ExecutionType: model.ExecutionTypeGateway}, ErrDuplicateMessageGenerationRun
	} else if !errors.Is(err, repository.ErrNotFound) {
		return nil, err
	}

	preparation, err := s.prepareMessageSendBranch(ctx, &input)
	if err != nil {
		return nil, err
	}
	attachments, err := s.resolveAttachments(ctx, input.UserID, input.FileIDs)
	if err != nil {
		return nil, err
	}
	selectedResources := make([]GatewayInputResource, 0, len(input.InputResourceRefs))
	if len(input.InputResourceRefs) > 0 {
		availableResources, resourceErr := s.gatewayExecutor.ListInputResources(
			ctx, input.UserID, conversation.ExecutionDeviceID, conversation.ExecutionWorkspaceID,
		)
		if resourceErr != nil {
			return nil, resourceErr
		}
		selectedResources, resourceErr = selectGatewayInputResources(input.InputResourceRefs, availableResources.Items)
		if resourceErr != nil {
			return nil, resourceErr
		}
	}
	providerInput := make([]map[string]string, 0, len(attachments)+len(selectedResources)+1)
	providerInput = append(providerInput, map[string]string{"kind": "text", "text": input.Content})
	for _, item := range selectedResources {
		providerInput = append(providerInput, map[string]string{"kind": item.Kind, "resourceRef": item.ResourceRef})
	}
	for _, attachment := range attachments {
		artifact, artifactErr := s.gatewayExecutor.CreateArtifact(ctx, input.UserID, conversation.ExecutionWorkspaceID, attachment.FileID)
		if artifactErr != nil {
			return nil, ErrInvalidFileReference
		}
		providerInput = append(providerInput, map[string]string{"kind": "artifact", "artifactRef": artifact.ArtifactID})
	}
	encodedInput, _ := json.Marshal(providerInput)
	settings, err := gatewayTurnSettings(input.Options)
	if err != nil {
		return nil, err
	}
	startedAt := time.Now().UTC()
	run := &model.Run{
		RunID: runID, RequestID: strings.TrimSpace(input.RequestID), UserID: input.UserID,
		ConversationID: input.ConversationID, TaskType: "agent", Endpoint: "local_gateway",
		Provider: strings.TrimSpace(conversation.Provider), ProviderProtocol: "local_gateway", Status: "queued", StartedAt: startedAt,
	}
	pair, err := s.createMessagePairWithRun(ctx, input, runID, preparation, attachments, nil, run)
	if err != nil {
		if errors.Is(err, repository.ErrDuplicate) {
			return nil, ErrDuplicateMessageGenerationRun
		}
		return nil, err
	}
	s.persistInitialConversationFallbackTitle(ctx, *conversation, *pair.user)

	s.generationStreams.register(ctx, runID, input.UserID, nil)
	if err := s.queueGatewayTurn(ctx, input, conversation, encodedInput, settings, runID); err != nil {
		endedAt := time.Now().UTC()
		_ = s.repo.FailGatewayTurn(context.Background(), input.UserID, runID, "gateway_start_failed", err.Error(), endedAt)
		s.generationStreams.finish(context.Background(), runID)
		pair.user.Status, pair.assistant.Status = "error", "error"
		pair.user.ErrorCode, pair.assistant.ErrorCode = "gateway_start_failed", "gateway_start_failed"
		pair.user.ErrorMessage, pair.assistant.ErrorMessage = err.Error(), err.Error()
		return &SendMessageResult{UserMessage: *pair.user, AssistantMessage: *pair.assistant, ExecutionType: model.ExecutionTypeGateway}, err
	}
	return &SendMessageResult{UserMessage: *pair.user, AssistantMessage: *pair.assistant, UpstreamProtocol: "local_gateway", StartedAt: startedAt, ExecutionType: model.ExecutionTypeGateway}, nil
}

func (s *Service) queueGatewayTurn(ctx context.Context, input SendMessageInput, conversation *model.Conversation, providerInput, settings json.RawMessage, runID string) error {
	thread, err := s.gatewayExecutor.GetThreadByConversation(ctx, input.UserID, conversation.ID)
	if err == nil && thread.Status != "failed" {
		err = s.gatewayExecutor.StartTurn(ctx, input.UserID, GatewayStartTurnInput{
			ThreadID: thread.ThreadID, RunID: runID, Input: providerInput, Settings: settings, IdempotencyKey: uuid.NewString(),
		})
		return err
	}
	if err != nil && !errors.Is(err, ErrExecutionBindingNotFound) {
		return err
	}
	err = s.gatewayExecutor.StartThread(ctx, input.UserID, GatewayStartThreadInput{
		DeviceID: conversation.ExecutionDeviceID, ProfileID: conversation.ExecutionProfileID, WorkspaceID: conversation.ExecutionWorkspaceID,
		ConversationID: conversation.ID, Title: conversation.Title, Settings: settings,
		InitialInput: providerInput, InitialRunID: runID, IdempotencyKey: uuid.NewString(),
	})
	return err
}

func gatewayTurnSettings(options map[string]interface{}) (json.RawMessage, error) {
	settings := map[string]string{}
	allowed := map[string]map[string]bool{
		"reasoningEffort": {"low": true, "medium": true, "high": true, "xhigh": true},
		"approvalPolicy":  {"untrusted": true, "on-request": true, "never": true},
		"sandboxPolicy":   {"read-only": true, "workspace-write": true},
	}
	for key, value := range options {
		values, ok := allowed[key]
		text, textOK := value.(string)
		if !ok || !textOK || !values[text] {
			return nil, ErrInvalidExecutionTarget
		}
		settings[key] = text
	}
	encoded, err := json.Marshal(settings)
	return encoded, err
}

func (s *Service) awaitGatewayTurn(ctx context.Context, input SendMessageInput, initial *SendMessageResult, onDelta func(string) error) (*SendMessageResult, error) {
	runID := initial.AssistantMessage.RunID
	replay, events, unsubscribe, ok := s.SubscribeMessageGeneration(ctx, input.UserID, runID, 0)
	if !ok {
		return initial, ErrExecutionUnavailable
	}
	defer unsubscribe()
	handle := func(payload map[string]interface{}) (bool, error) {
		typeName, _ := payload["type"].(string)
		switch typeName {
		case "delta":
			if delta, _ := payload["delta"].(string); delta != "" && onDelta != nil {
				return false, onDelta(delta)
			}
		case "upstream_think_delta":
			if input.OnEvent != nil {
				return false, input.OnEvent(typeName, payload)
			}
		case "gateway_completed":
			return true, nil
		}
		return false, nil
	}
	for _, event := range replay {
		if done, err := handle(event.Payload); done || err != nil {
			return s.gatewayTurnResult(ctx, input.UserID, runID, initial, err)
		}
	}
	for event := range events {
		if done, err := handle(event.Payload); done || err != nil {
			return s.gatewayTurnResult(ctx, input.UserID, runID, initial, err)
		}
	}
	return initial, ErrExecutionUnavailable
}

func (s *Service) gatewayTurnResult(ctx context.Context, userID uint, runID string, initial *SendMessageResult, streamErr error) (*SendMessageResult, error) {
	userMessage, assistantMessage, err := s.repo.GetMessagePairByRunID(ctx, userID, runID)
	if err != nil {
		return initial, err
	}
	result := &SendMessageResult{UserMessage: *userMessage, AssistantMessage: *assistantMessage, ExecutionType: model.ExecutionTypeGateway}
	if streamErr != nil {
		return result, streamErr
	}
	if assistantMessage.Status == "success" {
		return result, nil
	}
	return result, fmt.Errorf("%w: %s", ErrUpstreamRequestFailed, assistantMessage.ErrorMessage)
}

func (s *Service) GetTurnResult(ctx context.Context, userID uint, runID string) (*SendMessageResult, error) {
	return s.gatewayTurnResult(ctx, userID, normalizeRunID(runID), nil, nil)
}
