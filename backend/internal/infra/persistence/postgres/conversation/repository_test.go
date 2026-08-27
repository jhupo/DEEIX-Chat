package conversation

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	domainconversation "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/conversation"
	model "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/models"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/testutil"
	"gorm.io/gorm"
)

func TestTranslateErrorAllowsNil(t *testing.T) {
	if err := translateError(nil); err != nil {
		t.Fatalf("translateError(nil) = %v, want nil", err)
	}
}

func TestGatewayProjectionRecoversInterruptedHTTPStream(t *testing.T) {
	db := openConversationRepositoryTestDB(t)
	if err := db.AutoMigrate(&model.ConversationExecutionEvent{}); err != nil {
		t.Fatal(err)
	}
	repo := NewRepo(db)
	now := time.Now().UTC()
	conversation := model.Conversation{
		UserID: 1, PublicID: "gateway_recovery", Title: "gateway recovery", LabelsJSON: "[]",
		ExecutionType: domainconversation.ExecutionTypeGateway, SessionKey: "gateway_recovery", Status: "active",
	}
	if err := db.Create(&conversation).Error; err != nil {
		t.Fatal(err)
	}
	runID := "run_gateway_recovery"
	messages := []model.Message{
		{ConversationID: conversation.ID, UserID: 1, PublicID: "gateway_user", Role: "user", ContentType: "text", Content: "continue", BranchReason: "default", Status: "success", RunID: runID},
		{ConversationID: conversation.ID, UserID: 1, PublicID: "gateway_assistant", Role: "assistant", ContentType: "text", Content: "partial", BranchReason: "default", Status: "error", ErrorCode: "stream_interrupted", ErrorMessage: "stream closed", RunID: runID},
	}
	if err := db.Create(&messages).Error; err != nil {
		t.Fatal(err)
	}
	run := model.ConversationRun{RunID: runID, UserID: 1, ConversationID: conversation.ID, Status: "running", StartedAt: now}
	if err := db.Create(&run).Error; err != nil {
		t.Fatal(err)
	}
	delta := &domainconversation.ExecutionEvent{
		ConversationID: conversation.ID, UserID: 1, RunID: runID, SourceKey: "agent:delta", Kind: "item/agentMessage/delta",
		PayloadJSON: `{"delta":" continued"}`, TextDelta: " continued", OccurredAt: now.Add(time.Second),
	}
	if applied, err := repo.ProjectExecutionEvent(context.Background(), delta); err != nil || !applied {
		t.Fatalf("project recovery delta: applied=%v err=%v", applied, err)
	}
	terminal := &domainconversation.ExecutionEvent{
		ConversationID: conversation.ID, UserID: 1, RunID: runID, SourceKey: "agent:completed", Kind: "turn/completed",
		PayloadJSON: `{"turn":{"status":"completed"}}`, TerminalStatus: "completed", OccurredAt: now.Add(2 * time.Second),
	}
	if applied, err := repo.ProjectExecutionEvent(context.Background(), terminal); err != nil || !applied {
		t.Fatalf("project recovery terminal: applied=%v err=%v", applied, err)
	}
	var assistant model.Message
	if err := db.First(&assistant, messages[1].ID).Error; err != nil {
		t.Fatal(err)
	}
	if assistant.Status != "success" || assistant.Content != "partial continued" || assistant.ErrorCode != "" || assistant.ErrorMessage != "" {
		t.Fatalf("recovered assistant = %#v", assistant)
	}
	if err := db.First(&run, run.ID).Error; err != nil || run.Status != "success" || run.EndedAt == nil {
		t.Fatalf("recovered run = %#v, %v", run, err)
	}
	late := &domainconversation.ExecutionEvent{
		ConversationID: conversation.ID, UserID: 1, RunID: runID, SourceKey: "agent:late-delta", Kind: "item/agentMessage/delta",
		PayloadJSON: `{"delta":" must not be appended"}`, TextDelta: " must not be appended", OccurredAt: now.Add(3 * time.Second),
	}
	if applied, err := repo.ProjectExecutionEvent(context.Background(), late); err != nil || applied {
		t.Fatalf("late gateway event: applied=%v err=%v", applied, err)
	}
	if err := db.First(&assistant, messages[1].ID).Error; err != nil || assistant.Content != "partial continued" {
		t.Fatalf("late gateway event changed assistant = %#v, %v", assistant, err)
	}
}

func TestGatewayProjectionTruncatesLongTerminalError(t *testing.T) {
	db := openConversationRepositoryTestDB(t)
	if err := db.AutoMigrate(&model.ConversationExecutionEvent{}); err != nil {
		t.Fatal(err)
	}
	repo := NewRepo(db)
	now := time.Now().UTC()
	conversation := model.Conversation{
		UserID: 1, PublicID: "gateway_long_error", Title: "gateway long error", LabelsJSON: "[]",
		ExecutionType: domainconversation.ExecutionTypeGateway, SessionKey: "gateway_long_error", Status: "active",
	}
	if err := db.Create(&conversation).Error; err != nil {
		t.Fatal(err)
	}
	runID := "run_gateway_long_error"
	messages := []model.Message{
		{ConversationID: conversation.ID, UserID: 1, PublicID: "gateway_long_error_user", Role: "user", ContentType: "text", Content: "continue", BranchReason: "default", Status: "pending", RunID: runID},
		{ConversationID: conversation.ID, UserID: 1, PublicID: "gateway_long_error_assistant", Role: "assistant", ContentType: "text", BranchReason: "default", Status: "pending", RunID: runID},
	}
	if err := db.Create(&messages).Error; err != nil {
		t.Fatal(err)
	}
	run := model.ConversationRun{RunID: runID, UserID: 1, ConversationID: conversation.ID, Status: "running", StartedAt: now}
	if err := db.Create(&run).Error; err != nil {
		t.Fatal(err)
	}
	rawError := strings.Repeat("A", 130) + strings.Repeat("错", 130)
	payload := `{"turn":{"status":"failed","error":"` + rawError + `"}}`
	terminal := &domainconversation.ExecutionEvent{
		ConversationID: conversation.ID, UserID: 1, RunID: runID, SourceKey: "agent:long-error", Kind: "turn/completed",
		PayloadJSON: payload, TerminalStatus: "failed", ErrorCode: "upstream_forbidden", ErrorMessage: rawError,
		OccurredAt: now.Add(time.Second),
	}
	if applied, err := repo.ProjectExecutionEvent(context.Background(), terminal); err != nil || !applied {
		t.Fatalf("project long-error terminal: applied=%v err=%v", applied, err)
	}

	wantError := truncateText(rawError, 255)
	var assistant model.Message
	if err := db.First(&assistant, messages[1].ID).Error; err != nil {
		t.Fatal(err)
	}
	if assistant.Status != "error" || assistant.ErrorCode != "upstream_forbidden" || assistant.ErrorMessage != wantError {
		t.Fatalf("projected assistant status/error = %q/%q/%q", assistant.Status, assistant.ErrorCode, assistant.ErrorMessage)
	}
	if err := db.First(&run, run.ID).Error; err != nil {
		t.Fatal(err)
	}
	if run.Status != "error" || run.ErrorCode != "upstream_forbidden" || run.ErrorMessage != wantError || run.EndedAt == nil {
		t.Fatalf("projected run = %#v", run)
	}
	if len([]rune(wantError)) != 255 || len([]rune(rawError)) <= len([]rune(wantError)) {
		t.Fatalf("error lengths: raw=%d truncated=%d", len([]rune(rawError)), len([]rune(wantError)))
	}
	var event model.ConversationExecutionEvent
	if err := db.Where("source_key = ?", terminal.SourceKey).First(&event).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(event.PayloadJSON, rawError) {
		t.Fatalf("execution event payload did not retain the full error: %q", event.PayloadJSON)
	}
}

func TestReconcileOrphanGatewayTurnsRepairsTerminalAndUndispatchedRuns(t *testing.T) {
	db := openConversationRepositoryTestDB(t)
	if err := db.AutoMigrate(&model.AgentTurn{}); err != nil {
		t.Fatal(err)
	}
	repo := NewRepo(db)
	now := time.Now().UTC()
	conversation := model.Conversation{
		UserID: 1, PublicID: "gateway_dispatch_reconcile", Title: "gateway dispatch reconcile", LabelsJSON: "[]",
		ExecutionType: domainconversation.ExecutionTypeGateway, SessionKey: "gateway_dispatch_reconcile", Status: "active",
	}
	if err := db.Create(&conversation).Error; err != nil {
		t.Fatal(err)
	}
	createRun := func(runID, endpoint string, createdAt time.Time) model.ConversationRun {
		run := model.ConversationRun{
			RunID: runID, UserID: 1, ConversationID: conversation.ID, TaskType: "agent", Endpoint: endpoint,
			ProviderProtocol: endpoint, Status: "queued", StartedAt: createdAt, BaseModel: model.BaseModel{CreatedAt: createdAt},
		}
		if err := db.Create(&run).Error; err != nil {
			t.Fatal(err)
		}
		return run
	}
	old := now.Add(-5 * time.Minute)
	orphan := createRun("run_gateway_orphan", "local_gateway", old)
	dispatched := createRun("run_gateway_dispatched", "local_gateway", old)
	young := createRun("run_gateway_young", "local_gateway", now)
	cloud := createRun("run_cloud_queued", "responses", old)
	terminal := createRun("run_gateway_terminal", "local_gateway", old)
	if err := db.Model(&terminal).Update("status", "running").Error; err != nil {
		t.Fatal(err)
	}
	messages := []model.Message{
		{ConversationID: conversation.ID, UserID: 1, PublicID: "gateway_orphan_user", Role: "user", ContentType: "text", Content: "continue", BranchReason: "default", Status: "pending", RunID: orphan.RunID},
		{ConversationID: conversation.ID, UserID: 1, PublicID: "gateway_orphan_assistant", Role: "assistant", ContentType: "text", BranchReason: "default", Status: "pending", RunID: orphan.RunID},
		{ConversationID: conversation.ID, UserID: 1, PublicID: "gateway_terminal_user", Role: "user", ContentType: "text", Content: "continue", BranchReason: "default", Status: "success", RunID: terminal.RunID},
		{ConversationID: conversation.ID, UserID: 1, PublicID: "gateway_terminal_assistant", Role: "assistant", ContentType: "text", Content: "partial", BranchReason: "default", Status: "error", ErrorCode: "stream_interrupted", ErrorMessage: "stream closed", RunID: terminal.RunID},
	}
	if err := db.Create(&messages).Error; err != nil {
		t.Fatal(err)
	}
	turn := model.AgentTurn{
		PublicID: "agturn_dispatched_0123456789abcdef", UserID: 1, ThreadID: 1, RunID: dispatched.RunID,
		Status: "queued", InputJSON: `[]`, SettingsJSON: `{}`,
	}
	if err := db.Create(&turn).Error; err != nil {
		t.Fatal(err)
	}
	failedTurn := model.AgentTurn{
		PublicID: "agturn_terminal_0123456789abcdef", UserID: 1, ThreadID: 1, RunID: terminal.RunID,
		Status: "failed", ErrorCode: "device_revoked", ErrorMessage: "device revoked",
		InputJSON: `[]`, SettingsJSON: `{}`, ControlPlaneModel: model.ControlPlaneModel{CreatedAt: old, UpdatedAt: old},
	}
	if err := db.Create(&failedTurn).Error; err != nil {
		t.Fatal(err)
	}

	runIDs, err := repo.ReconcileOrphanGatewayTurns(context.Background(), now.Add(-time.Minute), 10)
	if err != nil || !reflect.DeepEqual(runIDs, []string{terminal.RunID, orphan.RunID}) {
		t.Fatalf("ReconcileOrphanGatewayTurns() = %v, %v", runIDs, err)
	}
	if err := db.First(&orphan, orphan.ID).Error; err != nil || orphan.Status != "error" || orphan.ErrorCode != "gateway_dispatch_interrupted" || orphan.EndedAt == nil {
		t.Fatalf("orphan run = %#v, %v", orphan, err)
	}
	for index := range messages {
		if err := db.First(&messages[index], messages[index].ID).Error; err != nil {
			t.Fatal(err)
		}
	}
	if messages[0].Status != "error" || messages[0].ErrorCode != "gateway_dispatch_interrupted" ||
		messages[1].Status != "error" || messages[1].ErrorCode != "gateway_dispatch_interrupted" {
		t.Fatalf("orphan messages = %#v", messages[:2])
	}
	if messages[2].Status != "success" || messages[3].Status != "error" || messages[3].ErrorCode != "device_revoked" || messages[3].ErrorMessage != "device revoked" {
		t.Fatalf("terminal messages = %#v", messages[2:])
	}
	if err := db.First(&terminal, terminal.ID).Error; err != nil || terminal.Status != "error" || terminal.ErrorCode != "device_revoked" || terminal.EndedAt == nil {
		t.Fatalf("terminal run = %#v, %v", terminal, err)
	}
	for _, run := range []*model.ConversationRun{&dispatched, &young, &cloud} {
		if err := db.First(run, run.ID).Error; err != nil || run.Status != "queued" {
			t.Fatalf("protected run = %#v, %v", run, err)
		}
	}
	if replay, err := repo.ReconcileOrphanGatewayTurns(context.Background(), now.Add(-time.Minute), 10); err != nil || len(replay) != 0 {
		t.Fatalf("replayed reconciliation = %v, %v", replay, err)
	}
}

func TestAttachmentDurationSecondsFromMetaJSON(t *testing.T) {
	if got := attachmentDurationSecondsFromMetaJSON(`{"duration_seconds":6}`); got != 6 {
		t.Fatalf("expected attachment duration 6, got %d", got)
	}
	for _, raw := range []string{"", `{}`, `{"duration_seconds":0}`, `{"duration_seconds":"6"}`} {
		if got := attachmentDurationSecondsFromMetaJSON(raw); got != 0 {
			t.Fatalf("expected invalid attachment duration for %q, got %d", raw, got)
		}
	}
}

func TestCreateContextArtifactsRejectsIncompleteOwnerScope(t *testing.T) {
	repo := NewRepo(openConversationRepositoryTestDB(t))
	valid := domainconversation.ContextArtifact{
		ConversationID: 7,
		MessageID:      11,
		UserID:         1,
		RunID:          "run_1",
		Kind:           domainconversation.ContextArtifactToolResult,
		Content:        "evidence",
	}
	tests := map[string]func(*domainconversation.ContextArtifact){
		"conversation": func(item *domainconversation.ContextArtifact) { item.ConversationID = 0 },
		"message":      func(item *domainconversation.ContextArtifact) { item.MessageID = 0 },
		"user":         func(item *domainconversation.ContextArtifact) { item.UserID = 0 },
		"run":          func(item *domainconversation.ContextArtifact) { item.RunID = "  " },
	}
	for name, invalidate := range tests {
		t.Run(name, func(t *testing.T) {
			item := valid
			invalidate(&item)
			err := repo.CreateContextArtifacts(context.Background(), []domainconversation.ContextArtifact{item})
			if !errors.Is(err, repository.ErrInvalidInput) {
				t.Fatalf("CreateContextArtifacts() error = %v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestCreateContextArtifactsNormalizesRunOwner(t *testing.T) {
	db := openConversationRepositoryTestDB(t)
	if err := db.AutoMigrate(&model.ChatContextRecord{}); err != nil {
		t.Fatalf("migrate context records: %v", err)
	}
	repo := NewRepo(db)
	items := []domainconversation.ContextArtifact{{
		ConversationID: 7,
		MessageID:      11,
		UserID:         1,
		RunID:          "  run_1  ",
		Kind:           domainconversation.ContextArtifactToolResult,
		Content:        "normalized evidence",
	}}

	if err := repo.CreateContextArtifacts(context.Background(), items); err != nil {
		t.Fatalf("CreateContextArtifacts() error = %v", err)
	}
	if items[0].RunID != "run_1" {
		t.Fatalf("artifact run id = %q, want normalized run_1", items[0].RunID)
	}
}

func TestListRecentContextArtifactsFiltersBranchBeforeLimit(t *testing.T) {
	db := openConversationRepositoryTestDB(t)
	if err := db.AutoMigrate(&model.Message{}, &model.ChatContextRecord{}); err != nil {
		t.Fatalf("migrate context records: %v", err)
	}
	repo := NewRepo(db)
	ctx := context.Background()
	rootMessageID := uint(1)
	activeOwnerID := uint(10)
	leafMessageID := uint(12)
	branchMessages := []model.Message{
		{
			BaseModel:      model.BaseModel{ID: rootMessageID},
			ConversationID: 7,
			UserID:         1,
			PublicID:       "msg_branch_root",
			Role:           "user",
			Status:         "success",
		},
		{
			BaseModel:       model.BaseModel{ID: activeOwnerID},
			ConversationID:  7,
			UserID:          1,
			PublicID:        "msg_artifact_owner",
			ParentMessageID: &rootMessageID,
			RunID:           "run_active",
			Role:            "assistant",
			Status:          "success",
		},
		{
			BaseModel:       model.BaseModel{ID: leafMessageID},
			ConversationID:  7,
			UserID:          1,
			PublicID:        "msg_branch_leaf",
			ParentMessageID: &activeOwnerID,
			Role:            "user",
			Status:          "pending",
		},
	}
	for index := 0; index < 31; index++ {
		branchMessages = append(branchMessages, model.Message{
			BaseModel:       model.BaseModel{ID: uint(100 + index)},
			ConversationID:  7,
			UserID:          1,
			PublicID:        fmt.Sprintf("msg_sibling_%d", index),
			ParentMessageID: &rootMessageID,
			Role:            "assistant",
			Status:          "success",
		})
	}
	if err := db.Create(&branchMessages).Error; err != nil {
		t.Fatalf("create branch messages: %v", err)
	}

	items := []model.ChatContextRecord{
		{
			RecordType:     chatContextRecordArtifact,
			ConversationID: 7,
			MessageID:      activeOwnerID,
			UserID:         1,
			RunID:          "run_active",
			Kind:           string(domainconversation.ContextArtifactToolResult),
			SourceType:     "tool_call",
			SourceID:       "active",
			Content:        "active branch evidence",
		},
		{
			RecordType:     chatContextRecordArtifact,
			ConversationID: 7,
			MessageID:      rootMessageID,
			UserID:         1,
			Kind:           string(domainconversation.ContextArtifactToolResult),
			SourceType:     "tool_call",
			SourceID:       "user-owned",
			Content:        "legacy evidence with ambiguous branch ownership",
		},
		{
			RecordType:     chatContextRecordArtifact,
			ConversationID: 7,
			MessageID:      activeOwnerID,
			UserID:         1,
			RunID:          "run_wrong_owner",
			Kind:           string(domainconversation.ContextArtifactToolResult),
			SourceType:     "tool_call",
			SourceID:       "mismatched-run",
			Content:        "evidence must not borrow an unrelated assistant owner",
		},
	}
	for index := 0; index < 31; index++ {
		items = append(items, model.ChatContextRecord{
			RecordType:     chatContextRecordArtifact,
			ConversationID: 7,
			MessageID:      uint(100 + index),
			UserID:         1,
			Kind:           string(domainconversation.ContextArtifactToolResult),
			SourceType:     "tool_call",
			SourceID:       fmt.Sprintf("sibling-%d", index),
			Content:        "sibling branch evidence",
		})
	}
	if err := db.Create(&items).Error; err != nil {
		t.Fatalf("create context records: %v", err)
	}

	artifacts, err := repo.ListRecentContextArtifacts(ctx, repository.ContextArtifactListFilter{
		Scope: repository.HistoricalMessageScope{
			ConversationID: 7,
			UserID:         1,
			LeafMessageID:  leafMessageID,
		},
		Kinds: []domainconversation.ContextArtifactKind{domainconversation.ContextArtifactToolResult},
		Limit: 1,
	})
	if err != nil {
		t.Fatalf("ListRecentContextArtifacts() error = %v", err)
	}
	if len(artifacts) != 1 || artifacts[0].MessageID != activeOwnerID {
		t.Fatalf("expected active branch evidence before limit, got %#v", artifacts)
	}
}

func TestConversationProjectDefaultsRoundTripAndDelete(t *testing.T) {
	db := openConversationRepositoryTestDB(t)
	repo := NewRepo(db)
	ctx := context.Background()
	project := domainconversation.ConversationProject{
		UserID:            1,
		PublicID:          "project_defaults",
		Name:              "Project defaults",
		MCPDefaultMode:    domainconversation.ConversationProjectMCPDefaultModeCustom,
		DefaultMCPToolIDs: []uint{7, 3},
		DefaultSkillIDs:   []uint{11, 5},
		Status:            "active",
	}
	if err := repo.CreateConversationProject(ctx, &project); err != nil {
		t.Fatalf("CreateConversationProject() error = %v", err)
	}
	if !reflect.DeepEqual(project.DefaultMCPToolIDs, []uint{7, 3}) || !reflect.DeepEqual(project.DefaultSkillIDs, []uint{11, 5}) {
		t.Fatalf("created defaults = MCP %v Skills %v", project.DefaultMCPToolIDs, project.DefaultSkillIDs)
	}

	loaded, err := repo.GetConversationProjectByPublicID(ctx, 1, project.PublicID)
	if err != nil {
		t.Fatalf("GetConversationProjectByPublicID() error = %v", err)
	}
	if loaded.MCPDefaultMode != domainconversation.ConversationProjectMCPDefaultModeCustom ||
		!reflect.DeepEqual(loaded.DefaultMCPToolIDs, []uint{7, 3}) ||
		!reflect.DeepEqual(loaded.DefaultSkillIDs, []uint{11, 5}) {
		t.Fatalf("loaded project defaults = %#v", loaded)
	}

	nextMCPToolIDs := []uint{}
	nextSkillIDs := []uint{5}
	inheritMode := domainconversation.ConversationProjectMCPDefaultModeInherit
	updated, err := repo.UpdateConversationProjectMetadataByPublicID(ctx, 1, project.PublicID, domainconversation.ConversationProjectPatch{
		MCPDefaultMode:    &inheritMode,
		DefaultMCPToolIDs: &nextMCPToolIDs,
		DefaultSkillIDs:   &nextSkillIDs,
	})
	if err != nil {
		t.Fatalf("UpdateConversationProjectMetadataByPublicID() error = %v", err)
	}
	if updated.MCPDefaultMode != inheritMode || len(updated.DefaultMCPToolIDs) != 0 || !reflect.DeepEqual(updated.DefaultSkillIDs, nextSkillIDs) {
		t.Fatalf("updated project defaults = %#v", updated)
	}

	if _, err = repo.DeleteConversationProjectByPublicID(ctx, 1, project.PublicID, false, false); err != nil {
		t.Fatalf("DeleteConversationProjectByPublicID() error = %v", err)
	}
	var associationCount int64
	if err = db.Model(&model.ConversationProjectMCPTool{}).Where("project_id = ?", project.ID).Count(&associationCount).Error; err != nil {
		t.Fatalf("count project MCP associations: %v", err)
	}
	if associationCount != 0 {
		t.Fatalf("project MCP association count = %d, want 0", associationCount)
	}
	if err = db.Model(&model.ConversationProjectSkill{}).Where("project_id = ?", project.ID).Count(&associationCount).Error; err != nil {
		t.Fatalf("count project Skill associations: %v", err)
	}
	if associationCount != 0 {
		t.Fatalf("project Skill association count = %d, want 0", associationCount)
	}
}

func TestListConversationEventLogsHydratesRunRouteSnapshot(t *testing.T) {
	db := openConversationRepositoryTestDB(t)
	repo := NewRepo(db)
	ctx := context.Background()
	now := time.Now()

	run := model.ConversationRun{
		RunID:             "run_with_route",
		UserID:            1,
		ConversationID:    2,
		ProviderProtocol:  "openai_responses",
		UpstreamName:      "OpenAI Official",
		PlatformModelName: "gpt-5.5",
		RoutedBindingCode: "binding_openai",
		UpstreamModelName: "gpt-5.5-pro",
		Status:            "error",
		StartedAt:         now,
	}
	if err := db.Create(&run).Error; err != nil {
		t.Fatalf("create conversation run: %v", err)
	}

	events := []model.ChatRunEvent{
		{
			ConversationID: 2,
			UserID:         1,
			RunID:          run.RunID,
			EventScope:     "trace_event",
			EventID:        "event_with_route",
			EventType:      "error",
			Status:         "error",
			StartedAt:      now,
		},
		{
			ConversationID: 2,
			UserID:         1,
			RunID:          "run_before_route",
			EventScope:     "trace_event",
			EventID:        "event_without_route",
			EventType:      "error",
			Status:         "error",
			StartedAt:      now,
		},
	}
	if err := db.Create(&events).Error; err != nil {
		t.Fatalf("create conversation events: %v", err)
	}

	items, total, err := repo.ListConversationEventLogs(ctx, repository.ConversationEventLogListFilter{}, 0, 10)
	if err != nil {
		t.Fatalf("ListConversationEventLogs() error = %v", err)
	}
	if total != 2 || len(items) != 2 {
		t.Fatalf("got total=%d len=%d, want 2", total, len(items))
	}
	itemsByRunID := make(map[string]domainconversation.EventLog, len(items))
	for _, item := range items {
		itemsByRunID[item.RunID] = item
	}
	withRoute := itemsByRunID[run.RunID]
	if withRoute.UpstreamName != run.UpstreamName ||
		withRoute.ProviderProtocol != run.ProviderProtocol ||
		withRoute.PlatformModelName != run.PlatformModelName ||
		withRoute.RoutedBindingCode != run.RoutedBindingCode ||
		withRoute.UpstreamModelName != run.UpstreamModelName {
		t.Fatalf("route snapshot = %#v, want run snapshot %#v", withRoute, run)
	}
	withoutRoute := itemsByRunID["run_before_route"]
	if withoutRoute.UpstreamName != "" || withoutRoute.ProviderProtocol != "" || withoutRoute.UpstreamModelName != "" {
		t.Fatalf("unexpected route snapshot for unmatched run: %#v", withoutRoute)
	}
}

func TestConversationEventLogListAndDetailBoundPayloads(t *testing.T) {
	db := openConversationRepositoryTestDB(t)
	repo := NewRepo(db)
	ctx := context.Background()
	now := time.Now()
	largePayload := strings.Repeat("x", maxConversationEventDetailPayloadBytes+1)
	events := []model.ChatRunEvent{
		{
			ConversationID:  1,
			UserID:          1,
			RunID:           "run_normal_payload",
			EventScope:      "trace_event",
			EventID:         "event_normal_payload",
			EventType:       "error",
			Status:          "error",
			ContentMarkdown: "request failed after upload",
			PayloadJSON:     `{"error":"上游不可用"}`,
			InputJSON:       `{"input":true}`,
			OutputJSON:      `{"output":true}`,
			ErrorJSON:       `{"code":"upstream_unavailable"}`,
			StartedAt:       now,
		},
		{
			ConversationID: 1,
			UserID:         1,
			RunID:          "run_large_payload",
			EventScope:     "trace_event",
			EventID:        "event_large_payload",
			EventType:      "error",
			Status:         "error",
			PayloadJSON:    largePayload,
			StartedAt:      now,
		},
	}
	if err := db.Create(&events).Error; err != nil {
		t.Fatalf("create conversation events: %v", err)
	}

	items, total, err := repo.ListConversationEventLogs(ctx, repository.ConversationEventLogListFilter{}, 0, 10)
	if err != nil {
		t.Fatalf("ListConversationEventLogs() error = %v", err)
	}
	if total != 2 || len(items) != 2 {
		t.Fatalf("got total=%d len=%d, want 2", total, len(items))
	}
	itemsByRunID := make(map[string]domainconversation.EventLog, len(items))
	for _, item := range items {
		itemsByRunID[item.RunID] = item
		if item.ContentMarkdown != "" || item.PayloadJSON != "" || item.InputJSON != "" || item.OutputJSON != "" || item.ErrorJSON != "" {
			t.Fatalf("list item contains detail payloads: %#v", item)
		}
	}
	if got := itemsByRunID["run_normal_payload"].PayloadSizeBytes; got != int64(len(events[0].PayloadJSON)) {
		t.Fatalf("normal payload size = %d, want %d", got, len(events[0].PayloadJSON))
	}
	if itemsByRunID["run_normal_payload"].PayloadOmitted {
		t.Fatal("normal list payload should not be marked omitted")
	}
	if got := itemsByRunID["run_large_payload"].PayloadSizeBytes; got != int64(len(largePayload)) {
		t.Fatalf("large payload size = %d, want %d", got, len(largePayload))
	}
	if !itemsByRunID["run_large_payload"].PayloadOmitted {
		t.Fatal("large list payload should be marked omitted")
	}

	normalDetail, err := repo.GetConversationEventLog(ctx, events[0].ID)
	if err != nil {
		t.Fatalf("GetConversationEventLog(normal) error = %v", err)
	}
	if normalDetail.ContentMarkdown != events[0].ContentMarkdown ||
		normalDetail.PayloadJSON != events[0].PayloadJSON ||
		normalDetail.InputJSON != events[0].InputJSON ||
		normalDetail.OutputJSON != events[0].OutputJSON ||
		normalDetail.ErrorJSON != events[0].ErrorJSON ||
		normalDetail.PayloadOmitted {
		t.Fatalf("normal detail = %#v", normalDetail)
	}

	largeDetail, err := repo.GetConversationEventLog(ctx, events[1].ID)
	if err != nil {
		t.Fatalf("GetConversationEventLog(large) error = %v", err)
	}
	if largeDetail.PayloadJSON != "" || !largeDetail.PayloadOmitted {
		t.Fatalf("large detail should omit payload, got %#v", largeDetail)
	}
	if largeDetail.PayloadSizeBytes != int64(len(largePayload)) {
		t.Fatalf("large detail payload size = %d, want %d", largeDetail.PayloadSizeBytes, len(largePayload))
	}
}

func TestConversationMessageTraceReadsBoundPayloads(t *testing.T) {
	db := openConversationRepositoryTestDB(t)
	repo := NewRepo(db)
	ctx := context.Background()
	now := time.Now()
	largePayload := strings.Repeat("x", maxConversationEventDetailPayloadBytes+1)
	items := []model.ChatRunEvent{
		{
			MessageID:       11,
			RunID:           "run_trace_block_large",
			EventScope:      "trace_block",
			EventID:         "trace_block_large",
			EventType:       "process",
			ContentMarkdown: "处理失败",
			PayloadJSON:     largePayload,
			StartedAt:       now,
		},
		{
			MessageID:   11,
			RunID:       "run_trace_event_large",
			EventScope:  "trace_event",
			EventID:     "trace_event_large",
			EventType:   "error",
			PayloadJSON: largePayload,
			StartedAt:   now,
		},
	}
	if err := db.Create(&items).Error; err != nil {
		t.Fatalf("create trace events: %v", err)
	}

	blocks, err := repo.ListConversationMessageTracesByMessageIDs(ctx, []uint{11})
	if err != nil {
		t.Fatalf("list message traces: %v", err)
	}
	if len(blocks) != 1 || blocks[0].PayloadJSON != "" || blocks[0].ContentMarkdown != "处理失败" {
		t.Fatalf("large trace block was not safely loaded: %#v", blocks)
	}

	events, err := repo.ListConversationMessageTraceEventsByMessageIDs(ctx, []uint{11})
	if err != nil {
		t.Fatalf("list trace events: %v", err)
	}
	if len(events) != 1 || events[0].PayloadJSON != "" {
		t.Fatalf("large trace event was not safely loaded: %#v", events)
	}
}

func TestListMessagesBeforeIDReturnsPreviousWindowAscending(t *testing.T) {
	db := openConversationRepositoryTestDB(t)
	repo := NewRepo(db)
	ctx := context.Background()

	conversation := model.Conversation{
		UserID:     1,
		PublicID:   "conv_before",
		Title:      "before window",
		LabelsJSON: "[]",
		SessionKey: "session_before",
		Status:     "active",
	}
	if err := db.Create(&conversation).Error; err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	messages := make([]model.Message, 0, 5)
	var parentID *uint
	for index := 1; index <= 5; index++ {
		message := model.Message{
			ConversationID:  conversation.ID,
			UserID:          1,
			PublicID:        fmt.Sprintf("msg_%d", index),
			ParentMessageID: parentID,
			Role:            "user",
			ContentType:     "text",
			Content:         fmt.Sprintf("message %d", index),
			BranchReason:    "default",
			Status:          "success",
		}
		if err := db.Create(&message).Error; err != nil {
			t.Fatalf("create message %d: %v", index, err)
		}
		messages = append(messages, message)
		nextParentID := message.ID
		parentID = &nextParentID
	}

	got, total, err := repo.ListMessagesBeforeID(ctx, conversation.ID, messages[4].ID, 2)
	if err != nil {
		t.Fatalf("ListMessagesBeforeID() error = %v", err)
	}
	if total != int64(len(messages)) {
		t.Fatalf("total = %d, want %d", total, len(messages))
	}
	if len(got) != 2 || got[0].PublicID != "msg_3" || got[1].PublicID != "msg_4" {
		t.Fatalf("unexpected previous window: %#v", got)
	}
	if got[1].ParentPublicID != "msg_3" {
		t.Fatalf("expected parent public id hydrated, got %q", got[1].ParentPublicID)
	}
}

func TestListMessageAncestorsUntilStopsAtBoundary(t *testing.T) {
	db := openConversationRepositoryTestDB(t)
	repo := NewRepo(db)
	ctx := context.Background()

	conversation := model.Conversation{
		UserID:     1,
		PublicID:   "conv_ancestors_until",
		Title:      "ancestors until",
		LabelsJSON: "[]",
		SessionKey: "session_ancestors_until",
		Status:     "active",
	}
	if err := db.Create(&conversation).Error; err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	messages := make([]model.Message, 0, 6)
	var parentID *uint
	for index := 1; index <= 6; index++ {
		message := model.Message{
			ConversationID:  conversation.ID,
			UserID:          1,
			PublicID:        fmt.Sprintf("msg_%d", index),
			ParentMessageID: parentID,
			Role:            "user",
			ContentType:     "text",
			Content:         fmt.Sprintf("message %d", index),
			BranchReason:    "default",
			Status:          "success",
		}
		if err := db.Create(&message).Error; err != nil {
			t.Fatalf("create message %d: %v", index, err)
		}
		messages = append(messages, message)
		nextParentID := message.ID
		parentID = &nextParentID
	}

	got, found, err := repo.ListMessageAncestorsUntil(ctx, conversation.ID, messages[5].ID, messages[2].ID, 10)
	if err != nil {
		t.Fatalf("ListMessageAncestorsUntil() error = %v", err)
	}
	if !found {
		t.Fatal("expected boundary to be found")
	}
	if len(got) != 4 {
		t.Fatalf("expected boundary through leaf, got %#v", got)
	}
	if got[0].PublicID != "msg_3" || got[len(got)-1].PublicID != "msg_6" {
		t.Fatalf("expected msg_3..msg_6, got %#v", got)
	}
	if got[0].ParentPublicID != "msg_2" {
		t.Fatalf("expected boundary parent public id hydrated, got %q", got[0].ParentPublicID)
	}
}

func TestListMessageAncestorsUntilReportsMissingBoundary(t *testing.T) {
	db := openConversationRepositoryTestDB(t)
	repo := NewRepo(db)
	ctx := context.Background()

	conversation := model.Conversation{
		UserID:     1,
		PublicID:   "conv_missing_boundary",
		Title:      "missing boundary",
		LabelsJSON: "[]",
		SessionKey: "session_missing_boundary",
		Status:     "active",
	}
	if err := db.Create(&conversation).Error; err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	message := model.Message{
		ConversationID: conversation.ID,
		UserID:         1,
		PublicID:       "msg_1",
		Role:           "user",
		ContentType:    "text",
		Content:        "message 1",
		BranchReason:   "default",
		Status:         "success",
	}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create message: %v", err)
	}

	got, found, err := repo.ListMessageAncestorsUntil(ctx, conversation.ID, message.ID, message.ID+100, 10)
	if err != nil {
		t.Fatalf("ListMessageAncestorsUntil() error = %v", err)
	}
	if found {
		t.Fatal("expected boundary to be missing")
	}
	if len(got) != 1 || got[0].PublicID != "msg_1" {
		t.Fatalf("expected available ancestor path, got %#v", got)
	}
}

// 祖先链走的是手写 CTE，与 GetMessageByID 的常规 GORM 查询是两条取数路径。
// 这里逐字段比对两者结果，确保 CTE 不会丢列——曾因漏掉 reasoning_content 导致推理回传失效。
// 注意覆盖边界：比对的是 domain.Message，因此只能守住会映射进领域模型的列；
// 未进入领域模型的列（如 is_compacted）不在此测试范围内。
func TestListMessageAncestorsMatchesFullColumnLoad(t *testing.T) {
	db := openConversationRepositoryTestDB(t)
	repo := NewRepo(db)
	ctx := context.Background()

	conversation := model.Conversation{
		UserID:     1,
		PublicID:   "conv_ancestors_columns",
		Title:      "ancestors columns",
		LabelsJSON: "[]",
		SessionKey: "session_ancestors_columns",
		Status:     "active",
	}
	if err := db.Create(&conversation).Error; err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	root := model.Message{
		ConversationID: conversation.ID,
		UserID:         1,
		PublicID:       "msg_columns_root",
		Role:           "user",
		ContentType:    "text",
		Content:        "root",
		BranchReason:   "default",
		Status:         "success",
	}
	if err := db.Create(&root).Error; err != nil {
		t.Fatalf("create root message: %v", err)
	}

	editedAt := time.Now().UTC().Truncate(time.Second)
	sourceID := root.ID
	// 所有可空/可选列都填非零值，任何一列被 CTE 丢弃都会在比对中暴露。
	leaf := model.Message{
		ConversationID:   conversation.ID,
		UserID:           1,
		PublicID:         "msg_columns_leaf",
		ParentMessageID:  &root.ID,
		RunID:            "run_columns",
		Role:             "assistant",
		ContentType:      "text",
		Content:          "leaf",
		ReasoningContent: "historical reasoning",
		BranchReason:     "retry",
		SourceMessageID:  &sourceID,
		TokenUsage:       321,
		InputTokens:      111,
		OutputTokens:     222,
		CacheReadTokens:  33,
		CacheWriteTokens: 44,
		ReasoningTokens:  125,
		LatencyMS:        987,
		Status:           "success",
		ErrorCode:        "none",
		ErrorMessage:     "no error",
		IsCompacted:      true,
		EditedAt:         &editedAt,
	}
	if err := db.Create(&leaf).Error; err != nil {
		t.Fatalf("create leaf message: %v", err)
	}

	want, err := repo.GetMessageByID(ctx, conversation.ID, leaf.ID)
	if err != nil {
		t.Fatalf("GetMessageByID() error = %v", err)
	}
	if want.ReasoningContent == "" {
		t.Fatal("baseline load lost reasoning content")
	}

	ancestors, err := repo.ListMessageAncestors(ctx, conversation.ID, leaf.ID, 10)
	if err != nil {
		t.Fatalf("ListMessageAncestors() error = %v", err)
	}
	if len(ancestors) != 2 {
		t.Fatalf("expected root and leaf, got %d", len(ancestors))
	}
	if !reflect.DeepEqual(ancestors[1], *want) {
		t.Fatalf("ListMessageAncestors dropped columns:\n cte = %#v\nfull = %#v", ancestors[1], *want)
	}

	until, found, err := repo.ListMessageAncestorsUntil(ctx, conversation.ID, leaf.ID, root.ID, 10)
	if err != nil {
		t.Fatalf("ListMessageAncestorsUntil() error = %v", err)
	}
	if !found {
		t.Fatal("expected boundary to be found")
	}
	if len(until) != 2 {
		t.Fatalf("expected root and leaf, got %d", len(until))
	}
	if !reflect.DeepEqual(until[1], *want) {
		t.Fatalf("ListMessageAncestorsUntil dropped columns:\n cte = %#v\nfull = %#v", until[1], *want)
	}
}

// 祖先链加载必须保留 reasoning_content，否则「回传推理上下文」在后续轮次拿不到历史推理。
func TestListMessageAncestorsPreservesReasoningContent(t *testing.T) {
	db := openConversationRepositoryTestDB(t)
	repo := NewRepo(db)
	ctx := context.Background()

	conversation := model.Conversation{
		UserID:     1,
		PublicID:   "conv_ancestors_reasoning",
		Title:      "ancestors reasoning",
		LabelsJSON: "[]",
		SessionKey: "session_ancestors_reasoning",
		Status:     "active",
	}
	if err := db.Create(&conversation).Error; err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	var parentID *uint
	messages := make([]model.Message, 0, 4)
	for index := 1; index <= 4; index++ {
		role := "user"
		reasoning := ""
		if index%2 == 0 {
			role = "assistant"
			reasoning = fmt.Sprintf("reasoning %d", index)
		}
		message := model.Message{
			ConversationID:   conversation.ID,
			UserID:           1,
			PublicID:         fmt.Sprintf("msg_reasoning_%d", index),
			ParentMessageID:  parentID,
			Role:             role,
			ContentType:      "text",
			Content:          fmt.Sprintf("message %d", index),
			ReasoningContent: reasoning,
			BranchReason:     "default",
			Status:           "success",
		}
		if err := db.Create(&message).Error; err != nil {
			t.Fatalf("create message %d: %v", index, err)
		}
		messages = append(messages, message)
		nextParentID := message.ID
		parentID = &nextParentID
	}

	leafID := messages[len(messages)-1].ID
	assertReasoning := func(t *testing.T, method string, got []domainconversation.Message) {
		t.Helper()
		if len(got) != len(messages) {
			t.Fatalf("%s: expected %d ancestors, got %d", method, len(messages), len(got))
		}
		for index, item := range got {
			want := messages[index].ReasoningContent
			if item.ReasoningContent != want {
				t.Fatalf("%s: ancestor %d reasoning content = %q, want %q", method, index, item.ReasoningContent, want)
			}
		}
	}

	ancestors, err := repo.ListMessageAncestors(ctx, conversation.ID, leafID, 10)
	if err != nil {
		t.Fatalf("ListMessageAncestors() error = %v", err)
	}
	assertReasoning(t, "ListMessageAncestors", ancestors)

	until, found, err := repo.ListMessageAncestorsUntil(ctx, conversation.ID, leafID, messages[0].ID, 10)
	if err != nil {
		t.Fatalf("ListMessageAncestorsUntil() error = %v", err)
	}
	if !found {
		t.Fatal("expected boundary to be found")
	}
	assertReasoning(t, "ListMessageAncestorsUntil", until)
}

func TestUpdateAssistantMessageCompletionPersistsReasoningContent(t *testing.T) {
	db := openConversationRepositoryTestDB(t)
	repo := NewRepo(db)
	ctx := context.Background()

	conversation := model.Conversation{
		UserID:     1,
		PublicID:   "conv_reasoning_completion",
		Title:      "reasoning completion",
		LabelsJSON: "[]",
		SessionKey: "session_reasoning_completion",
		Status:     "active",
	}
	if err := db.Create(&conversation).Error; err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	message := model.Message{
		ConversationID: conversation.ID,
		UserID:         1,
		PublicID:       "msg_reasoning_completion",
		Role:           "assistant",
		ContentType:    "text",
		Content:        "",
		BranchReason:   "default",
		Status:         "pending",
	}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create message: %v", err)
	}

	err := repo.UpdateAssistantMessageCompletion(ctx, message.ID, repository.AssistantMessageCompletionUpdate{
		ContentType:      "text",
		Content:          "final answer",
		ReasoningContent: "stored reasoning",
		Status:           "success",
	})
	if err != nil {
		t.Fatalf("UpdateAssistantMessageCompletion() error = %v", err)
	}

	got, err := repo.GetMessageByID(ctx, conversation.ID, message.ID)
	if err != nil {
		t.Fatalf("GetMessageByID() error = %v", err)
	}
	if got.Content != "final answer" || got.ReasoningContent != "stored reasoning" {
		t.Fatalf("unexpected completed message: %#v", got)
	}
}

func TestUpdateConversationMetadataTrimsTitle(t *testing.T) {
	db := openConversationRepositoryTestDB(t)
	repo := NewRepo(db)
	ctx := context.Background()

	conversation := model.Conversation{
		UserID:     1,
		PublicID:   "conv_metadata_trim",
		Title:      " 新对话 ",
		LabelsJSON: "[]",
		SessionKey: "session_metadata_trim",
		Status:     "active",
	}
	if err := db.Create(&conversation).Error; err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	updated, err := repo.UpdateConversationMetadata(ctx, conversation.ID, repository.ConversationMetadataPatch{
		Title: "trimmed title",
	})
	if err != nil {
		t.Fatalf("UpdateConversationMetadata() error = %v", err)
	}
	if updated.Title != "trimmed title" {
		t.Fatalf("updated title = %q, want %q", updated.Title, "trimmed title")
	}
}

func TestUpdateConversationLabelsAppliesGeneratedLabelsWhenEligible(t *testing.T) {
	db := openConversationRepositoryTestDB(t)
	repo := NewRepo(db)
	conversation := model.Conversation{
		PublicID:   "generated-label-eligible",
		UserID:     1,
		Title:      "已有标题",
		LabelsJSON: `[]`,
		SessionKey: "generated-label-eligible-session",
		Status:     "active",
	}
	if err := db.Create(&conversation).Error; err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	updated, applied, err := repo.SetGeneratedConversationLabelsIfEligible(context.Background(), conversation.ID, `["自动标签"]`)
	if err != nil {
		t.Fatalf("SetGeneratedConversationLabelsIfEligible() error = %v", err)
	}
	if !applied || updated.LabelsJSON != `["自动标签"]` {
		t.Fatalf("generated labels were not applied: applied=%v labels=%q", applied, updated.LabelsJSON)
	}
}

func TestUpdateConversationLabelsByPublicIDIsUserScopedAndMarksManualManagement(t *testing.T) {
	db := openConversationRepositoryTestDB(t)
	repo := NewRepo(db)
	conversation := model.Conversation{
		PublicID:   "manual-label-user-scope",
		UserID:     1,
		Title:      "已有标题",
		LabelsJSON: `[]`,
		SessionKey: "manual-label-user-scope-session",
		Status:     "active",
	}
	if err := db.Create(&conversation).Error; err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	if _, err := repo.UpdateConversationLabelsByPublicID(context.Background(), 2, conversation.PublicID, `["越权标签"]`); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("expected other user update to return not found, got %v", err)
	}
	updated, err := repo.UpdateConversationLabelsByPublicID(context.Background(), 1, conversation.PublicID, `["手动标签"]`)
	if err != nil {
		t.Fatalf("UpdateConversationLabelsByPublicID() error = %v", err)
	}
	if updated.LabelsJSON != `["手动标签"]` || !updated.LabelsManuallyManaged {
		t.Fatalf("manual labels were not persisted correctly: %#v", updated)
	}
}

func TestUpdateConversationLabelsGeneratedLabelsDoNotOverwriteManualLabels(t *testing.T) {
	db := openConversationRepositoryTestDB(t)
	repo := NewRepo(db)
	conversation := model.Conversation{
		PublicID:              "generated-label-race",
		UserID:                1,
		Title:                 "已有标题",
		LabelsJSON:            `["手动标签"]`,
		LabelsManuallyManaged: true,
		SessionKey:            "generated-label-race-session",
		Status:                "active",
	}
	if err := db.Create(&conversation).Error; err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	var persisted model.Conversation
	if err := db.First(&persisted, conversation.ID).Error; err != nil {
		t.Fatalf("reload conversation: %v", err)
	}

	updated, applied, err := repo.SetGeneratedConversationLabelsIfEligible(context.Background(), conversation.ID, `["自动标签"]`)
	if err != nil {
		t.Fatalf("SetGeneratedConversationLabelsIfEligible() error = %v", err)
	}
	if applied {
		t.Fatal("generated labels update unexpectedly applied")
	}
	if updated.LabelsJSON != `["手动标签"]` {
		t.Fatalf("generated labels overwrote manual labels: %q", updated.LabelsJSON)
	}
	if !updated.UpdatedAt.Equal(persisted.UpdatedAt) {
		t.Fatalf("skipped generated labels changed updated_at: got %v, want %v", updated.UpdatedAt, persisted.UpdatedAt)
	}
}

func TestUpdateConversationLabelsGeneratedLabelsDoNotRestoreManuallyClearedLabels(t *testing.T) {
	db := openConversationRepositoryTestDB(t)
	repo := NewRepo(db)
	conversation := model.Conversation{
		PublicID:              "generated-label-manual-clear",
		UserID:                1,
		Title:                 "已有标题",
		LabelsJSON:            `[]`,
		LabelsManuallyManaged: true,
		SessionKey:            "gen-label-manual-clear",
		Status:                "active",
	}
	if err := db.Create(&conversation).Error; err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	var persisted model.Conversation
	if err := db.First(&persisted, conversation.ID).Error; err != nil {
		t.Fatalf("reload conversation: %v", err)
	}

	updated, applied, err := repo.SetGeneratedConversationLabelsIfEligible(context.Background(), conversation.ID, `["自动标签"]`)
	if err != nil {
		t.Fatalf("SetGeneratedConversationLabelsIfEligible() error = %v", err)
	}
	if applied {
		t.Fatal("generated labels update unexpectedly applied")
	}
	if updated.LabelsJSON != `[]` {
		t.Fatalf("generated labels restored manually cleared labels: %q", updated.LabelsJSON)
	}
	if !updated.UpdatedAt.Equal(persisted.UpdatedAt) {
		t.Fatalf("skipped generated labels changed updated_at: got %v, want %v", updated.UpdatedAt, persisted.UpdatedAt)
	}
}

func TestUpdateConversationMetadataCanReplaceAutomaticFallbackTitle(t *testing.T) {
	db := openConversationRepositoryTestDB(t)
	repo := NewRepo(db)
	ctx := context.Background()

	conversation := model.Conversation{
		UserID:     1,
		PublicID:   "conv_metadata_fallback",
		Title:      "画一张城市夜景",
		LabelsJSON: "[]",
		SessionKey: "session_metadata_fallback",
		Status:     "active",
	}
	if err := db.Create(&conversation).Error; err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	updated, err := repo.UpdateConversationMetadata(ctx, conversation.ID, repository.ConversationMetadataPatch{
		Title:             "城市夜景图像生成",
		ReplaceableTitles: []string{"画一张城市夜景"},
	})
	if err != nil {
		t.Fatalf("UpdateConversationMetadata() error = %v", err)
	}
	if updated.Title != "城市夜景图像生成" {
		t.Fatalf("updated title = %q, want %q", updated.Title, "城市夜景图像生成")
	}

	if err := db.Model(&model.Conversation{}).Where("id = ?", conversation.ID).Update("title", "手动标题").Error; err != nil {
		t.Fatalf("set manual title: %v", err)
	}
	updated, err = repo.UpdateConversationMetadata(ctx, conversation.ID, repository.ConversationMetadataPatch{
		Title:             "不应覆盖",
		ReplaceableTitles: []string{"画一张城市夜景"},
	})
	if err != nil {
		t.Fatalf("UpdateConversationMetadata() error = %v", err)
	}
	if updated.Title != "手动标题" {
		t.Fatalf("manual title was overwritten: got %q", updated.Title)
	}
}

func TestListConversationsByUserSearchesMetadataProjectsAndMessages(t *testing.T) {
	db := openConversationRepositoryTestDB(t)
	repo := NewRepo(db)
	ctx := context.Background()

	project := model.ConversationProject{
		UserID:      1,
		PublicID:    "proj_research",
		Name:        "Research Notes",
		Description: "knowledge base",
		Status:      "active",
	}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}

	projectConversation := model.Conversation{
		UserID:        1,
		ProjectID:     &project.ID,
		PublicID:      "conv_project_search",
		Title:         "Project conversation",
		LabelsJSON:    "[]",
		Model:         "gpt-test",
		Provider:      "openai",
		ExecutionType: domainconversation.ExecutionTypeCloud,
		SessionKey:    "session_project_search",
		Status:        "active",
	}
	titleConversation := model.Conversation{
		UserID:        1,
		PublicID:      "conv_title_search",
		Title:         "Quarterly Budget",
		LabelsJSON:    `["finance"]`,
		Model:         "claude-test",
		Provider:      "anthropic",
		ExecutionType: domainconversation.ExecutionTypeCloud,
		SessionKey:    "session_title_search",
		Status:        "active",
	}
	messageConversation := model.Conversation{
		UserID:        1,
		PublicID:      "conv_message_search",
		Title:         "Ordinary chat",
		LabelsJSON:    "[]",
		Model:         "gemini-test",
		Provider:      "gemini",
		ExecutionType: domainconversation.ExecutionTypeCloud,
		SessionKey:    "session_message_search",
		Status:        "active",
	}
	toolOnlyConversation := model.Conversation{
		UserID:        1,
		PublicID:      "conv_tool_only_search",
		Title:         "Tool output",
		LabelsJSON:    "[]",
		Model:         "gpt-test",
		Provider:      "openai",
		ExecutionType: domainconversation.ExecutionTypeCloud,
		SessionKey:    "session_tool_only_search",
		Status:        "active",
	}
	wildcardConversation := model.Conversation{
		UserID:        1,
		PublicID:      "conv_literal_wildcard_search",
		Title:         "Progress 100%",
		LabelsJSON:    "[]",
		Model:         "gpt-test",
		Provider:      "openai",
		ExecutionType: domainconversation.ExecutionTypeCloud,
		SessionKey:    "session_literal_wildcard_search",
		Status:        "active",
	}
	otherUserConversation := model.Conversation{
		UserID:        2,
		PublicID:      "conv_other_user",
		Title:         "Private Budget",
		LabelsJSON:    "[]",
		Model:         "gpt-test",
		Provider:      "openai",
		ExecutionType: domainconversation.ExecutionTypeCloud,
		SessionKey:    "session_other_user",
		Status:        "active",
	}
	for _, conversation := range []model.Conversation{
		projectConversation,
		titleConversation,
		messageConversation,
		toolOnlyConversation,
		wildcardConversation,
		otherUserConversation,
	} {
		if err := db.Create(&conversation).Error; err != nil {
			t.Fatalf("create conversation %q: %v", conversation.PublicID, err)
		}
	}

	var messageTarget model.Conversation
	if err := db.Where("public_id = ?", "conv_message_search").First(&messageTarget).Error; err != nil {
		t.Fatalf("load message target: %v", err)
	}
	if err := db.Create(&model.Message{
		ConversationID: messageTarget.ID,
		UserID:         1,
		PublicID:       "msg_search",
		Role:           "user",
		ContentType:    "text",
		Content:        "The launch checklist mentions AuroraKeyword",
		BranchReason:   "default",
		Status:         "success",
	}).Error; err != nil {
		t.Fatalf("create message: %v", err)
	}
	var toolOnlyTarget model.Conversation
	if err := db.Where("public_id = ?", "conv_tool_only_search").First(&toolOnlyTarget).Error; err != nil {
		t.Fatalf("load tool-only target: %v", err)
	}
	if err := db.Create(&model.Message{
		ConversationID: toolOnlyTarget.ID,
		UserID:         1,
		PublicID:       "msg_tool_only_search",
		Role:           "tool",
		ContentType:    "text",
		Content:        "InternalToolOnlyKeyword",
		BranchReason:   "default",
		Status:         "success",
	}).Error; err != nil {
		t.Fatalf("create tool-only message: %v", err)
	}

	tests := []struct {
		name   string
		query  string
		wantID string
	}{
		{name: "title", query: "budget", wantID: "conv_title_search"},
		{name: "project", query: "research", wantID: "conv_project_search"},
		{name: "message", query: "aurorakeyword", wantID: "conv_message_search"},
		{name: "literal wildcard", query: "%", wantID: "conv_literal_wildcard_search"},
		{name: "tool messages are excluded", query: "internaltoolonlykeyword", wantID: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items, total, err := repo.ListConversationsByUser(ctx, 1, 0, 10, "active", "all", "all", "all", domainconversation.ExecutionTypeCloud, "", tt.query)
			if err != nil {
				t.Fatalf("ListConversationsByUser() error = %v", err)
			}
			if tt.wantID == "" {
				if total != 0 || len(items) != 0 {
					t.Fatalf("items = %#v, total = %d, want no results", items, total)
				}
				return
			}
			if total != 1 || len(items) != 1 || items[0].PublicID != tt.wantID {
				t.Fatalf("items = %#v, want %q", items, tt.wantID)
			}
		})
	}
}

func TestConversationExecutionScopesAreIsolated(t *testing.T) {
	db := openConversationRepositoryTestDB(t)
	repo := NewRepo(db)
	ctx := context.Background()
	project := model.ConversationProject{
		UserID: 1, PublicID: "proj_cloud_scope", Name: "Cloud", Status: "active",
	}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	conversations := []model.Conversation{
		{UserID: 1, PublicID: "conv_cloud_scope", Title: "Cloud", LabelsJSON: "[]", ExecutionType: domainconversation.ExecutionTypeCloud, SessionKey: "session_cloud_scope", Status: "active"},
		{UserID: 1, PublicID: "conv_device_a", Title: "Device A", LabelsJSON: "[]", ExecutionType: domainconversation.ExecutionTypeGateway, ExecutionDeviceID: "agd_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ExecutionWorkspaceID: "workspace_a", SessionKey: "session_device_a", Status: "active"},
		{UserID: 1, PublicID: "conv_device_b", Title: "Device B", LabelsJSON: "[]", ExecutionType: domainconversation.ExecutionTypeGateway, ExecutionDeviceID: "agd_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", ExecutionWorkspaceID: "workspace_b", SessionKey: "session_device_b", Status: "active"},
	}
	if err := db.Create(&conversations).Error; err != nil {
		t.Fatalf("create conversations: %v", err)
	}

	tests := []struct {
		name, executionType, deviceID, wantPublicID string
	}{
		{name: "cloud", executionType: domainconversation.ExecutionTypeCloud, wantPublicID: "conv_cloud_scope"},
		{name: "device a", executionType: domainconversation.ExecutionTypeGateway, deviceID: "agd_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", wantPublicID: "conv_device_a"},
		{name: "device b", executionType: domainconversation.ExecutionTypeGateway, deviceID: "agd_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", wantPublicID: "conv_device_b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items, total, err := repo.ListConversationsByUser(ctx, 1, 0, 10, "active", "all", "all", "all", tt.executionType, tt.deviceID, "")
			if err != nil {
				t.Fatalf("ListConversationsByUser() error = %v", err)
			}
			if total != 1 || len(items) != 1 || items[0].PublicID != tt.wantPublicID {
				t.Fatalf("items = %#v, total = %d, want only %q", items, total, tt.wantPublicID)
			}
		})
	}

	items, total, err := repo.ListConversationsByUser(
		ctx,
		1,
		0,
		10,
		"active",
		"all",
		"all",
		"workspace_a",
		domainconversation.ExecutionTypeGateway,
		"agd_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"",
	)
	if err != nil {
		t.Fatalf("ListConversationsByUser() workspace filter error = %v", err)
	}
	if total != 1 || len(items) != 1 || items[0].PublicID != "conv_device_a" {
		t.Fatalf("workspace filtered items = %#v, total = %d", items, total)
	}

	if _, err := repo.UpdateConversationProjectAssignmentByPublicID(ctx, 1, "conv_device_a", &project.ID); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("gateway project assignment error = %v, want ErrNotFound", err)
	}
}

func TestGatewayUnassignedListsOnlyHiddenRecentWorkspace(t *testing.T) {
	db := openConversationRepositoryTestDB(t)
	if err := db.AutoMigrate(&model.AgentDevice{}, &model.AgentWorkspace{}); err != nil {
		t.Fatal(err)
	}
	device := model.AgentDevice{PublicID: "agd_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", UserID: 1, Name: "desktop", Platform: "windows", PublicKey: []byte("key"), PublicKeyFingerprint: "fingerprint", CredentialVersion: 1, Status: "active", NextServerSeq: 1}
	if err := db.Create(&device).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	workspaces := []model.AgentWorkspace{
		{PublicID: "workspace-project", UserID: 1, DeviceID: device.ID, RuntimeProfileID: 1, Name: "Project", Status: "available", LastSeenAt: now},
		{PublicID: "workspace-recent", UserID: 1, DeviceID: device.ID, RuntimeProfileID: 1, Name: "Recent", Hidden: true, Status: "available", LastSeenAt: now},
	}
	if err := db.Create(&workspaces).Error; err != nil {
		t.Fatal(err)
	}
	items := []model.Conversation{
		{UserID: 1, PublicID: "conv_project", Title: "Project", LabelsJSON: "[]", ExecutionType: domainconversation.ExecutionTypeGateway, ExecutionDeviceID: device.PublicID, ExecutionWorkspaceID: "workspace-project", SessionKey: "session_project", Status: "active"},
		{UserID: 1, PublicID: "conv_recent", Title: "Recent", LabelsJSON: "[]", ExecutionType: domainconversation.ExecutionTypeGateway, ExecutionDeviceID: device.PublicID, ExecutionWorkspaceID: "workspace-recent", SessionKey: "session_recent", Status: "active"},
	}
	if err := db.Create(&items).Error; err != nil {
		t.Fatal(err)
	}
	got, total, err := NewRepo(db).ListConversationsByUser(context.Background(), 1, 0, 10, "active", "unstarred", "all", "unassigned", domainconversation.ExecutionTypeGateway, device.PublicID, "")
	if err != nil || total != 1 || len(got) != 1 || got[0].PublicID != "conv_recent" {
		t.Fatalf("recent conversations = %#v, total=%d, err=%v", got, total, err)
	}
}

func TestListConversationsForSearchReturnsOrderedWindowWithoutStatusFiltering(t *testing.T) {
	db := openConversationRepositoryTestDB(t)
	repo := NewRepo(db)
	ctx := context.Background()
	now := time.Now()

	items := []model.Conversation{
		{
			BaseModel:     model.BaseModel{UpdatedAt: now.Add(-2 * time.Hour)},
			UserID:        1,
			PublicID:      "conv_search_oldest",
			Title:         "Needle oldest",
			LabelsJSON:    "[]",
			Model:         "gpt-test",
			Provider:      "openai",
			ExecutionType: domainconversation.ExecutionTypeCloud,
			SessionKey:    "session_search_oldest",
			Status:        "active",
		},
		{
			BaseModel:     model.BaseModel{UpdatedAt: now.Add(-time.Hour)},
			UserID:        1,
			PublicID:      "conv_search_middle",
			Title:         "Needle middle",
			LabelsJSON:    "[]",
			Model:         "gpt-test",
			Provider:      "openai",
			ExecutionType: domainconversation.ExecutionTypeCloud,
			SessionKey:    "session_search_middle",
			Status:        "archived",
		},
		{
			BaseModel:     model.BaseModel{UpdatedAt: now},
			UserID:        1,
			PublicID:      "conv_search_latest",
			Title:         "Needle latest",
			LabelsJSON:    "[]",
			Model:         "gpt-test",
			Provider:      "openai",
			ExecutionType: domainconversation.ExecutionTypeCloud,
			SessionKey:    "session_search_latest",
			Status:        "active",
		},
	}
	if err := db.Create(&items).Error; err != nil {
		t.Fatalf("create conversations: %v", err)
	}

	results, err := repo.ListConversationsForSearch(ctx, 1, 1, 2, domainconversation.ExecutionTypeCloud, "", "needle")
	if err != nil {
		t.Fatalf("ListConversationsForSearch() error = %v", err)
	}
	if len(results) != 2 || results[0].PublicID != "conv_search_middle" || results[1].PublicID != "conv_search_oldest" {
		t.Fatalf("results = %#v, want middle and oldest conversations", results)
	}
}

func TestListLatestBranchPreviewMessagesReturnsLatestVisibleWindow(t *testing.T) {
	db := openConversationRepositoryTestDB(t)
	repo := NewRepo(db)
	ctx := context.Background()

	conversation := model.Conversation{
		UserID:     1,
		PublicID:   "conv_latest_branch_preview",
		Title:      "Latest branch preview",
		LabelsJSON: "[]",
		Model:      "gpt-test",
		Provider:   "openai",
		SessionKey: "session_latest_branch_preview",
		Status:     "active",
	}
	if err := db.Create(&conversation).Error; err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	createMessage := func(publicID string, role string, parentID *uint) model.Message {
		t.Helper()
		item := model.Message{
			ConversationID:  conversation.ID,
			UserID:          1,
			PublicID:        publicID,
			ParentMessageID: parentID,
			Role:            role,
			ContentType:     "text",
			Content:         publicID + " content",
			BranchReason:    "default",
			Status:          "success",
		}
		if err := db.Create(&item).Error; err != nil {
			t.Fatalf("create message %q: %v", publicID, err)
		}
		return item
	}

	root := createMessage("msg_root", "user", nil)
	rootID := root.ID
	createMessage("msg_old_branch", "assistant", &rootID)

	latestBranch := createMessage("msg_latest_branch", "assistant", &rootID)
	latestVisibleIDs := []string{root.PublicID, latestBranch.PublicID}
	parentID := latestBranch.ID
	for i := 1; i <= 12; i++ {
		role := "user"
		if i%2 == 0 {
			role = "assistant"
		}
		item := createMessage(fmt.Sprintf("msg_latest_%02d", i), role, &parentID)
		latestVisibleIDs = append(latestVisibleIDs, item.PublicID)
		parentID = item.ID
	}
	createMessage("msg_latest_tool", "tool", &parentID)

	items, err := repo.ListLatestBranchPreviewMessages(ctx, conversation.ID, 100, 10)
	if err != nil {
		t.Fatalf("ListLatestBranchPreviewMessages() error = %v", err)
	}
	if len(items) != 10 {
		t.Fatalf("len(items) = %d, want 10", len(items))
	}

	wantPublicIDs := latestVisibleIDs[len(latestVisibleIDs)-10:]
	for i, item := range items {
		if item.PublicID != wantPublicIDs[i] {
			t.Fatalf("items[%d].PublicID = %q, want %q", i, item.PublicID, wantPublicIDs[i])
		}
		if item.Role != "user" && item.Role != "assistant" {
			t.Fatalf("items[%d].Role = %q, want visible role", i, item.Role)
		}
	}
}

func openConversationRepositoryTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := testutil.Postgres(t)
	if err := db.AutoMigrate(&model.Conversation{}, &model.ConversationProject{}, &model.ConversationProjectMCPTool{}, &model.ConversationProjectSkill{}, &model.ConversationShare{}, &model.Message{}, &model.Attachment{}, &model.FileObject{}, &model.ConversationRun{}, &model.ChatRunEvent{}); err != nil {
		t.Fatalf("migrate models: %v", err)
	}
	return db
}

// parent_message_id 上没有外键，「父消息同会话」只靠应用层保证。这里绕过应用层直接写入
// 一条跨会话的父指针，确认递归查询不会走出当前会话——否则外部内容会进入 prompt 并被
// 烤进压缩摘要反复重放。ListMessageAncestorsUntil 早已有此约束，两者需保持一致。
func TestListMessageAncestorsStopsAtConversationBoundary(t *testing.T) {
	db := openConversationRepositoryTestDB(t)
	repo := NewRepo(db)
	ctx := context.Background()

	makeConversation := func(publicID string) model.Conversation {
		conversation := model.Conversation{
			UserID: 1, PublicID: publicID, Title: publicID,
			LabelsJSON: "[]", SessionKey: "session_" + publicID, Status: "active",
		}
		if err := db.Create(&conversation).Error; err != nil {
			t.Fatalf("create conversation %s: %v", publicID, err)
		}
		return conversation
	}
	foreign := makeConversation("conv_foreign")
	own := makeConversation("conv_own")

	// 另一个会话中的消息，内容不应被泄漏到本会话的祖先链里。
	foreignMessage := model.Message{
		ConversationID: foreign.ID, UserID: 1, PublicID: "msg_foreign",
		Role: "assistant", ContentType: "text", Content: "FOREIGN_SECRET",
		ReasoningContent: "FOREIGN_REASONING", BranchReason: "default", Status: "success",
	}
	if err := db.Create(&foreignMessage).Error; err != nil {
		t.Fatalf("create foreign message: %v", err)
	}

	leaf := model.Message{
		ConversationID: own.ID, UserID: 1, PublicID: "msg_own_leaf",
		ParentMessageID: &foreignMessage.ID,
		Role:            "user", ContentType: "text", Content: "own leaf",
		BranchReason: "default", Status: "success",
	}
	if err := db.Create(&leaf).Error; err != nil {
		t.Fatalf("create leaf: %v", err)
	}

	got, err := repo.ListMessageAncestors(ctx, own.ID, leaf.ID, 10)
	if err != nil {
		t.Fatalf("ListMessageAncestors() error = %v", err)
	}
	for _, item := range got {
		if item.ConversationID != own.ID {
			t.Fatalf("ancestor walked into conversation %d: %#v", item.ConversationID, item)
		}
		if strings.Contains(item.Content, "FOREIGN_SECRET") {
			t.Fatalf("foreign content leaked into ancestor chain: %#v", item)
		}
	}
	if len(got) != 1 || got[0].PublicID != "msg_own_leaf" {
		t.Fatalf("expected only the in-conversation leaf, got %#v", got)
	}
}

func TestListRecentContextArtifactsUsesCTEForLongBranchAndSnapshotBoundary(t *testing.T) {
	db := openConversationRepositoryTestDB(t)
	if err := db.AutoMigrate(&model.Message{}, &model.ChatContextRecord{}); err != nil {
		t.Fatalf("migrate context records: %v", err)
	}
	repo := NewRepo(db)
	ctx := context.Background()
	conversationID := uint(77)

	const branchLength = 1205
	var parentMessageID *uint
	branchMessages := make([]model.Message, 0, branchLength)
	branchMessageIDs := make([]uint, 0, branchLength)
	for index := 0; index < branchLength; index++ {
		messageID := uint(10_000 + index)
		message := model.Message{
			BaseModel:       model.BaseModel{ID: messageID},
			ConversationID:  conversationID,
			UserID:          1,
			PublicID:        fmt.Sprintf("msg_context_long_%d", index),
			ParentMessageID: parentMessageID,
			Role:            []string{"user", "assistant"}[index%2],
			ContentType:     "text",
			Content:         fmt.Sprintf("message %d", index),
			BranchReason:    "default",
			Status:          "success",
		}
		branchMessages = append(branchMessages, message)
		branchMessageIDs = append(branchMessageIDs, messageID)
		parentMessageID = &messageID
	}
	if err := db.CreateInBatches(&branchMessages, 50).Error; err != nil {
		t.Fatalf("create %d branch messages: %v", branchLength, err)
	}
	sibling := model.Message{
		ConversationID:  conversationID,
		UserID:          1,
		PublicID:        "msg_context_long_sibling",
		ParentMessageID: &branchMessageIDs[10],
		Role:            "assistant",
		ContentType:     "text",
		Content:         "sibling",
		BranchReason:    "retry",
		Status:          "success",
	}
	if err := db.Create(&sibling).Error; err != nil {
		t.Fatalf("create sibling: %v", err)
	}
	artifacts := []model.ChatContextRecord{
		{
			RecordType: chatContextRecordArtifact, ConversationID: conversationID, MessageID: branchMessageIDs[1], UserID: 1,
			Kind: string(domainconversation.ContextArtifactToolResult), SourceType: "tool_call", SourceID: "covered", Content: "covered evidence",
		},
		{
			RecordType: chatContextRecordArtifact, ConversationID: conversationID, MessageID: branchMessageIDs[999], UserID: 1,
			Kind: string(domainconversation.ContextArtifactToolResult), SourceType: "tool_call", SourceID: "boundary", Content: "boundary evidence",
		},
		{
			RecordType: chatContextRecordArtifact, ConversationID: conversationID, MessageID: branchMessageIDs[branchLength-2], UserID: 1,
			Kind: string(domainconversation.ContextArtifactToolResult), SourceType: "tool_call", SourceID: "retained", Content: "retained evidence",
		},
		{
			RecordType: chatContextRecordArtifact, ConversationID: conversationID, MessageID: sibling.ID, UserID: 1,
			Kind: string(domainconversation.ContextArtifactToolResult), SourceType: "tool_call", SourceID: "sibling", Content: "sibling evidence",
		},
	}
	if err := db.Create(&artifacts).Error; err != nil {
		t.Fatalf("create context artifacts: %v", err)
	}

	items, err := repo.ListRecentContextArtifacts(ctx, repository.ContextArtifactListFilter{
		Scope: repository.HistoricalMessageScope{
			ConversationID:          conversationID,
			UserID:                  1,
			LeafMessageID:           branchMessageIDs[branchLength-1],
			ExcludeThroughMessageID: branchMessageIDs[999],
		},
		Kinds: []domainconversation.ContextArtifactKind{domainconversation.ContextArtifactToolResult},
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("ListRecentContextArtifacts() error = %v", err)
	}
	if len(items) != 1 || items[0].SourceID != "retained" {
		t.Fatalf("expected only retained long-branch artifact, got %#v", items)
	}

	items, err = repo.ListRecentContextArtifacts(ctx, repository.ContextArtifactListFilter{
		Scope: repository.HistoricalMessageScope{
			ConversationID:          conversationID,
			UserID:                  1,
			LeafMessageID:           branchMessageIDs[branchLength-1],
			ExcludeThroughMessageID: sibling.ID,
		},
		Kinds: []domainconversation.ContextArtifactKind{domainconversation.ContextArtifactToolResult},
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("ListRecentContextArtifacts(invalid boundary) error = %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected non-ancestor boundary to fail closed, got %#v", items)
	}
}

func TestListRecentContextArtifactsHistoricalScopeTerminatesCycle(t *testing.T) {
	db := openConversationRepositoryTestDB(t)
	if err := db.AutoMigrate(&model.Message{}, &model.ChatContextRecord{}); err != nil {
		t.Fatalf("migrate context records: %v", err)
	}
	repo := NewRepo(db)
	ctx := context.Background()
	conversationID := uint(88)
	first := model.Message{
		ConversationID: conversationID, UserID: 1, PublicID: "msg_scope_cycle_first",
		Role: "assistant", ContentType: "text", Content: "first", BranchReason: "default", Status: "success",
	}
	if err := db.Create(&first).Error; err != nil {
		t.Fatalf("create first message: %v", err)
	}
	second := model.Message{
		ConversationID: conversationID, UserID: 1, PublicID: "msg_scope_cycle_second",
		ParentMessageID: &first.ID,
		Role:            "user", ContentType: "text", Content: "second", BranchReason: "default", Status: "success",
	}
	if err := db.Create(&second).Error; err != nil {
		t.Fatalf("create second message: %v", err)
	}
	if err := db.Model(&first).Update("parent_message_id", second.ID).Error; err != nil {
		t.Fatalf("create cycle: %v", err)
	}
	artifact := model.ChatContextRecord{
		RecordType: chatContextRecordArtifact, ConversationID: conversationID, MessageID: first.ID, UserID: 1,
		Kind: string(domainconversation.ContextArtifactToolResult), SourceType: "tool_call", SourceID: "cycle", Content: "cycle evidence",
	}
	if err := db.Create(&artifact).Error; err != nil {
		t.Fatalf("create cycle artifact: %v", err)
	}

	items, err := repo.ListRecentContextArtifacts(ctx, repository.ContextArtifactListFilter{
		Scope: repository.HistoricalMessageScope{ConversationID: conversationID, UserID: 1, LeafMessageID: second.ID},
		Kinds: []domainconversation.ContextArtifactKind{domainconversation.ContextArtifactToolResult},
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("ListRecentContextArtifacts() error = %v", err)
	}
	if len(items) != 1 || items[0].MessageID != first.ID {
		t.Fatalf("expected cycle to terminate with one historical artifact, got %#v", items)
	}
}

func TestHistoricalMessageScopeStopsAtUserBoundary(t *testing.T) {
	db := openConversationRepositoryTestDB(t)
	if err := db.AutoMigrate(&model.Message{}); err != nil {
		t.Fatalf("migrate messages: %v", err)
	}
	conversationID := uint(89)
	ownerAncestor := model.Message{
		ConversationID: conversationID, UserID: 1, PublicID: "msg_scope_owner_ancestor",
		Role: "assistant", ContentType: "text", Content: "owner ancestor", BranchReason: "default", Status: "success",
	}
	if err := db.Create(&ownerAncestor).Error; err != nil {
		t.Fatalf("create owner ancestor: %v", err)
	}
	foreignParent := model.Message{
		ConversationID: conversationID, UserID: 2, PublicID: "msg_scope_foreign_parent",
		ParentMessageID: &ownerAncestor.ID,
		Role:            "assistant", ContentType: "text", Content: "foreign parent", BranchReason: "default", Status: "success",
	}
	if err := db.Create(&foreignParent).Error; err != nil {
		t.Fatalf("create foreign parent: %v", err)
	}
	leaf := model.Message{
		ConversationID: conversationID, UserID: 1, PublicID: "msg_scope_owner_leaf",
		ParentMessageID: &foreignParent.ID,
		Role:            "user", ContentType: "text", Content: "owner leaf", BranchReason: "default", Status: "pending",
	}
	if err := db.Create(&leaf).Error; err != nil {
		t.Fatalf("create owner leaf: %v", err)
	}

	var messageIDs []uint
	if err := historicalMessageScopeSubquery(db, repository.HistoricalMessageScope{
		ConversationID: conversationID,
		UserID:         1,
		LeafMessageID:  leaf.ID,
	}).Scan(&messageIDs).Error; err != nil {
		t.Fatalf("query historical scope: %v", err)
	}
	if len(messageIDs) != 0 {
		t.Fatalf("expected traversal to stop at foreign-user parent, got message ids %v", messageIDs)
	}
}
