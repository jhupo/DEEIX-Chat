package conversation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	model "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/conversation"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/nativetool"
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
	if _, err := nativetool.ResolveConversationPluginRefs(input.InputResourceRefs, s.cfg.Snapshot().ConversationPluginKeys); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrConversationPluginUnavailable, err)
	}
	projection := newCloudExecutionProjection(s, ctx, input, input.OnEvent)
	input.OnEvent = projection.handle
	wrappedDelta := func(delta string) error {
		if err := projection.start(); err != nil {
			return err
		}
		if onDelta != nil {
			return onDelta(delta)
		}
		return nil
	}
	result, err := s.sendMessageInternal(ctx, input, wrappedDelta, stream)
	terminalStatus := "completed"
	if err != nil {
		terminalStatus = "failed"
		if errors.Is(err, ErrMessageGenerationCanceled) {
			terminalStatus = "interrupted"
		}
	}
	terminalCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	projection.ctx = terminalCtx
	if eventErr := projection.complete(terminalStatus, err, result); err == nil && eventErr != nil {
		err = eventErr
	}
	return result, err
}

type cloudReasoningProjection struct {
	itemID string
	text   string
	open   bool
}

type cloudToolProjection struct {
	itemID   string
	row      model.ToolCall
	terminal bool
}

type cloudExecutionProjection struct {
	service        *Service
	ctx            context.Context
	input          SendMessageInput
	emit           func(string, map[string]interface{}) error
	reasoning      map[string]*cloudReasoningProjection
	reasoningOrder []string
	tools          map[string]*cloudToolProjection
	toolOrder      []string
	started        bool
}

func newCloudExecutionProjection(
	service *Service,
	ctx context.Context,
	input SendMessageInput,
	emit func(string, map[string]interface{}) error,
) *cloudExecutionProjection {
	return &cloudExecutionProjection{
		service: service, ctx: ctx, input: input, emit: emit,
		reasoning: make(map[string]*cloudReasoningProjection),
		tools:     make(map[string]*cloudToolProjection),
	}
}

func (p *cloudExecutionProjection) start() error {
	if p.started {
		return nil
	}
	if err := p.project("turn/started", map[string]interface{}{
		"turn": map[string]interface{}{"status": "inProgress"},
	}); err != nil {
		return err
	}
	p.started = true
	return nil
}

func (p *cloudExecutionProjection) handle(kind string, payload map[string]interface{}) error {
	if err := p.start(); err != nil {
		return err
	}
	switch strings.TrimSpace(kind) {
	case cloudReasoningSummaryEvent:
		return p.projectReasoning(payload)
	case cloudToolActivityEvent:
		return p.projectTools(payload)
	default:
		if p.emit != nil {
			return p.emit(kind, payload)
		}
		return nil
	}
}

func (p *cloudExecutionProjection) project(kind string, payload map[string]interface{}) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	event := &model.ExecutionEvent{
		ConversationID: p.input.ConversationID, UserID: p.input.UserID, RunID: normalizeRunID(p.input.ClientRunID),
		SourceKey: "cloud:" + strings.ReplaceAll(uuid.NewString(), "-", ""), Kind: kind,
		PayloadJSON: string(encoded), OccurredAt: time.Now().UTC(),
	}
	applied, err := p.service.repo.ProjectExecutionEvent(p.ctx, event)
	if err != nil || !applied || p.emit == nil {
		return err
	}
	streamPayload := gatewayStreamPayload(event)
	delete(streamPayload, "type")
	return p.emit("execution_event", streamPayload)
}

func cloudProjectionString(payload map[string]interface{}, key string) string {
	value, _ := payload[key].(string)
	return strings.TrimSpace(value)
}

func cloudReasoningIdentity(payload map[string]interface{}) (string, string) {
	return cloudProjectionString(payload, "itemID"), cloudProjectionString(payload, "kind")
}

func (p *cloudExecutionProjection) projectReasoning(payload map[string]interface{}) error {
	itemID, kind := cloudReasoningIdentity(payload)
	if itemID == "" {
		return nil
	}
	state := p.reasoning[itemID]
	if state == nil {
		state = &cloudReasoningProjection{itemID: itemID}
		p.reasoning[itemID] = state
		p.reasoningOrder = append(p.reasoningOrder, itemID)
	}

	if kind == messageTraceThinkKindSummary || kind == "summary_part_added" {
		if !state.open {
			if err := p.project("item/started", map[string]interface{}{
				"itemID": itemID,
				"item": map[string]interface{}{
					"itemID": itemID, "kind": "reasoning", "status": "inProgress", "summary": []string{},
				},
			}); err != nil {
				return err
			}
			state.open = true
		}
		if kind == "summary_part_added" {
			if err := p.project("item/reasoning/summaryPartAdded", map[string]interface{}{"itemID": itemID}); err != nil {
				return err
			}
		} else {
			delta := cloudProjectionString(payload, "delta")
			if delta == "" {
				content := cloudProjectionString(payload, "contentMarkdown")
				if strings.HasPrefix(content, state.text) {
					delta = content[len(state.text):]
				}
			}
			if delta != "" {
				state.text += delta
				if err := p.project("item/reasoning/summaryTextDelta", map[string]interface{}{
					"itemID": itemID, "delta": delta,
				}); err != nil {
					return err
				}
			}
		}
	}

	status := cloudProjectionString(payload, "status")
	if status == messageTraceStatusCompleted || status == messageTraceStatusError {
		return p.completeReasoning(state, status)
	}
	return nil
}

func (p *cloudExecutionProjection) completeReasoning(state *cloudReasoningProjection, status string) error {
	if state == nil || !state.open {
		return nil
	}
	state.open = false
	itemStatus := "completed"
	if status == messageTraceStatusError || status == "failed" {
		itemStatus = "failed"
	}
	return p.project("item/completed", map[string]interface{}{
		"itemID": state.itemID,
		"item": map[string]interface{}{
			"itemID": state.itemID, "kind": "reasoning", "status": itemStatus, "summary": []string{state.text},
		},
	})
}

func cloudToolStatus(status string) (string, bool) {
	switch strings.TrimSpace(status) {
	case "success", "completed", "reused":
		return "completed", true
	case "error", "failed":
		return "failed", true
	default:
		return "inProgress", false
	}
}

func cloudToolItem(row model.ToolCall, itemID, status string) map[string]interface{} {
	return map[string]interface{}{
		"itemID":     itemID,
		"kind":       "dynamicToolCall",
		"status":     status,
		"tool":       strings.TrimSpace(row.ToolName),
		"toolType":   strings.TrimSpace(row.ToolType),
		"arguments":  strings.TrimSpace(row.InputJSON),
		"result":     strings.TrimSpace(row.OutputJSON),
		"error":      strings.TrimSpace(row.ErrorJSON),
		"durationMs": row.LatencyMS,
	}
}

func (p *cloudExecutionProjection) projectTools(payload map[string]interface{}) error {
	rows, _ := payload["tools"].([]model.ToolCall)
	for _, row := range rows {
		itemID := strings.TrimSpace(row.ToolCallID)
		if itemID == "" || strings.TrimSpace(row.ToolName) == "" {
			continue
		}
		status, terminal := cloudToolStatus(row.Status)
		state := p.tools[itemID]
		if state == nil {
			state = &cloudToolProjection{itemID: itemID}
			p.tools[itemID] = state
			p.toolOrder = append(p.toolOrder, itemID)
			if err := p.project("item/started", map[string]interface{}{
				"itemID": itemID, "item": cloudToolItem(row, itemID, "inProgress"),
			}); err != nil {
				return err
			}
		}
		state.row = row
		if terminal && !state.terminal {
			state.terminal = true
			if err := p.project("item/completed", map[string]interface{}{
				"itemID": itemID, "item": cloudToolItem(row, itemID, status),
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *cloudExecutionProjection) complete(status string, runErr error, result *SendMessageResult) error {
	if err := p.start(); err != nil {
		return err
	}
	reasoningStatus := messageTraceStatusCompleted
	if status == "failed" {
		reasoningStatus = messageTraceStatusError
	}
	for _, itemID := range p.reasoningOrder {
		if err := p.completeReasoning(p.reasoning[itemID], reasoningStatus); err != nil {
			return err
		}
	}
	for _, itemID := range p.toolOrder {
		state := p.tools[itemID]
		if state.terminal {
			continue
		}
		itemStatus := "completed"
		if status == "failed" {
			itemStatus = "failed"
		} else if status == "interrupted" {
			itemStatus = "interrupted"
		}
		if err := p.project("item/completed", map[string]interface{}{
			"itemID": itemID, "item": cloudToolItem(state.row, itemID, itemStatus),
		}); err != nil {
			return err
		}
		state.terminal = true
	}
	turn := map[string]interface{}{"status": status}
	if runErr != nil {
		turn["error"] = map[string]interface{}{"message": runErr.Error()}
	}
	if result != nil && result.AssistantMessage.LatencyMS > 0 {
		turn["durationMs"] = result.AssistantMessage.LatencyMS
	}
	return p.project("turn/completed", map[string]interface{}{"turn": turn})
}

const executionEventPageSize = 500

type ExecutionEventPage struct {
	Events  []model.ExecutionEvent
	Cursor  uint64
	HasMore bool
}

func (s *Service) ListExecutionEvents(ctx context.Context, userID uint, conversationPublicID string, after uint64, runIDs []string) (*ExecutionEventPage, error) {
	conversation, err := s.repo.GetConversationByPublicID(ctx, strings.TrimSpace(conversationPublicID), userID)
	if err != nil {
		return nil, ErrConversationNotFound
	}
	if after > 0 {
		events, listErr := s.repo.ListExecutionEvents(ctx, userID, conversation.ID, after, nil, executionEventPageSize)
		if listErr != nil {
			return nil, listErr
		}
		cursor := conversation.ExecutionEventSeq
		if len(events) > 0 {
			cursor = events[len(events)-1].Seq
		}
		return &ExecutionEventPage{
			Events: compactExecutionHistory(events), Cursor: cursor,
			HasMore: len(events) == executionEventPageSize && cursor < conversation.ExecutionEventSeq,
		}, nil
	}

	runIDs = normalizedExecutionRunIDs(runIDs)
	if len(runIDs) == 0 {
		return &ExecutionEventPage{Events: []model.ExecutionEvent{}, Cursor: conversation.ExecutionEventSeq}, nil
	}
	events, err := s.repo.ListExecutionEventHistory(ctx, userID, conversation.ID, runIDs)
	if err != nil {
		return nil, err
	}
	cursor := conversation.ExecutionEventSeq
	if len(events) > 0 {
		cursor = max(cursor, events[len(events)-1].Seq)
	}
	return &ExecutionEventPage{
		Events: compactExecutionHistory(events), Cursor: cursor,
	}, nil
}

func normalizedExecutionRunIDs(values []string) []string {
	result := make([]string, 0, min(len(values), 64))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = normalizeRunID(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
		if len(result) == 64 {
			break
		}
	}
	return result
}

type compactedTextDelta struct {
	event     model.ExecutionEvent
	text      strings.Builder
	truncated bool
}

func compactExecutionHistory(events []model.ExecutionEvent) []model.ExecutionEvent {
	retained := make([]model.ExecutionEvent, 0, len(events)/4)
	latest := make(map[string]model.ExecutionEvent)
	first := make(map[string]model.ExecutionEvent)
	deltas := make(map[string]*compactedTextDelta)
	for _, event := range events {
		itemID, delta, truncated := executionEventDelta(event)
		key := event.RunID + ":" + itemID
		switch event.Kind {
		case "item/commandExecution/outputDelta", "item/agentMessage/delta", "item/reasoning/summaryTextDelta":
			if itemID == "" || delta == "" {
				continue
			}
			buffer := deltas[event.Kind+":"+key]
			if buffer == nil {
				buffer = &compactedTextDelta{event: event}
				deltas[event.Kind+":"+key] = buffer
			}
			if buffer.text.Len() < maxExecutionHistoryTextBytes {
				remaining := maxExecutionHistoryTextBytes - buffer.text.Len()
				if len(delta) > remaining {
					for remaining > 0 && !utf8.ValidString(delta[:remaining]) {
						remaining--
					}
					delta = delta[:remaining]
					buffer.truncated = true
				}
				buffer.text.WriteString(delta)
			} else {
				buffer.truncated = true
			}
			buffer.truncated = buffer.truncated || truncated
		case "item/reasoning/summaryPartAdded":
			buffer := deltas["item/reasoning/summaryTextDelta:"+key]
			if buffer != nil && buffer.text.Len() > 0 && buffer.text.Len()+2 <= maxExecutionHistoryTextBytes {
				buffer.text.WriteString("\n\n")
			}
		case "item/reasoning/textDelta", "item/plan/delta":
			continue
		case "turn/started":
			if _, exists := first[event.RunID+":"+event.Kind]; !exists {
				first[event.RunID+":"+event.Kind] = event
			}
		case "thread/tokenUsage/updated":
			latest[event.RunID+":"+event.Kind] = event
		case "turn/completed", "turn/plan/updated", "turn/diff/updated", "model/rerouted":
			latest[event.RunID+":"+event.Kind] = event
		case "item/fileChange/patchUpdated":
			latest[event.Kind+":"+key] = event
		case "item/started":
			if itemID == "" {
				retained = append(retained, event)
			} else if _, exists := first[event.Kind+":"+key]; !exists {
				first[event.Kind+":"+key] = event
			}
		case "item/completed":
			if itemID == "" {
				retained = append(retained, event)
			} else {
				latest[event.Kind+":"+key] = compactExecutionItemEvent(event)
			}
		}
	}
	for _, event := range first {
		retained = append(retained, event)
	}
	for _, event := range latest {
		retained = append(retained, event)
	}
	for _, buffer := range deltas {
		payload := map[string]any{"itemID": executionEventItemID(buffer.event), "truncated": buffer.truncated}
		if buffer.event.Kind == "item/commandExecution/outputDelta" {
			payload["outputDelta"] = buffer.text.String()
		} else {
			payload["delta"] = buffer.text.String()
			if buffer.event.Kind == "item/agentMessage/delta" {
				payload["phase"] = "commentary"
			}
		}
		encoded, _ := json.Marshal(payload)
		buffer.event.PayloadJSON = string(encoded)
		retained = append(retained, buffer.event)
	}
	sort.Slice(retained, func(i, j int) bool { return retained[i].Seq < retained[j].Seq })
	return retained
}

const maxExecutionHistoryTextBytes = 256 << 10

const maxExecutionHistoryToolFieldBytes = 16 << 10

func compactExecutionItemEvent(event model.ExecutionEvent) model.ExecutionEvent {
	var payload struct {
		Item map[string]any `json:"item"`
	}
	if json.Unmarshal([]byte(event.PayloadJSON), &payload) != nil || payload.Item == nil {
		return event
	}
	kind, _ := payload.Item["kind"].(string)
	switch strings.TrimSpace(kind) {
	case "mcpToolCall", "dynamicToolCall", "collabToolCall", "webSearch", "imageGeneration":
	default:
		return event
	}
	truncated := false
	for _, field := range []string{"arguments", "result", "error"} {
		value, ok := payload.Item[field].(string)
		if !ok {
			continue
		}
		value, clipped := truncateExecutionHistoryText(value, maxExecutionHistoryToolFieldBytes)
		if clipped {
			payload.Item[field] = value
			truncated = true
		}
	}
	if !truncated {
		return event
	}
	payload.Item["truncated"] = true
	encoded, err := json.Marshal(payload)
	if err == nil {
		event.PayloadJSON = string(encoded)
	}
	return event
}

func truncateExecutionHistoryText(value string, limit int) (string, bool) {
	if len(value) <= limit {
		return value, false
	}
	for limit > 0 && !utf8.ValidString(value[:limit]) {
		limit--
	}
	return value[:limit], true
}

func executionEventItemID(event model.ExecutionEvent) string {
	var payload struct {
		ItemID string `json:"itemID"`
	}
	_ = json.Unmarshal([]byte(event.PayloadJSON), &payload)
	return strings.TrimSpace(payload.ItemID)
}

func executionEventDelta(event model.ExecutionEvent) (string, string, bool) {
	var payload struct {
		ItemID      string `json:"itemID"`
		Delta       string `json:"delta"`
		OutputDelta string `json:"outputDelta"`
		Phase       string `json:"phase"`
		Truncated   bool   `json:"truncated"`
	}
	if json.Unmarshal([]byte(event.PayloadJSON), &payload) != nil ||
		(event.Kind == "item/agentMessage/delta" && payload.Phase != "commentary") {
		return "", "", false
	}
	if event.Kind == "item/commandExecution/outputDelta" {
		payload.Delta = payload.OutputDelta
	}
	return strings.TrimSpace(payload.ItemID), payload.Delta, payload.Truncated
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
		len(input.SelectedToolIDs) != 0 || len(input.SkillIDs) != 0 || len(input.KnowledgeBaseIDs) != 0 || len(input.InputResourceRefs) > 16 || input.HTMLVisualPromptEnabled ||
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
	settings, err := gatewayTurnSettings(input.PlatformModelName, input.Options)
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

	s.generationStreams.register(ctx, runID, input.UserID, conversation.PublicID, nil)
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

func gatewayTurnSettings(modelName string, options map[string]interface{}) (json.RawMessage, error) {
	settings := map[string]string{}
	if modelName = strings.TrimSpace(modelName); modelName == "" || len(modelName) > 256 || len(options) != 4 {
		return nil, ErrInvalidExecutionTarget
	}
	settings["model"] = modelName
	allowed := map[string]map[string]bool{
		"reasoningEffort":   {"low": true, "medium": true, "high": true, "xhigh": true},
		"approvalPolicy":    {"on-request": true, "never": true},
		"approvalsReviewer": {"user": true, "auto_review": true},
		"sandboxPolicy":     {"workspace-write": true, "danger-full-access": true},
	}
	for key, value := range options {
		values, ok := allowed[key]
		text, textOK := value.(string)
		if !ok || !textOK || !values[text] {
			return nil, ErrInvalidExecutionTarget
		}
		settings[key] = text
	}
	if !validGatewayApprovalMode(settings["approvalPolicy"], settings["approvalsReviewer"], settings["sandboxPolicy"]) {
		return nil, ErrInvalidExecutionTarget
	}
	encoded, err := json.Marshal(settings)
	return encoded, err
}

func validGatewayApprovalMode(approvalPolicy, approvalsReviewer, sandboxPolicy string) bool {
	return approvalPolicy == "on-request" && approvalsReviewer == "user" && sandboxPolicy == "workspace-write" ||
		approvalPolicy == "on-request" && approvalsReviewer == "auto_review" && sandboxPolicy == "workspace-write" ||
		approvalPolicy == "never" && approvalsReviewer == "user" && sandboxPolicy == "danger-full-access"
}

func (s *Service) awaitGatewayTurn(ctx context.Context, input SendMessageInput, initial *SendMessageResult, onDelta func(string) error) (*SendMessageResult, error) {
	runID := initial.AssistantMessage.RunID
	replay, events, unsubscribe, ok := s.SubscribeMessageGeneration(ctx, input.UserID, runID, 0, true)
	if !ok {
		return initial, ErrExecutionUnavailable
	}
	defer unsubscribe()
	handle := func(payload map[string]interface{}) (bool, error) {
		typeName, _ := payload["type"].(string)
		switch typeName {
		case "delta":
			if delta, _ := payload["delta"].(string); delta != "" {
				// Gateway deltas already carry the durable stream sequence. Keep the
				// original payload for the HTTP stream so a later resume starts after
				// the last event instead of replaying the whole run.
				if input.OnEvent != nil {
					return false, input.OnEvent(typeName, payload)
				}
				if onDelta != nil {
					return false, onDelta(delta)
				}
			}
		case "execution_event":
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

func (s *Service) SteerConversationRun(ctx context.Context, userID uint, runID, content, idempotencyKey string) error {
	runID, content, idempotencyKey = normalizeRunID(runID), strings.TrimSpace(content), strings.TrimSpace(idempotencyKey)
	if userID == 0 || runID == "" || content == "" || len(content) > 1024*1024 || idempotencyKey == "" {
		return ErrInvalidExecutionTarget
	}
	conversation, err := s.repo.GetConversationExecutionByRunID(ctx, userID, runID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrExecutionConflict
		}
		return err
	}
	if conversation.ExecutionType != model.ExecutionTypeGateway || s.gatewayExecutor == nil {
		return ErrExecutionConflict
	}
	encoded, err := json.Marshal([]map[string]string{{"kind": "text", "text": content}})
	if err != nil {
		return err
	}
	if err = s.gatewayExecutor.SteerRun(ctx, userID, runID, idempotencyKey, encoded); err != nil {
		if errors.Is(err, ErrExecutionBindingNotFound) || errors.Is(err, ErrExecutionUnavailable) {
			return ErrExecutionConflict
		}
		return err
	}
	return nil
}
