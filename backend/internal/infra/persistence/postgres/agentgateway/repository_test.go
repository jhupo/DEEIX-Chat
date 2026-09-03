package agentgateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	domainagent "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/agentgateway"
	model "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/models"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/agentprotocol"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/testutil"
)

func TestRetriableThreadCreateFailure(t *testing.T) {
	for _, code := range []string{"provider_error", "source_not_found", "artifact_error"} {
		if !retriableThreadCreateFailure(code) {
			t.Fatalf("explicit failure %q was not retryable", code)
		}
	}
	for _, code := range []string{"", "timeout", "outcome_unknown"} {
		if retriableThreadCreateFailure(code) {
			t.Fatalf("uncertain failure %q was retryable", code)
		}
	}
}

func TestValidWorkspaceSessionAcceptsLargeCodexTurn(t *testing.T) {
	events := make([]workspaceSessionEvent, 1509)
	for index := range events {
		events[index] = workspaceSessionEvent{
			Kind: "item/completed", SourceEventRef: fmt.Sprintf("event-%d", index), Payload: json.RawMessage(`{}`),
		}
	}
	session := workspaceSession{SourceThreadRef: "thread_source", HistoryLoaded: true, HistoryProjectionVersion: historyProjectionVersion, Messages: []workspaceSessionMessage{{
		Role: "assistant", Status: "success", Content: "done", SourceTurnRef: "turn_source", SourceMessageRef: "message-assistant", ExecutionEvents: events,
	}}}
	if !validWorkspaceSession(session, false) {
		t.Fatal("valid Codex turn exceeded the session projection limit")
	}
	session.Messages[0].ExecutionEvents = make([]workspaceSessionEvent, maxWorkspaceSessionEvents+1)
	if validWorkspaceSession(session, false) {
		t.Fatal("unbounded Codex turn was accepted")
	}
	session.Messages[0].ExecutionEvents = nil
	session.HistoryProjectionVersion = historyProjectionVersion + 1
	if validWorkspaceSession(session, false) {
		t.Fatal("future Codex session projection was accepted")
	}
}

func TestWorkspaceSessionSnapshotsReconcileEveryWorkspace(t *testing.T) {
	database := testutil.Postgres(t)
	if err := database.AutoMigrate(
		&model.AgentDevice{}, &model.AgentRuntimeProfile{}, &model.AgentWorkspace{}, &model.AgentThread{}, &model.Conversation{},
	); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)
	device := model.AgentDevice{
		PublicID: "agd_snapshot_reconcile", UserID: 7, Name: "desktop", Platform: "windows",
		PublicKey: []byte("public-key"), PublicKeyFingerprint: strings.Repeat("7", 64),
		CredentialVersion: 1, Status: domainagent.DeviceStatusActive,
	}
	if err := database.Create(&device).Error; err != nil {
		t.Fatal(err)
	}
	profile := model.AgentRuntimeProfile{
		PublicID: "codex-snapshot-reconcile", UserID: 7, DeviceID: device.ID, Provider: "codex", Status: "ready",
	}
	if err := database.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}
	workspaces := []model.AgentWorkspace{
		{PublicID: "workspace-snapshot-a", UserID: 7, DeviceID: device.ID, RuntimeProfileID: profile.ID, Name: "A", Status: "available", LastSeenAt: now},
		{PublicID: "workspace-snapshot-b", UserID: 7, DeviceID: device.ID, RuntimeProfileID: profile.ID, Name: "B", Status: "available", LastSeenAt: now},
	}
	if err := database.Create(&workspaces).Error; err != nil {
		t.Fatal(err)
	}
	session := func(ref, name string) workspaceSession {
		return workspaceSession{SourceThreadRef: ref, Name: name, Status: "active", CreatedAt: now.Unix(), UpdatedAt: now.Unix()}
	}
	alpha, beta, gamma := session("thread-alpha", "Alpha"), session("thread-beta", "Beta"), session("thread-gamma", "Gamma")
	if _, err := syncWorkspaceSessions(database, &device, profile.ID, workspaces[0].ID, []workspaceSession{alpha, beta}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := syncWorkspaceSessions(database, &device, profile.ID, workspaces[1].ID, []workspaceSession{gamma}, now); err != nil {
		t.Fatal(err)
	}

	var alphaThread, betaThread, gammaThread model.AgentThread
	if err := database.Where("runtime_profile_id = ? AND source_thread_ref = ?", profile.ID, alpha.SourceThreadRef).First(&alphaThread).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Where("runtime_profile_id = ? AND source_thread_ref = ?", profile.ID, beta.SourceThreadRef).First(&betaThread).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Where("runtime_profile_id = ? AND source_thread_ref = ?", profile.ID, gamma.SourceThreadRef).First(&gammaThread).Error; err != nil {
		t.Fatal(err)
	}
	alphaConversationID := alphaThread.ConversationID
	changed, err := syncWorkspaceSessions(database, &device, profile.ID, workspaces[0].ID, []workspaceSession{beta}, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	var alphaConversation model.Conversation
	if err := database.Unscoped().First(&alphaConversation, alphaConversationID).Error; err != nil ||
		alphaConversation.DeletedAt.Valid != true || !slices.Equal(changed, []string{alphaConversation.PublicID}) {
		t.Fatalf("missing conversation = %#v changed=%#v err=%v", alphaConversation, changed, err)
	}
	if err := database.First(&alphaThread, alphaThread.ID).Error; err != nil || alphaThread.Status != threadStatusMissing {
		t.Fatalf("missing thread = %#v err=%v", alphaThread, err)
	}
	if err := database.First(&gammaThread, gammaThread.ID).Error; err != nil || gammaThread.Status != "active" {
		t.Fatalf("other workspace thread changed = %#v err=%v", gammaThread, err)
	}

	changed, err = syncWorkspaceSessions(database, &device, profile.ID, workspaces[1].ID, []workspaceSession{alpha, gamma}, now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if err := database.First(&alphaThread, alphaThread.ID).Error; err != nil || alphaThread.Status != "active" ||
		alphaThread.WorkspaceID != workspaces[1].ID || alphaThread.ConversationID != alphaConversationID {
		t.Fatalf("restored moved thread = %#v err=%v", alphaThread, err)
	}
	if err := database.First(&alphaConversation, alphaConversationID).Error; err != nil ||
		alphaConversation.ExecutionWorkspaceID != workspaces[1].PublicID || !slices.Equal(changed, []string{alphaConversation.PublicID}) {
		t.Fatalf("restored moved conversation = %#v changed=%#v err=%v", alphaConversation, changed, err)
	}
	var betaConversation model.Conversation
	if err := database.First(&betaConversation, betaThread.ConversationID).Error; err != nil {
		t.Fatal(err)
	}
	changed, err = syncWorkspaceSessions(database, &device, profile.ID, workspaces[0].ID, nil, now.Add(3*time.Second))
	if err != nil || !slices.Equal(changed, []string{betaConversation.PublicID}) {
		t.Fatalf("empty workspace snapshot changed=%#v err=%v", changed, err)
	}
	if err := database.First(&betaThread, betaThread.ID).Error; err != nil || betaThread.Status != threadStatusMissing {
		t.Fatalf("empty workspace snapshot thread = %#v err=%v", betaThread, err)
	}
	if err := database.Model(&betaThread).Update("status", "active").Error; err != nil {
		t.Fatal(err)
	}
	changed, err = syncWorkspaceSessions(database, &device, profile.ID, workspaces[0].ID, []workspaceSession{beta}, now.Add(4*time.Second))
	if err != nil || !slices.Equal(changed, []string{betaConversation.PublicID}) {
		t.Fatalf("repair inconsistent active session changed=%#v err=%v", changed, err)
	}
	if err := database.First(&betaConversation, betaConversation.ID).Error; err != nil || betaConversation.DeletedAt.Valid {
		t.Fatalf("active session conversation was not restored = %#v err=%v", betaConversation, err)
	}

	if err := database.Model(&alphaThread).Update("status", "deleted").Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Model(&alphaConversation).Update("deleted_at", now.Add(4*time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Model(&gammaThread).Update("status", threadStatusDeletingActive).Error; err != nil {
		t.Fatal(err)
	}
	changed, err = syncWorkspaceSessions(database, &device, profile.ID, workspaces[1].ID, []workspaceSession{alpha, gamma}, now.Add(5*time.Second))
	if err != nil || len(changed) != 0 {
		t.Fatalf("delete-protected snapshot changed=%#v err=%v", changed, err)
	}
	if err := database.First(&alphaThread, alphaThread.ID).Error; err != nil || alphaThread.Status != "deleted" {
		t.Fatalf("deleted thread was revived = %#v err=%v", alphaThread, err)
	}
	if err := database.Unscoped().First(&alphaConversation, alphaConversationID).Error; err != nil || !alphaConversation.DeletedAt.Valid {
		t.Fatalf("deleted conversation was revived = %#v err=%v", alphaConversation, err)
	}
	if err := database.First(&gammaThread, gammaThread.ID).Error; err != nil || gammaThread.Status != threadStatusDeletingActive {
		t.Fatalf("deleting thread was revived = %#v err=%v", gammaThread, err)
	}
}

func TestSyncWorkspaceSessionExecutionEventsReplacesDerivedHistory(t *testing.T) {
	database := testutil.Postgres(t)
	if err := database.AutoMigrate(&model.Conversation{}, &model.ConversationExecutionEvent{}); err != nil {
		t.Fatal(err)
	}
	conversation := model.Conversation{
		PublicID: "conversation_history_reprojection", UserID: 7, Title: "History", ExecutionEventSeq: 2,
	}
	if err := database.Create(&conversation).Error; err != nil {
		t.Fatal(err)
	}
	rows := []model.ConversationExecutionEvent{
		{ConversationID: conversation.ID, UserID: 7, RunID: "run_history", SourceKey: "history:run_history:0", Seq: 1, Kind: "turn/started", PayloadJSON: `{}`},
		{ConversationID: conversation.ID, UserID: 7, RunID: "run_live", SourceKey: "event:run_live:0", Seq: 2, Kind: "turn/started", PayloadJSON: `{}`},
	}
	if err := database.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	messages := []workspaceSessionMessage{{
		Role: "assistant", Status: "success", RunID: "run_history", CreatedAt: 1,
		ExecutionEvents: []workspaceSessionEvent{{
			Kind: "item/completed", SourceEventRef: "item-compact",
			Payload: json.RawMessage(`{"itemID":"compact","item":{"kind":"contextCompaction","status":"completed"}}`),
		}},
	}}
	if err := syncWorkspaceSessionExecutionEvents(database, &conversation, messages, true, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	var events []model.ConversationExecutionEvent
	if err := database.Where("conversation_id = ?", conversation.ID).Order("seq ASC").Find(&events).Error; err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].RunID != "run_live" || events[1].RunID != "run_history" ||
		events[1].Seq != 3 || events[1].Kind != "item/completed" {
		t.Fatalf("reprojected execution events = %#v", events)
	}
}

func TestSyncWorkspaceSessionExecutionEventsScopesKeysToConversation(t *testing.T) {
	database := testutil.Postgres(t)
	if err := database.AutoMigrate(&model.Conversation{}, &model.ConversationExecutionEvent{}); err != nil {
		t.Fatal(err)
	}
	conversations := []model.Conversation{
		{PublicID: "conversation_history_scope_a", UserID: 7, Title: "Original"},
		{PublicID: "conversation_history_scope_b", UserID: 7, Title: "Fork"},
	}
	if err := database.Create(&conversations).Error; err != nil {
		t.Fatal(err)
	}
	messages := []workspaceSessionMessage{{
		Role: "assistant", Status: "success", RunID: "run_shared_turn", CreatedAt: 1,
		ExecutionEvents: []workspaceSessionEvent{{
			Kind: "item/completed", SourceEventRef: "item-shared",
			Payload: json.RawMessage(`{"itemID":"shared","item":{"kind":"reasoning","status":"completed"}}`),
		}},
	}}
	for index := range conversations {
		if err := syncWorkspaceSessionExecutionEvents(database, &conversations[index], messages, true, time.Now().UTC()); err != nil {
			t.Fatalf("project conversation %d: %v", index, err)
		}
	}
	var events []model.ConversationExecutionEvent
	if err := database.Order("id ASC").Find(&events).Error; err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].SourceKey == events[1].SourceKey {
		t.Fatalf("conversation-scoped event keys = %#v", events)
	}
}

func TestWorkspaceHistoryDeltaUpdatesActiveTurnInPlace(t *testing.T) {
	database := testutil.Postgres(t)
	if err := database.AutoMigrate(
		&model.Conversation{}, &model.Message{}, &model.Attachment{}, &model.ConversationRun{},
		&model.ConversationExecutionEvent{}, &model.AgentWorkspace{}, &model.AgentThread{}, &model.AgentTurn{},
	); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	conversation := model.Conversation{
		PublicID: "conversation_history_delta", UserID: 7, Title: "Any project", ExecutionType: "gateway", Status: "active",
		BaseModel: model.BaseModel{CreatedAt: now, UpdatedAt: now},
	}
	if err := database.Create(&conversation).Error; err != nil {
		t.Fatal(err)
	}
	workspace := model.AgentWorkspace{PublicID: "workspace-history-delta", UserID: 7, DeviceID: 1, RuntimeProfileID: 1, Status: "available"}
	if err := database.Create(&workspace).Error; err != nil {
		t.Fatal(err)
	}
	sourceThreadRef := "thread-history-delta"
	thread := model.AgentThread{
		PublicID: "agth_history_delta", UserID: 7, DeviceID: 1, RuntimeProfileID: 1, WorkspaceID: workspace.ID,
		ConversationID: conversation.ID, SourceThreadRef: &sourceThreadRef, Status: "active", HistoryStatus: "loading", HistoryVersion: 5,
	}
	if err := database.Create(&thread).Error; err != nil {
		t.Fatal(err)
	}
	completedEvents := []workspaceSessionEvent{
		{Kind: "turn/started", SourceEventRef: "turn-started", Payload: json.RawMessage(`{"turn":{"status":"running"}}`)},
		{Kind: "turn/completed", SourceEventRef: "turn-completed", Payload: json.RawMessage(`{"turn":{"status":"completed"}}`)},
	}
	initial := workspaceSession{
		SourceThreadRef: sourceThreadRef, HistoryLoaded: true, HistoryProjectionVersion: historyProjectionVersion, Status: "active", UpdatedAt: now.Unix(),
		Messages: []workspaceSessionMessage{
			{Role: "user", Status: "success", Content: "first", SourceTurnRef: "turn-first", SourceMessageRef: "message-first-user", CreatedAt: now.Unix()},
			{Role: "assistant", Status: "success", Content: "done", SourceTurnRef: "turn-first", SourceMessageRef: "message-first-assistant", CreatedAt: now.Unix(), ExecutionEvents: completedEvents},
		},
	}
	if changed, err := syncExistingWorkspaceSession(database, &thread, &workspace, initial, now); err != nil || !changed {
		t.Fatalf("initial history projection: changed=%v err=%v", changed, err)
	}
	if err := database.First(&thread, thread.ID).Error; err != nil {
		t.Fatal(err)
	}
	activeSourceTurnRef := "turn-second"
	activeTurn := model.AgentTurn{
		PublicID: "agturn_history_delta_active", UserID: 7, ThreadID: thread.ID,
		RunID: "run_history_delta_active", SourceTurnRef: &activeSourceTurnRef, Status: "queued", InputJSON: "[]", SettingsJSON: "{}",
	}
	if err := database.Create(&activeTurn).Error; err != nil {
		t.Fatal(err)
	}
	active := workspaceSession{
		SourceThreadRef: sourceThreadRef, HistoryLoaded: true, HistoryProjectionVersion: historyProjectionVersion, HistoryDelta: true, Status: "active", UpdatedAt: now.Add(time.Second).Unix(),
		Messages: []workspaceSessionMessage{
			{Role: "user", Status: "success", Content: "second", SourceTurnRef: "turn-second", SourceMessageRef: "message-second-user", CreatedAt: now.Add(time.Second).Unix()},
			{Role: "assistant", Status: "pending", ReasoningContent: "planning", SourceTurnRef: "turn-second", SourceMessageRef: "message-second-assistant", CreatedAt: now.Add(time.Second).Unix(), ExecutionEvents: []workspaceSessionEvent{
				{Kind: "turn/started", SourceEventRef: "turn-started", Payload: json.RawMessage(`{"turn":{"status":"running"}}`)},
				{Kind: "item/completed", SourceEventRef: "item-reasoning", Payload: json.RawMessage(`{"itemID":"reasoning","item":{"kind":"reasoning","status":"inProgress"}}`)},
			}},
		},
	}
	if changed, err := syncExistingWorkspaceSession(database, &thread, &workspace, active, now.Add(time.Second)); err != nil || !changed {
		t.Fatalf("active history delta: changed=%v err=%v", changed, err)
	}
	var activeAssistant model.Message
	if err := database.Where("conversation_id = ? AND source_ref = ?", conversation.ID, "message-second-assistant").First(&activeAssistant).Error; err != nil ||
		activeAssistant.Status != "pending" || activeAssistant.Content != "" || activeAssistant.ReasoningContent != "planning" ||
		activeAssistant.RunID != activeTurn.RunID {
		t.Fatalf("active assistant = %#v err=%v", activeAssistant, err)
	}
	if err := database.First(&activeTurn, activeTurn.ID).Error; err != nil || activeTurn.Status != "running" {
		t.Fatalf("active Agent turn = %#v err=%v", activeTurn, err)
	}
	if err := database.Model(&activeTurn).Updates(map[string]any{
		"status": "interrupted", "error_code": "gateway_interrupted", "error_message": "connection closed",
	}).Error; err != nil {
		t.Fatal(err)
	}
	completed := active
	completed.Messages[1].Status = "success"
	completed.Messages[1].Content = "second done"
	completed.Messages[1].ExecutionEvents = append(completed.Messages[1].ExecutionEvents, workspaceSessionEvent{
		Kind: "turn/completed", SourceEventRef: "turn-completed", Payload: json.RawMessage(`{"turn":{"status":"completed"}}`),
	})
	if changed, err := syncExistingWorkspaceSession(database, &thread, &workspace, completed, now.Add(2*time.Second)); err != nil || !changed {
		t.Fatalf("completed history delta: changed=%v err=%v", changed, err)
	}
	var completedAssistant model.Message
	if err := database.Where("conversation_id = ? AND source_ref = ?", conversation.ID, "message-second-assistant").First(&completedAssistant).Error; err != nil ||
		completedAssistant.ID != activeAssistant.ID || completedAssistant.Status != "success" || completedAssistant.Content != "second done" {
		t.Fatalf("completed assistant = %#v err=%v", completedAssistant, err)
	}
	if err := database.First(&activeTurn, activeTurn.ID).Error; err != nil || activeTurn.Status != "completed" ||
		activeTurn.ErrorCode != "" || activeTurn.ErrorMessage != "" {
		t.Fatalf("completed Agent turn = %#v err=%v", activeTurn, err)
	}
	var liveCount, totalCount int64
	if err := database.Model(&model.Message{}).Where("conversation_id = ?", conversation.ID).Count(&liveCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Unscoped().Model(&model.Message{}).Where("conversation_id = ?", conversation.ID).Count(&totalCount).Error; err != nil {
		t.Fatal(err)
	}
	if liveCount != 4 || totalCount != 4 {
		t.Fatalf("message projection created stale rows: live=%d total=%d", liveCount, totalCount)
	}
	var secondUser model.Message
	if err := database.Where("conversation_id = ? AND source_ref = ?", conversation.ID, "message-second-user").First(&secondUser).Error; err != nil ||
		secondUser.ParentMessageID == nil || completedAssistant.ParentMessageID == nil || *completedAssistant.ParentMessageID != secondUser.ID {
		t.Fatalf("delta message lineage = user:%#v assistant:%#v err=%v", secondUser, completedAssistant, err)
	}

	secondUserContext := active.Messages[0]
	assistantContext := completed.Messages[1]
	assistantContext.ExecutionEvents = nil
	steered := workspaceSession{
		SourceThreadRef: sourceThreadRef, HistoryLoaded: true, HistoryProjectionVersion: historyProjectionVersion, HistoryDelta: true, Status: "active", UpdatedAt: now.Add(3 * time.Second).Unix(),
		Messages: []workspaceSessionMessage{
			secondUserContext,
			{Role: "user", Status: "success", Content: "steer", SourceTurnRef: "turn-second", SourceMessageRef: "message-steer-user", CreatedAt: now.Add(time.Second).Unix()},
			assistantContext,
		},
	}
	if changed, err := syncExistingWorkspaceSession(database, &thread, &workspace, steered, now.Add(3*time.Second)); err != nil || !changed {
		t.Fatalf("steered history delta: changed=%v err=%v", changed, err)
	}
	var steeringUser model.Message
	if err := database.Where("conversation_id = ? AND source_ref = ?", conversation.ID, "message-steer-user").First(&steeringUser).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Where("conversation_id = ? AND source_ref = ?", conversation.ID, "message-second-assistant").First(&completedAssistant).Error; err != nil ||
		completedAssistant.ParentMessageID == nil || *completedAssistant.ParentMessageID != steeringUser.ID {
		t.Fatalf("steered assistant lineage = steer:%#v assistant:%#v err=%v", steeringUser, completedAssistant, err)
	}

	nextTurn := workspaceSession{
		SourceThreadRef: sourceThreadRef, HistoryLoaded: true, HistoryProjectionVersion: historyProjectionVersion, HistoryDelta: true, Status: "active", UpdatedAt: now.Add(4 * time.Second).Unix(),
		Messages: []workspaceSessionMessage{
			assistantContext,
			{Role: "user", Status: "success", Content: "third", SourceTurnRef: "turn-third", SourceMessageRef: "message-third-user", CreatedAt: now.Add(2 * time.Second).Unix()},
		},
	}
	if changed, err := syncExistingWorkspaceSession(database, &thread, &workspace, nextTurn, now.Add(4*time.Second)); err != nil || !changed {
		t.Fatalf("next-turn history delta: changed=%v err=%v", changed, err)
	}
	var thirdUser model.Message
	if err := database.Where("conversation_id = ? AND source_ref = ?", conversation.ID, "message-third-user").First(&thirdUser).Error; err != nil ||
		thirdUser.ParentMessageID == nil || *thirdUser.ParentMessageID != completedAssistant.ID {
		t.Fatalf("next-turn lineage = assistant:%#v user:%#v err=%v", completedAssistant, thirdUser, err)
	}
}

func TestStartTurnRepairsStaleAgentTurnsFromMessageBeforeRun(t *testing.T) {
	database := testutil.Postgres(t)
	if err := database.AutoMigrate(
		&model.AgentDevice{}, &model.AgentRuntimeProfile{}, &model.AgentWorkspace{}, &model.AgentThread{},
		&model.AgentTurn{}, &model.AgentInteraction{}, &model.AgentCommand{}, &model.AgentIdempotencyRecord{},
		&model.Conversation{}, &model.Message{}, &model.ConversationRun{},
	); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 3, 3, 0, 0, 0, time.UTC)
	device := model.AgentDevice{
		PublicID: "agd_start_turn_repair_0123456789ab", UserID: 7, Name: "desktop", Platform: "windows",
		PublicKey: bytes.Repeat([]byte("k"), 32), PublicKeyFingerprint: strings.Repeat("a", 64),
		CredentialVersion: 1, Status: domainagent.DeviceStatusActive, NextServerSeq: 1,
	}
	if err := database.Create(&device).Error; err != nil {
		t.Fatal(err)
	}
	leaseExpiresAt := now.Add(time.Hour)
	profile := model.AgentRuntimeProfile{
		PublicID: "codex-default", UserID: 7, DeviceID: device.ID, Provider: "codex", Status: domainagent.RuntimeStatusReady,
		LeaseExpiresAt: &leaseExpiresAt,
		ManifestJSON:   `{"commands":["turn.start"],"threadSettings":{"model":true,"reasoningEffort":["high"],"approvalPolicy":["on-request"],"approvalsReviewer":["user"],"sandboxPolicy":["workspace-write"]}}`,
	}
	if err := database.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}
	workspace := model.AgentWorkspace{
		PublicID: "workspace-start-turn-repair", UserID: 7, DeviceID: device.ID, RuntimeProfileID: profile.ID,
		Name: "project", Status: "available", LastSeenAt: now,
	}
	if err := database.Create(&workspace).Error; err != nil {
		t.Fatal(err)
	}
	conversation := model.Conversation{
		PublicID: "conversation_start_turn_repair", UserID: 7, Title: "Repair", ExecutionType: "gateway", Status: "active",
	}
	if err := database.Create(&conversation).Error; err != nil {
		t.Fatal(err)
	}
	sourceThreadRef := "thread-start-turn-repair"
	thread := model.AgentThread{
		PublicID: "agth_start_turn_repair_0123456789a", UserID: 7, DeviceID: device.ID, RuntimeProfileID: profile.ID,
		WorkspaceID: workspace.ID, ConversationID: conversation.ID, SourceThreadRef: &sourceThreadRef,
		Status: "active", HistoryStatus: "loaded", HistoryVersion: historyProjectionVersion,
	}
	if err := database.Create(&thread).Error; err != nil {
		t.Fatal(err)
	}
	staleSourceTurnRef := "turn-stale-running"
	staleTurn := model.AgentTurn{
		PublicID: "agturn_stale_running_0123456789ab", UserID: 7, ThreadID: thread.ID,
		RunID: "run_stale_running", SourceTurnRef: &staleSourceTurnRef, Status: "running", InputJSON: "[]", SettingsJSON: "{}",
	}
	if err := database.Create(&staleTurn).Error; err != nil {
		t.Fatal(err)
	}
	endedAt := now.Add(-time.Minute)
	if err := database.Create(&model.ConversationRun{
		RunID: staleTurn.RunID, UserID: 7, ConversationID: conversation.ID, Endpoint: "local_gateway",
		Status: "interrupted", ErrorCode: "gateway_interrupted", ErrorMessage: "local execution was interrupted",
		StartedAt: now.Add(-2 * time.Minute), EndedAt: &endedAt,
	}).Error; err != nil {
		t.Fatal(err)
	}
	messageUpdatedAt := now.Add(-30 * time.Second)
	if err := database.Create(&model.Message{
		ConversationID: conversation.ID, UserID: 7, PublicID: "msg_completed_projection", RunID: staleTurn.RunID,
		Role: "assistant", ContentType: "text", Content: "completed on device", Status: "success", BranchReason: "default",
		BaseModel: model.BaseModel{CreatedAt: messageUpdatedAt, UpdatedAt: messageUpdatedAt},
	}).Error; err != nil {
		t.Fatal(err)
	}
	fallbackSourceTurnRef := "turn-fallback-running"
	fallbackTurn := model.AgentTurn{
		PublicID: "agturn_fallback_running_0123456789", UserID: 7, ThreadID: thread.ID,
		RunID: "run_fallback_running", SourceTurnRef: &fallbackSourceTurnRef, Status: "running", InputJSON: "[]", SettingsJSON: "{}",
	}
	if err := database.Create(&fallbackTurn).Error; err != nil {
		t.Fatal(err)
	}
	fallbackEndedAt := now.Add(-15 * time.Second)
	if err := database.Create(&model.ConversationRun{
		RunID: fallbackTurn.RunID, UserID: 7, ConversationID: conversation.ID, Endpoint: "local_gateway",
		Status: "error", ErrorCode: "gateway_failed", ErrorMessage: "local execution failed",
		StartedAt: now.Add(-time.Minute), EndedAt: &fallbackEndedAt,
	}).Error; err != nil {
		t.Fatal(err)
	}
	settings := `{"model":"gpt-test","reasoningEffort":"high","approvalPolicy":"on-request","approvalsReviewer":"user","sandboxPolicy":"workspace-write"}`
	repo := NewRepo(database)
	created, err := repo.StartTurn(
		context.Background(), "41234567-89ab-4def-8123-456789abcdef", strings.Repeat("d", 64),
		&domainagent.Turn{
			PublicID: "agturn_new_after_repair_0123456789a", UserID: 7, ThreadPublicID: thread.PublicID,
			RunID: "run_new_after_repair", Status: "queued", InputJSON: `[{"kind":"text","text":"continue"}]`, SettingsJSON: settings,
		},
		&domainagent.Command{PublicID: "agcmd_new_after_repair_0123456789ab", Kind: "turn.start"}, now,
	)
	if err != nil || created == nil || created.RunID != "run_new_after_repair" {
		t.Fatalf("StartTurn() = %#v, %v", created, err)
	}
	if err := database.First(&staleTurn, staleTurn.ID).Error; err != nil || staleTurn.Status != "completed" ||
		staleTurn.ErrorCode != "" || staleTurn.ErrorMessage != "" || !staleTurn.UpdatedAt.Equal(messageUpdatedAt) {
		t.Fatalf("stale Agent turn was not repaired: %#v err=%v", staleTurn, err)
	}
	if err := database.First(&fallbackTurn, fallbackTurn.ID).Error; err != nil || fallbackTurn.Status != "failed" ||
		fallbackTurn.ErrorCode != "gateway_failed" || fallbackTurn.ErrorMessage != "local execution failed" ||
		!fallbackTurn.UpdatedAt.Equal(fallbackEndedAt) {
		t.Fatalf("fallback Agent turn was not repaired: %#v err=%v", fallbackTurn, err)
	}
}

func TestWorkspaceHistoryDeltaDoesNotReplaceOlderProjection(t *testing.T) {
	database := testutil.Postgres(t)
	if err := database.AutoMigrate(
		&model.Conversation{}, &model.Message{}, &model.Attachment{}, &model.ConversationRun{},
		&model.ConversationExecutionEvent{}, &model.AgentWorkspace{}, &model.AgentThread{}, &model.AgentTurn{},
	); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)
	conversation := model.Conversation{
		PublicID: "conversation_projection_upgrade", UserID: 7, Title: "Every project",
		ExecutionType: "gateway", Status: "active", BaseModel: model.BaseModel{CreatedAt: now, UpdatedAt: now},
	}
	if err := database.Create(&conversation).Error; err != nil {
		t.Fatal(err)
	}
	workspace := model.AgentWorkspace{PublicID: "workspace-projection-upgrade", UserID: 7, DeviceID: 1, RuntimeProfileID: 1, Status: "available"}
	if err := database.Create(&workspace).Error; err != nil {
		t.Fatal(err)
	}
	sourceThreadRef := "thread-projection-upgrade"
	thread := model.AgentThread{
		PublicID: "agth_projection_upgrade", UserID: 7, DeviceID: 1, RuntimeProfileID: 1,
		WorkspaceID: workspace.ID, ConversationID: conversation.ID, SourceThreadRef: &sourceThreadRef,
		Status: "active", HistoryStatus: "loaded", HistoryVersion: historyProjectionVersion - 1,
	}
	if err := database.Create(&thread).Error; err != nil {
		t.Fatal(err)
	}
	existing := []model.Message{
		{ConversationID: conversation.ID, UserID: 7, PublicID: "message_projection_user", Role: "user", ContentType: "text", Content: "first", SourceRef: "source-existing-user", Status: "success"},
		{ConversationID: conversation.ID, UserID: 7, PublicID: "message_projection_assistant", Role: "assistant", ContentType: "text", Content: "complete history", SourceRef: "source-existing-assistant", Status: "success"},
	}
	if err := database.Create(&existing).Error; err != nil {
		t.Fatal(err)
	}
	delta := workspaceSession{
		SourceThreadRef: sourceThreadRef, HistoryLoaded: true, HistoryProjectionVersion: historyProjectionVersion, HistoryDelta: true, Status: "active",
		UpdatedAt: now.Add(time.Second).Unix(),
		Messages: []workspaceSessionMessage{{
			Role: "assistant", Status: "interrupted", Content: "last turn", SourceTurnRef: "turn-last",
			SourceMessageRef: "message-last-assistant", CreatedAt: now.Add(time.Second).Unix(),
			ExecutionEvents: []workspaceSessionEvent{
				{Kind: "turn/started", SourceEventRef: "turn:started", Payload: json.RawMessage(`{"turn":{"status":"running"}}`)},
				{Kind: "turn/completed", SourceEventRef: "turn:completed", Payload: json.RawMessage(`{"turn":{"status":"interrupted"}}`)},
			},
		}},
	}
	changed, err := syncExistingWorkspaceSession(database, &thread, &workspace, delta, now.Add(time.Second))
	if err != nil || !changed {
		t.Fatalf("defer projection upgrade delta: changed=%v err=%v", changed, err)
	}
	var messages []model.Message
	if err := database.Where("conversation_id = ?", conversation.ID).Order("id ASC").Find(&messages).Error; err != nil ||
		len(messages) != 2 || messages[0].Content != "first" || messages[1].Content != "complete history" {
		t.Fatalf("older projection was replaced by a tail delta: %#v err=%v", messages, err)
	}
	if err := database.First(&thread, thread.ID).Error; err != nil || thread.HistoryStatus != "unloaded" ||
		thread.HistoryVersion != historyProjectionVersion-1 {
		t.Fatalf("projection upgrade state = %#v err=%v", thread, err)
	}
}

func TestWorkspaceHistoryProjectionRequiresCurrentProducerVersion(t *testing.T) {
	database := testutil.Postgres(t)
	if err := database.AutoMigrate(
		&model.Conversation{}, &model.Message{}, &model.Attachment{}, &model.ConversationRun{},
		&model.ConversationExecutionEvent{}, &model.AgentWorkspace{}, &model.AgentThread{}, &model.AgentTurn{},
	); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 2, 11, 0, 0, 0, time.UTC)
	conversation := model.Conversation{
		PublicID: "conversation_producer_upgrade", UserID: 7, Title: "Producer upgrade",
		ExecutionType: "gateway", Status: "active", BaseModel: model.BaseModel{CreatedAt: now, UpdatedAt: now},
	}
	if err := database.Create(&conversation).Error; err != nil {
		t.Fatal(err)
	}
	workspace := model.AgentWorkspace{PublicID: "workspace-producer-upgrade", UserID: 7, DeviceID: 1, RuntimeProfileID: 1, Status: "available"}
	if err := database.Create(&workspace).Error; err != nil {
		t.Fatal(err)
	}
	sourceThreadRef := "thread-producer-upgrade"
	thread := model.AgentThread{
		PublicID: "agth_producer_upgrade", UserID: 7, DeviceID: 1, RuntimeProfileID: 1,
		WorkspaceID: workspace.ID, ConversationID: conversation.ID, SourceThreadRef: &sourceThreadRef,
		Status: "active", HistoryStatus: "loading", HistoryVersion: historyProjectionVersion - 1,
	}
	if err := database.Create(&thread).Error; err != nil {
		t.Fatal(err)
	}
	existing := model.Message{
		ConversationID: conversation.ID, UserID: 7, PublicID: "message_producer_existing", Role: "assistant",
		ContentType: "text", Content: "authoritative old history", SourceRef: "source-producer-existing", Status: "success",
	}
	if err := database.Create(&existing).Error; err != nil {
		t.Fatal(err)
	}
	outdated := workspaceSession{
		SourceThreadRef: sourceThreadRef, HistoryLoaded: true, Status: "active", UpdatedAt: now.Add(time.Second).Unix(),
		Messages: []workspaceSessionMessage{{
			Role: "assistant", Status: "interrupted", Content: "premature terminal",
			SourceTurnRef: "turn-current", SourceMessageRef: "message-current-assistant", CreatedAt: now.Add(time.Second).Unix(),
		}},
	}
	if changed, err := syncExistingWorkspaceSession(database, &thread, &workspace, outdated, now.Add(time.Second)); err != nil || !changed {
		t.Fatalf("outdated producer projection: changed=%v err=%v", changed, err)
	}
	var messages []model.Message
	if err := database.Where("conversation_id = ?", conversation.ID).Find(&messages).Error; err != nil ||
		len(messages) != 1 || messages[0].Content != existing.Content {
		t.Fatalf("outdated producer replaced history: %#v err=%v", messages, err)
	}
	if err := database.First(&thread, thread.ID).Error; err != nil || thread.HistoryVersion != historyProjectionVersion-1 || thread.HistoryStatus != "unloaded" {
		t.Fatalf("outdated producer advanced projection: %#v err=%v", thread, err)
	}

	current := outdated
	current.HistoryProjectionVersion = historyProjectionVersion
	current.Messages[0].Status = "success"
	current.Messages[0].Content = "authoritative current history"
	if changed, err := syncExistingWorkspaceSession(database, &thread, &workspace, current, now.Add(2*time.Second)); err != nil || !changed {
		t.Fatalf("current producer projection: changed=%v err=%v", changed, err)
	}
	if err := database.Where("conversation_id = ?", conversation.ID).Find(&messages).Error; err != nil ||
		len(messages) != 1 || messages[0].Content != "authoritative current history" {
		t.Fatalf("current producer did not replace history: %#v err=%v", messages, err)
	}
	if err := database.First(&thread, thread.ID).Error; err != nil || thread.HistoryVersion != historyProjectionVersion || thread.HistoryStatus != "loaded" {
		t.Fatalf("current producer projection state: %#v err=%v", thread, err)
	}
}

func TestWorkspaceHistoryBatchProjectsOnlyAfterFinalChunk(t *testing.T) {
	database := testutil.Postgres(t)
	if err := database.AutoMigrate(
		&model.AgentDevice{}, &model.AgentRuntimeProfile{}, &model.AgentWorkspace{}, &model.AgentThread{},
		&model.AgentBridgeFrame{}, &model.AgentEvent{}, &model.AgentTurn{}, &model.Conversation{},
		&model.Message{}, &model.Attachment{}, &model.ConversationRun{}, &model.ConversationExecutionEvent{},
	); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC)
	device := model.AgentDevice{
		PublicID: "agd_history_batch", UserID: 7, Name: "desktop", Platform: "windows",
		PublicKey: []byte("public-key"), PublicKeyFingerprint: strings.Repeat("1", 64),
		CredentialVersion: 1, Status: domainagent.DeviceStatusActive,
	}
	if err := database.Create(&device).Error; err != nil {
		t.Fatal(err)
	}
	profile := model.AgentRuntimeProfile{PublicID: "codex-history-batch", UserID: 7, DeviceID: device.ID, Provider: "codex", Status: "ready"}
	if err := database.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}
	workspace := model.AgentWorkspace{PublicID: "workspace-history-batch", UserID: 7, DeviceID: device.ID, RuntimeProfileID: profile.ID, Status: "available"}
	if err := database.Create(&workspace).Error; err != nil {
		t.Fatal(err)
	}
	conversation := model.Conversation{
		PublicID: "conversation_history_batch", UserID: 7, Title: "All projects", ExecutionType: "gateway", Status: "active",
		BaseModel: model.BaseModel{CreatedAt: now, UpdatedAt: now},
	}
	if err := database.Create(&conversation).Error; err != nil {
		t.Fatal(err)
	}
	sourceThreadRef := "thread-history-batch"
	thread := model.AgentThread{
		PublicID: "agth_history_batch", UserID: 7, DeviceID: device.ID, RuntimeProfileID: profile.ID,
		WorkspaceID: workspace.ID, ConversationID: conversation.ID, SourceThreadRef: &sourceThreadRef,
		Status: "active", HistoryStatus: "loaded", HistoryVersion: historyProjectionVersion,
	}
	if err := database.Create(&thread).Error; err != nil {
		t.Fatal(err)
	}
	base := workspaceSession{
		SourceThreadRef: sourceThreadRef, Name: "All projects", Model: "gpt-5.6-sol", ReasoningEffort: "high",
		ApprovalPolicy: "never", ApprovalsReviewer: "user", SandboxPolicy: "danger-full-access",
		HistoryLoaded: true, HistoryProjectionVersion: historyProjectionVersion, HistoryDelta: true, HistoryBatchRef: "history_batch_0123456789abcdef", HistoryChunkCount: 2,
		UpdatedAt: now.Add(time.Second).Unix(),
	}
	chunks := []workspaceSession{base, base}
	chunks[0].Messages = []workspaceSessionMessage{{
		Role: "assistant", Status: "success", Content: "done", SourceTurnRef: "turn-source", SourceMessageRef: "message-assistant", CreatedAt: now.Unix(),
		ExecutionEvents: []workspaceSessionEvent{{Kind: "turn/started", SourceEventRef: "turn:started", Payload: json.RawMessage(`{"turn":{"status":"running"}}`)}},
	}}
	chunks[1].HistoryChunkIndex = 1
	chunks[1].Messages = []workspaceSessionMessage{{
		Role: "assistant", Status: "success", Content: "done", SourceTurnRef: "turn-source", SourceMessageRef: "message-assistant", CreatedAt: now.Unix(),
		ExecutionEvents: []workspaceSessionEvent{{Kind: "turn/completed", SourceEventRef: "turn:completed", Payload: json.RawMessage(`{"turn":{"status":"completed"}}`)}},
	}}
	repo := NewRepo(database)
	for index, chunk := range chunks {
		payload, err := json.Marshal(chunk)
		if err != nil {
			t.Fatal(err)
		}
		applied, err := repo.ApplyEventFrame(context.Background(), device.ID, profile.ID, uint64(index+1), strings.Repeat(fmt.Sprint(index+2), 64), &domainagent.Event{
			PublicID: fmt.Sprintf("agev_history_batch_%d", index), Kind: "thread/history/updated",
			SourceThreadRef: sourceThreadRef, PayloadJSON: string(payload), OccurredAt: now.Add(time.Duration(index+1) * time.Second),
		}, now.Add(time.Duration(index+1)*time.Second))
		if err != nil || applied.Acknowledged != uint64(index+1) {
			t.Fatalf("apply history chunk %d: %#v %v", index, applied, err)
		}
		if index == 0 && len(applied.ConversationPublicIDs) != 0 {
			t.Fatalf("partial batch notified the conversation: %#v", applied)
		}
		if index == 1 && !slices.Equal(applied.ConversationPublicIDs, []string{conversation.PublicID}) {
			t.Fatalf("completed batch notification = %#v", applied)
		}
	}
	var messages []model.Message
	if err := database.Where("conversation_id = ?", conversation.ID).Find(&messages).Error; err != nil || len(messages) != 1 ||
		messages[0].Content != "done" || messages[0].Status != "success" {
		t.Fatalf("atomic history messages = %#v err=%v", messages, err)
	}
	var events []model.ConversationExecutionEvent
	if err := database.Where("conversation_id = ?", conversation.ID).Order("seq ASC").Find(&events).Error; err != nil ||
		len(events) != 2 || events[0].Kind != "turn/started" || events[1].Kind != "turn/completed" {
		t.Fatalf("atomic history events = %#v err=%v", events, err)
	}
	if err := database.First(&conversation, conversation.ID).Error; err != nil || conversation.Model != "gpt-5.6-sol" ||
		conversation.ReasoningEffort != "high" || conversation.ApprovalPolicy != "never" || conversation.SandboxPolicy != "danger-full-access" {
		t.Fatalf("history settings = %#v err=%v", conversation, err)
	}
	var staged int64
	if err := database.Model(&model.AgentEvent{}).Where("thread_id = ? AND conversation_projected_at IS NULL", thread.ID).Count(&staged).Error; err != nil || staged != 0 {
		t.Fatalf("staged history chunks = %d err=%v", staged, err)
	}

	outdated := base
	outdated.HistoryProjectionVersion = 0
	outdated.HistoryBatchRef = "history_batch_outdated_producer"
	outdated.HistoryChunkCount = 1
	outdated.Messages = []workspaceSessionMessage{{
		Role: "assistant", Status: "interrupted", Content: "premature terminal", SourceTurnRef: "turn-source",
		SourceMessageRef: "message-assistant", CreatedAt: now.Unix(),
	}}
	payload, err := json.Marshal(outdated)
	if err != nil {
		t.Fatal(err)
	}
	applied, err := repo.ApplyEventFrame(context.Background(), device.ID, profile.ID, 3, strings.Repeat("4", 64), &domainagent.Event{
		PublicID: "agev_history_batch_outdated", Kind: agentprotocol.SessionHistoryEventKind,
		SourceThreadRef: sourceThreadRef, PayloadJSON: string(payload), OccurredAt: now.Add(3 * time.Second),
	}, now.Add(3*time.Second))
	if err != nil || applied.Acknowledged != 3 || len(applied.ConversationPublicIDs) != 0 {
		t.Fatalf("outdated history event was not ignored: %#v %v", applied, err)
	}
	if err := database.Where("conversation_id = ?", conversation.ID).Find(&messages).Error; err != nil || len(messages) != 1 ||
		messages[0].Content != "done" || messages[0].Status != "success" {
		t.Fatalf("outdated history event changed messages: %#v err=%v", messages, err)
	}
}

func TestWorkspaceHistoryDeltaBeforeCatalogSnapshotIsAcknowledged(t *testing.T) {
	database := testutil.Postgres(t)
	if err := database.AutoMigrate(
		&model.AgentDevice{}, &model.AgentRuntimeProfile{}, &model.AgentBridgeFrame{}, &model.AgentEvent{},
		&model.AgentWorkspace{}, &model.AgentThread{}, &model.AgentTurn{}, &model.Conversation{},
		&model.Message{}, &model.Attachment{}, &model.ConversationRun{}, &model.ConversationExecutionEvent{},
	); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	device := model.AgentDevice{
		PublicID: "agd_history_before_catalog", UserID: 7, Name: "desktop", Platform: "windows",
		PublicKey: []byte("public-key"), PublicKeyFingerprint: strings.Repeat("2", 64),
		CredentialVersion: 1, Status: domainagent.DeviceStatusActive,
	}
	if err := database.Create(&device).Error; err != nil {
		t.Fatal(err)
	}
	profile := model.AgentRuntimeProfile{PublicID: "codex-history-before-catalog", UserID: 7, DeviceID: device.ID, Provider: "codex", Status: "ready"}
	if err := database.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}
	session := workspaceSession{
		SourceThreadRef: "thread-before-catalog", HistoryLoaded: true, HistoryProjectionVersion: historyProjectionVersion, HistoryDelta: true,
		HistoryBatchRef: "history_before_catalog", HistoryChunkCount: 1,
		Messages: []workspaceSessionMessage{{
			Role: "assistant", Status: "interrupted", SourceTurnRef: "turn-before-catalog",
			SourceMessageRef: "message-before-catalog",
		}},
	}
	payload, err := json.Marshal(session)
	if err != nil {
		t.Fatal(err)
	}
	event := domainagent.Event{
		PublicID: "agev_history_before_catalog", UserID: 7, DeviceID: device.ID, RuntimeProfileID: &profile.ID,
		Kind: agentprotocol.SessionHistoryEventKind, SourceThreadRef: session.SourceThreadRef,
		PayloadJSON: string(payload), OccurredAt: now,
	}
	applied, err := NewRepo(database).ApplyEventFrame(context.Background(), device.ID, profile.ID, 1, strings.Repeat("a", 64), &event, now)
	if err != nil || applied.Acknowledged != 1 {
		t.Fatalf("early history delta: applied=%#v err=%v", applied, err)
	}
	var stored model.AgentEvent
	if err := database.Where("public_id = ?", event.PublicID).First(&stored).Error; err != nil ||
		stored.ThreadID != nil || stored.WorkspaceID != nil || stored.ConversationProjectedAt == nil {
		t.Fatalf("stored early history delta = %#v err=%v", stored, err)
	}
}

func TestWorkspaceHistoryDeltaForMissingThreadIsAcknowledged(t *testing.T) {
	database := testutil.Postgres(t)
	if err := database.AutoMigrate(
		&model.AgentDevice{}, &model.AgentRuntimeProfile{}, &model.AgentBridgeFrame{}, &model.AgentEvent{},
		&model.AgentWorkspace{}, &model.AgentThread{}, &model.AgentTurn{}, &model.Conversation{},
		&model.Message{}, &model.Attachment{}, &model.ConversationRun{}, &model.ConversationExecutionEvent{},
	); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 2, 13, 0, 0, 0, time.UTC)
	device := model.AgentDevice{
		PublicID: "agd_history_missing_thread", UserID: 7, Name: "desktop", Platform: "windows",
		PublicKey: []byte("public-key"), PublicKeyFingerprint: strings.Repeat("3", 64),
		CredentialVersion: 1, Status: domainagent.DeviceStatusActive,
	}
	if err := database.Create(&device).Error; err != nil {
		t.Fatal(err)
	}
	profile := model.AgentRuntimeProfile{PublicID: "codex-history-missing-thread", UserID: 7, DeviceID: device.ID, Provider: "codex", Status: "ready"}
	if err := database.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}
	workspace := model.AgentWorkspace{PublicID: "workspace-history-missing-thread", UserID: 7, DeviceID: device.ID, RuntimeProfileID: profile.ID, Status: "available"}
	if err := database.Create(&workspace).Error; err != nil {
		t.Fatal(err)
	}
	conversation := model.Conversation{
		PublicID: "conversation_history_missing", UserID: 7, Title: "Missing", ExecutionType: "gateway", Status: "active",
		BaseModel: model.BaseModel{CreatedAt: now, UpdatedAt: now},
	}
	if err := database.Create(&conversation).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Delete(&conversation).Error; err != nil {
		t.Fatal(err)
	}
	sourceThreadRef := "thread-history-missing"
	thread := model.AgentThread{
		PublicID: "agth_history_missing", UserID: 7, DeviceID: device.ID, RuntimeProfileID: profile.ID,
		WorkspaceID: workspace.ID, ConversationID: conversation.ID, SourceThreadRef: &sourceThreadRef,
		Status: threadStatusMissing, HistoryStatus: "loaded", HistoryVersion: historyProjectionVersion,
	}
	if err := database.Create(&thread).Error; err != nil {
		t.Fatal(err)
	}
	session := workspaceSession{
		SourceThreadRef: sourceThreadRef, HistoryLoaded: true, HistoryProjectionVersion: historyProjectionVersion, HistoryDelta: true,
		HistoryBatchRef: "history_missing_thread", HistoryChunkCount: 1,
		Messages: []workspaceSessionMessage{{
			Role: "assistant", Status: "interrupted", SourceTurnRef: "turn-history-missing",
			SourceMessageRef: "message-history-missing",
		}},
	}
	payload, err := json.Marshal(session)
	if err != nil {
		t.Fatal(err)
	}
	event := domainagent.Event{
		PublicID: "agev_history_missing_thread", UserID: 7, DeviceID: device.ID, RuntimeProfileID: &profile.ID,
		Kind: agentprotocol.SessionHistoryEventKind, SourceThreadRef: sourceThreadRef,
		PayloadJSON: string(payload), OccurredAt: now,
	}
	applied, err := NewRepo(database).ApplyEventFrame(context.Background(), device.ID, profile.ID, 1, strings.Repeat("b", 64), &event, now)
	if err != nil || applied.Acknowledged != 1 {
		t.Fatalf("missing thread history delta: applied=%#v err=%v", applied, err)
	}
	var stored model.AgentEvent
	if err := database.Where("public_id = ?", event.PublicID).First(&stored).Error; err != nil || stored.ConversationProjectedAt == nil {
		t.Fatalf("stored missing thread delta = %#v err=%v", stored, err)
	}
	if err := database.Model(&thread).Update("status", "active").Error; err != nil {
		t.Fatal(err)
	}
	session.HistoryBatchRef = "history_active_deleted_conversation"
	payload, err = json.Marshal(session)
	if err != nil {
		t.Fatal(err)
	}
	event.PublicID = "agev_history_active_deleted_conversation"
	event.PayloadJSON = string(payload)
	applied, err = NewRepo(database).ApplyEventFrame(context.Background(), device.ID, profile.ID, 2, strings.Repeat("c", 64), &event, now.Add(time.Second))
	if err != nil || applied.Acknowledged != 2 {
		t.Fatalf("active thread with deleted conversation history delta: applied=%#v err=%v", applied, err)
	}
	if err := database.Where("public_id = ?", event.PublicID).First(&stored).Error; err != nil || stored.ConversationProjectedAt == nil {
		t.Fatalf("stored active/deleted history delta = %#v err=%v", stored, err)
	}
}

func TestSessionHistoryBatchRejectsDuplicateEventReferencesWithinChunk(t *testing.T) {
	event := workspaceSessionEvent{
		Kind: "item/completed", SourceEventRef: "item-event",
		Payload: json.RawMessage(`{"item":{"kind":"commandExecution","status":"completed"}}`),
	}
	chunk := workspaceSession{
		SourceThreadRef: "thread-history-duplicate", Status: "active", HistoryLoaded: true, HistoryProjectionVersion: historyProjectionVersion, HistoryDelta: true,
		HistoryBatchRef: "history_batch_duplicate", HistoryChunkCount: 1,
		Messages: []workspaceSessionMessage{{
			Role: "assistant", Status: "success", Content: "done", SourceTurnRef: "turn-source",
			SourceMessageRef: "message-assistant", ExecutionEvents: []workspaceSessionEvent{event, event},
		}},
	}
	if _, err := mergeWorkspaceSessionHistoryChunks([]workspaceSession{chunk}); !errors.Is(err, repository.ErrConflict) {
		t.Fatalf("duplicate event references were accepted: %v", err)
	}
}

func TestGetCommandReportsDevicePresence(t *testing.T) {
	database := testutil.Postgres(t)
	if err := database.AutoMigrate(&model.AgentDevice{}, &model.AgentRuntimeProfile{}, &model.AgentCommand{}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	device := model.AgentDevice{
		PublicID: "agd_command_presence", UserID: 7, Name: "desktop", Platform: "windows",
		PublicKey: []byte(strings.Repeat("k", 32)), PublicKeyFingerprint: strings.Repeat("c", 64),
		CredentialVersion: 1, Status: domainagent.DeviceStatusActive, NextServerSeq: 2,
	}
	if err := database.Create(&device).Error; err != nil {
		t.Fatal(err)
	}
	expiresAt := now.Add(time.Minute)
	profile := model.AgentRuntimeProfile{
		PublicID: "codex-command-presence", UserID: 7, DeviceID: device.ID, Provider: "codex",
		Status: domainagent.RuntimeStatusReady, LeaseExpiresAt: &expiresAt, PresenceExpiresAt: &expiresAt,
	}
	if err := database.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}
	command := model.AgentCommand{
		PublicID: "agcmd_command_presence", UserID: 7, DeviceID: device.ID, RuntimeProfileID: &profile.ID,
		ServerSeq: 1, Kind: "resource.refresh", PayloadJSON: `{}`, State: "acked", AckedAt: &now,
	}
	if err := database.Create(&command).Error; err != nil {
		t.Fatal(err)
	}
	repo := NewRepo(database)
	loaded, err := repo.GetCommand(context.Background(), 7, command.PublicID)
	if err != nil || !loaded.DeviceOnline {
		t.Fatalf("online command = %#v, %v", loaded, err)
	}

	expiredAt := now.Add(-time.Minute)
	if err := database.Model(&profile).Updates(map[string]any{"presence_expires_at": expiredAt}).Error; err != nil {
		t.Fatal(err)
	}
	loaded, err = repo.GetCommand(context.Background(), 7, command.PublicID)
	if err != nil || loaded.DeviceOnline {
		t.Fatalf("offline command = %#v, %v", loaded, err)
	}
}

func TestWorkspaceSessionProjectionTracksActiveAssistantAndEventRevisions(t *testing.T) {
	active := workspaceSessionMessage{Role: "assistant", Status: "pending", ExecutionEvents: []workspaceSessionEvent{{
		Kind: "turn/started", SourceEventRef: "turn-started", Payload: json.RawMessage(`{"turn":{"status":"running"}}`),
	}}}
	if status := workspaceSessionMessageStatus(active); status != "pending" {
		t.Fatalf("active assistant status = %q", status)
	}
	completed := active
	completed.Status = "success"
	completed.ExecutionEvents = append(completed.ExecutionEvents, workspaceSessionEvent{
		Kind: "turn/completed", SourceEventRef: "turn-completed", Payload: json.RawMessage(`{"turn":{"status":"completed"}}`),
	})
	if status := workspaceSessionMessageStatus(completed); status != "success" {
		t.Fatalf("completed assistant status = %q", status)
	}
	interrupted := completed
	interrupted.Status = "interrupted"
	interrupted.SourceTurnRef = "turn-interrupted"
	interrupted.SourceMessageRef = "message-interrupted"
	if !validWorkspaceSession(workspaceSession{
		SourceThreadRef: "thread-interrupted", HistoryLoaded: true, HistoryProjectionVersion: historyProjectionVersion, Messages: []workspaceSessionMessage{interrupted},
	}, false) {
		t.Fatal("interrupted assistant history was rejected")
	}
	statusOnlyDelta := workspaceSession{
		SourceThreadRef: "thread-interrupted", HistoryLoaded: true, HistoryProjectionVersion: historyProjectionVersion, HistoryDelta: true,
		HistoryBatchRef: "history_interrupted", HistoryChunkCount: 1,
		Messages: []workspaceSessionMessage{{
			Role: "assistant", Status: "interrupted", SourceTurnRef: "turn-interrupted",
			SourceMessageRef: "message-interrupted",
		}},
	}
	if !validWorkspaceSession(statusOnlyDelta, false) {
		t.Fatal("status-only interrupted assistant delta was rejected")
	}
	statusOnlyDelta.Messages[0].Status = "success"
	if !validWorkspaceSession(statusOnlyDelta, false) {
		t.Fatal("status-only successful assistant delta was rejected")
	}
	statusOnlyDelta.HistoryDelta = false
	statusOnlyDelta.HistoryBatchRef = ""
	statusOnlyDelta.HistoryChunkCount = 0
	if validWorkspaceSession(statusOnlyDelta, false) {
		t.Fatal("empty assistant was accepted outside a history delta")
	}
	first := workspaceSessionEventSourceKey("conversation_test", "run_test", active.ExecutionEvents[0])
	replayed := workspaceSessionEventSourceKey("conversation_test", "run_test", active.ExecutionEvents[0])
	revised := workspaceSessionEventSourceKey("conversation_test", "run_test", workspaceSessionEvent{
		Kind: "turn/started", SourceEventRef: "turn-started", Payload: json.RawMessage(`{"turn":{"status":"running","progress":1}}`),
	})
	if first != replayed || first == revised {
		t.Fatalf("event source keys are not payload-revision stable: %q %q %q", first, replayed, revised)
	}
}

func TestWorkspaceHistoryDoesNotReplaceActiveGatewayMessages(t *testing.T) {
	database := testutil.Postgres(t)
	if err := database.AutoMigrate(&model.Conversation{}, &model.Message{}, &model.ConversationRun{}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 28, 2, 0, 0, 0, time.UTC)
	conversation := model.Conversation{
		UserID: 7, PublicID: "active_gateway_history", Title: "Active work", LabelsJSON: "[]",
		ExecutionType: "gateway", ExecutionWorkspaceID: "workspace-active", SessionKey: "active_gateway_history", Status: "active",
	}
	if err := database.Create(&conversation).Error; err != nil {
		t.Fatal(err)
	}
	messages := []model.Message{
		{ConversationID: conversation.ID, UserID: 7, PublicID: "active_gateway_user", Role: "user", ContentType: "text", Content: "new work", BranchReason: "default", Status: "pending", RunID: "run_active_history"},
		{ConversationID: conversation.ID, UserID: 7, PublicID: "active_gateway_assistant", Role: "assistant", ContentType: "text", BranchReason: "default", Status: "pending", RunID: "run_active_history"},
	}
	if err := database.Create(&messages).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&model.ConversationRun{
		RunID: "run_active_history", UserID: 7, ConversationID: conversation.ID, TaskType: "agent",
		Endpoint: "local_gateway", ProviderProtocol: "local_gateway", Status: "queued", StartedAt: now,
	}).Error; err != nil {
		t.Fatal(err)
	}
	thread := model.AgentThread{UserID: 7, ConversationID: conversation.ID, WorkspaceID: 9, Status: "active"}
	workspace := model.AgentWorkspace{ControlPlaneModel: model.ControlPlaneModel{ID: 9}, PublicID: "workspace-active"}
	_, err := syncExistingWorkspaceSession(database, &thread, &workspace, workspaceSession{
		SourceThreadRef: "source-thread", Name: conversation.Title, Status: "active", HistoryLoaded: true, HistoryProjectionVersion: historyProjectionVersion,
		Messages: []workspaceSessionMessage{{Role: "user", Status: "success", Content: "older history", SourceTurnRef: "source-turn"}},
	}, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	var stored []model.Message
	if err := database.Where("conversation_id = ?", conversation.ID).Order("id ASC").Find(&stored).Error; err != nil || len(stored) != 2 {
		t.Fatalf("active Gateway messages were replaced: %#v %v", stored, err)
	}
	if stored[0].Content != "new work" || stored[0].Status != "pending" || stored[1].Status != "pending" {
		t.Fatalf("active Gateway messages changed: %#v", stored)
	}
}

func TestQueueTurnInterruptImmediatelyTerminalizesLocalTurn(t *testing.T) {
	database := testutil.Postgres(t)
	if err := database.AutoMigrate(
		&model.AgentDevice{}, &model.AgentRuntimeProfile{}, &model.AgentWorkspace{}, &model.AgentThread{},
		&model.AgentTurn{}, &model.AgentInteraction{}, &model.AgentCommand{}, &model.AgentIdempotencyRecord{},
	); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 28, 1, 0, 0, 0, time.UTC)
	lease := now.Add(time.Minute)
	device := model.AgentDevice{
		PublicID: "agd_interrupt", UserID: 7, Name: "desktop", Platform: "windows",
		PublicKey: []byte("public-key"), PublicKeyFingerprint: strings.Repeat("1", 64),
		CredentialVersion: 1, Status: domainagent.DeviceStatusActive, NextServerSeq: 1,
	}
	if err := database.Create(&device).Error; err != nil {
		t.Fatal(err)
	}
	profile := model.AgentRuntimeProfile{
		PublicID: "profile-interrupt", UserID: 7, DeviceID: device.ID, Provider: "codex",
		Status: domainagent.RuntimeStatusReady, ManifestJSON: "{}", LeaseExpiresAt: &lease,
	}
	if err := database.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}
	workspace := model.AgentWorkspace{
		PublicID: "workspace-interrupt", UserID: 7, DeviceID: device.ID, RuntimeProfileID: profile.ID,
		Name: "project", Status: "available", LastSeenAt: now,
	}
	if err := database.Create(&workspace).Error; err != nil {
		t.Fatal(err)
	}
	threadRef, turnRef := "source-thread", "source-turn"
	thread := model.AgentThread{
		PublicID: "agth_interrupt", UserID: 7, DeviceID: device.ID, RuntimeProfileID: profile.ID,
		WorkspaceID: workspace.ID, ConversationID: 1, SourceThreadRef: &threadRef, Status: "active",
	}
	if err := database.Create(&thread).Error; err != nil {
		t.Fatal(err)
	}
	turn := model.AgentTurn{
		PublicID: "agturn_interrupt", UserID: 7, ThreadID: thread.ID, RunID: "run_interrupt",
		SourceTurnRef: &turnRef, Status: "running", InputJSON: "[]", SettingsJSON: "{}",
	}
	if err := database.Create(&turn).Error; err != nil {
		t.Fatal(err)
	}
	interaction := model.AgentInteraction{
		PublicID: "agint_interrupt", UserID: 7, ThreadID: thread.ID, TurnID: &turn.ID,
		RuntimeProfileID: profile.ID, SourceRequestRef: "request-interrupt", Kind: "command_approval",
		RequestJSON: "{}", Status: "pending",
	}
	if err := database.Create(&interaction).Error; err != nil {
		t.Fatal(err)
	}
	repo := NewRepo(database)
	queued, err := repo.QueueTurnInterrupt(
		context.Background(), "interrupt-key", "interrupt-hash", 7, turn.PublicID,
		&domainagent.Command{PublicID: "agcmd_interrupt", Kind: "turn.interrupt"}, now,
	)
	if err != nil || queued.State != "queued" {
		t.Fatalf("queue interrupt = %#v, %v", queued, err)
	}
	if err := database.First(&turn, turn.ID).Error; err != nil || turn.Status != "interrupted" {
		t.Fatalf("interrupted turn = %#v, %v", turn, err)
	}
	if err := database.First(&interaction, interaction.ID).Error; err != nil || interaction.Status != "resolved" {
		t.Fatalf("resolved interaction = %#v, %v", interaction, err)
	}
	if err := updateAgentTurnTerminal(database, turn.ID, "completed", "", "", now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := database.First(&turn, turn.ID).Error; err != nil || turn.Status != "interrupted" {
		t.Fatalf("late completion changed interrupted turn = %#v, %v", turn, err)
	}
}

func TestDeviceEnrollmentIsIdempotentButDoesNotRestoreRevokedDevice(t *testing.T) {
	database := testutil.Postgres(t)
	if err := database.AutoMigrate(
		&model.Conversation{}, &model.AgentDevice{}, &model.AgentDeviceEnrollmentChallenge{}, &model.AgentCredential{},
		&model.AgentThread{}, &model.AgentTurn{}, &model.AgentInteraction{}, &model.AgentCommand{},
	); err != nil {
		t.Fatalf("migrate device enrollment tables: %v", err)
	}
	repo := NewRepo(database)
	now := time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC)
	publicKey := bytes.Repeat([]byte("k"), 32)
	fingerprint := strings.Repeat("a", 64)
	newChallenge := func(publicID string) *domainagent.DeviceEnrollmentChallenge {
		return &domainagent.DeviceEnrollmentChallenge{
			PublicID: publicID, UserID: 7, UserPublicID: strings.Repeat("b", 32), RemoteUserID: 9,
			Name: "desktop", Platform: "windows", PublicKey: publicKey,
			PublicKeyFingerprint: fingerprint, Nonce: strings.Repeat("n", 32), ExpiresAt: now.Add(time.Minute),
		}
	}
	challenge := newChallenge("age_0123456789abcdef0123456789abcdef")
	if err := repo.CreateEnrollmentChallenge(context.Background(), challenge); err != nil {
		t.Fatalf("create enrollment challenge: %v", err)
	}
	input := &domainagent.Device{
		PublicID: "agd_0123456789abcdef0123456789abcdef", UserID: 7, Name: "desktop", Platform: "windows",
		PublicKey: publicKey, PublicKeyFingerprint: fingerprint, CredentialVersion: 1,
		Status: domainagent.DeviceStatusActive, NextServerSeq: 1,
	}
	created, err := repo.ConsumeEnrollmentChallengeAndEnroll(context.Background(), challenge.ID, input, now)
	if err != nil {
		t.Fatalf("enroll device: %v", err)
	}
	replayed, err := repo.ConsumeEnrollmentChallengeAndEnroll(context.Background(), challenge.ID, input, now)
	if err != nil || replayed.PublicID != created.PublicID {
		t.Fatalf("active enrollment replay changed identity: %#v %v", replayed, err)
	}
	conversation := model.Conversation{
		UserID: 7, PublicID: strings.Repeat("c", 32), Title: "Work", LabelsJSON: "[]", ExecutionType: "gateway",
		ExecutionDeviceID: created.PublicID, SessionKey: strings.Repeat("c", 32), Status: "active", ContextPolicy: "{}",
	}
	if err := database.Create(&conversation).Error; err != nil {
		t.Fatal(err)
	}
	threadRef := "source-thread"
	thread := model.AgentThread{
		PublicID: "agth_0123456789abcdef0123456789abcdef", UserID: 7, DeviceID: created.ID,
		RuntimeProfileID: 1, WorkspaceID: 1, ConversationID: conversation.ID, SourceThreadRef: &threadRef,
		Title: "Work", Status: threadStatusDeletingActive,
	}
	if err := database.Create(&thread).Error; err != nil {
		t.Fatal(err)
	}
	turn := model.AgentTurn{
		PublicID: "agturn_0123456789abcdef0123456789abcdef", UserID: 7, ThreadID: thread.ID,
		RunID: strings.Repeat("d", 32), Status: "running", InputJSON: "[]", SettingsJSON: "{}",
	}
	if err := database.Create(&turn).Error; err != nil {
		t.Fatal(err)
	}
	interaction := model.AgentInteraction{
		PublicID: "agint_0123456789abcdef0123456789abcdef", UserID: 7, ThreadID: thread.ID,
		RuntimeProfileID: 1, SourceRequestRef: "request-1", Kind: "approval", RequestJSON: "{}", Status: "pending",
	}
	if err := database.Create(&interaction).Error; err != nil {
		t.Fatal(err)
	}
	command := model.AgentCommand{
		PublicID: "agcmd_0123456789abcdef0123456789abcdef", UserID: 7, DeviceID: created.ID,
		ThreadID: &thread.ID, ServerSeq: 1, Kind: "thread.lifecycle", PayloadJSON: `{"action":"delete"}`,
		State: "queued", TerminalJSON: "{}",
	}
	if err := database.Create(&command).Error; err != nil {
		t.Fatal(err)
	}
	if err := repo.RevokeDevice(context.Background(), 7, created.PublicID, now); err != nil {
		t.Fatalf("revoke device: %v", err)
	}
	if err := database.First(&thread, thread.ID).Error; err != nil || thread.Status != "active" {
		t.Fatalf("revoke did not restore deleting thread: %#v %v", thread, err)
	}
	if err := database.First(&turn, turn.ID).Error; err != nil || turn.Status != "failed" || turn.ErrorCode != "device_revoked" {
		t.Fatalf("revoke did not fail active turn: %#v %v", turn, err)
	}
	if err := database.First(&interaction, interaction.ID).Error; err != nil || interaction.Status != "failed" {
		t.Fatalf("revoke did not fail interaction: %#v %v", interaction, err)
	}
	if err := database.First(&command, command.ID).Error; err != nil || command.State != "failed" || command.CompletedAt == nil {
		t.Fatalf("revoke did not terminalize command: %#v %v", command, err)
	}
	retry := newChallenge("age_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err := repo.CreateEnrollmentChallenge(context.Background(), retry); err != nil {
		t.Fatalf("create retry challenge: %v", err)
	}
	if _, err := repo.ConsumeEnrollmentChallengeAndEnroll(context.Background(), retry.ID, input, now); err == nil {
		t.Fatal("revoked device was restored by enrollment")
	}
}

func TestRuntimeProofPersistsManifestSnapshot(t *testing.T) {
	database := testutil.Postgres(t)
	if err := database.AutoMigrate(
		&model.AgentDevice{}, &model.AgentRuntimeProfile{}, &model.AgentRuntimeProofChallenge{},
	); err != nil {
		t.Fatalf("migrate runtime profile tables: %v", err)
	}
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	device := model.AgentDevice{
		PublicID: "agd_0123456789abcdef0123456789abcdef", UserID: 7, Name: "desktop", Platform: "linux",
		PublicKey: bytes.Repeat([]byte("k"), 32), PublicKeyFingerprint: strings.Repeat("a", 64),
		CredentialVersion: 1, Status: domainagent.DeviceStatusActive, NextServerSeq: 1,
	}
	if err := database.Create(&device).Error; err != nil {
		t.Fatal(err)
	}
	repo := NewRepo(database)
	profile, challenge, err := repo.BeginRuntimeProof(context.Background(), device.ID, "codex-default",
		&domainagent.RuntimeProfile{PublicID: "codex-default", Provider: "codex", Status: domainagent.RuntimeStatusProving},
		&domainagent.RuntimeProofChallenge{PublicID: "agp_0123456789abcdef0123456789abcdef", Nonce: "nonce", ExpiresAt: now.Add(time.Minute)}, now)
	if err != nil {
		t.Fatalf("begin runtime proof: %v", err)
	}
	manifest := `{"provider":"codex","runtimeVersion":"0.147.0"}`
	if err := repo.CompleteRuntimeProof(context.Background(), device.ID, profile.ID, challenge.ID, 31, strings.Repeat("b", 64), manifest, now, now.Add(10*time.Minute)); err != nil {
		t.Fatalf("complete runtime proof: %v", err)
	}
	profiles, err := repo.ListRuntimeProfiles(context.Background(), 7, device.PublicID)
	if err != nil || len(profiles) != 1 || !jsonEqual(profiles[0].ManifestJSON, manifest) || profiles[0].Status != domainagent.RuntimeStatusReady {
		t.Fatalf("runtime profile manifest mismatch: %#v %v", profiles, err)
	}
}

func TestEmptyWorkspaceSyncRemovesStaleProjects(t *testing.T) {
	database := testutil.Postgres(t)
	if err := database.AutoMigrate(&model.Conversation{}, &model.AgentDevice{}, &model.AgentRuntimeProfile{}, &model.AgentWorkspace{}); err != nil {
		t.Fatalf("migrate workspace tables: %v", err)
	}
	now := time.Date(2026, 8, 15, 2, 0, 0, 0, time.UTC)
	device := model.AgentDevice{
		PublicID: "agd_0123456789abcdef0123456789abcdef", UserID: 7, Name: "desktop", Platform: "windows",
		PublicKey: bytes.Repeat([]byte("k"), 32), PublicKeyFingerprint: strings.Repeat("a", 64),
		CredentialVersion: 1, Status: domainagent.DeviceStatusActive, NextServerSeq: 1,
	}
	if err := database.Create(&device).Error; err != nil {
		t.Fatal(err)
	}
	leaseExpiresAt := now.Add(time.Hour)
	profile := model.AgentRuntimeProfile{
		PublicID: "codex-default", UserID: 7, DeviceID: device.ID, Provider: "codex",
		Status: domainagent.RuntimeStatusReady, LeaseExpiresAt: &leaseExpiresAt, PresenceExpiresAt: &leaseExpiresAt,
		ManifestJSON: `{"commands":["thread.read","turn.steer"],"threadSettings":{"model":true,"reasoningEffort":["high"],"approvalPolicy":["on-request","never"],"approvalsReviewer":["user","auto_review"],"sandboxPolicy":["workspace-write","danger-full-access"]}}`,
	}
	if err := database.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}
	workspace := model.AgentWorkspace{
		PublicID: "workspace_old", UserID: 7, DeviceID: device.ID, RuntimeProfileID: profile.ID,
		Name: "install-directory", Status: "available", LastSeenAt: now.Add(-time.Hour),
	}
	if err := database.Create(&workspace).Error; err != nil {
		t.Fatal(err)
	}
	repo := NewRepo(database)
	if err := repo.SyncWorkspaces(context.Background(), 7, device.ID, profile.ID, nil, now); err != nil {
		t.Fatal(err)
	}
	items, err := repo.ListWorkspaces(context.Background(), 7, device.PublicID)
	if err != nil || len(items) != 0 {
		t.Fatalf("stale project remained visible: %#v %v", items, err)
	}
}

func TestListWorkspacesHidesRecentRuntimeWorkspace(t *testing.T) {
	database := testutil.Postgres(t)
	if err := database.AutoMigrate(&model.Conversation{}, &model.AgentDevice{}, &model.AgentRuntimeProfile{}, &model.AgentWorkspace{}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 19, 8, 0, 0, 0, time.UTC)
	device := model.AgentDevice{PublicID: "agd_0123456789abcdef0123456789abcdef", UserID: 7, Name: "desktop", Platform: "windows", PublicKey: bytes.Repeat([]byte("k"), 32), PublicKeyFingerprint: strings.Repeat("a", 64), CredentialVersion: 1, Status: domainagent.DeviceStatusActive, NextServerSeq: 1}
	if err := database.Create(&device).Error; err != nil {
		t.Fatal(err)
	}
	lease := now.Add(time.Hour)
	profile := model.AgentRuntimeProfile{PublicID: "codex-default", UserID: 7, DeviceID: device.ID, Provider: "codex", Status: domainagent.RuntimeStatusReady, LeaseExpiresAt: &lease}
	if err := database.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}
	repo := NewRepo(database)
	items := []domainagent.Workspace{
		{PublicID: "workspace-project", Name: "Project"},
		{PublicID: "workspace-recent", Name: "Recent", Hidden: true},
	}
	if err := repo.SyncWorkspaces(context.Background(), 7, device.ID, profile.ID, items, now); err != nil {
		t.Fatal(err)
	}
	visible, err := repo.ListWorkspaces(context.Background(), 7, device.PublicID)
	if err != nil || len(visible) != 1 || visible[0].PublicID != "workspace-project" {
		t.Fatalf("visible workspaces = %#v, %v", visible, err)
	}
}

func TestListWorkspacesOrdersByActiveConversationActivity(t *testing.T) {
	database := testutil.Postgres(t)
	if err := database.AutoMigrate(&model.Conversation{}, &model.AgentDevice{}, &model.AgentRuntimeProfile{}, &model.AgentWorkspace{}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 22, 10, 0, 0, 0, time.UTC)
	device := model.AgentDevice{PublicID: "agd_0123456789abcdef0123456789abcdef", UserID: 7, Name: "desktop", Platform: "windows", PublicKey: bytes.Repeat([]byte("k"), 32), PublicKeyFingerprint: strings.Repeat("a", 64), CredentialVersion: 1, Status: domainagent.DeviceStatusActive, NextServerSeq: 1}
	if err := database.Create(&device).Error; err != nil {
		t.Fatal(err)
	}
	lease := now.Add(time.Hour)
	profile := model.AgentRuntimeProfile{PublicID: "codex-default", UserID: 7, DeviceID: device.ID, Provider: "codex", Status: domainagent.RuntimeStatusReady, LeaseExpiresAt: &lease}
	if err := database.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}
	olderWorkspace := model.AgentWorkspace{ControlPlaneModel: model.ControlPlaneModel{CreatedAt: now.Add(-4 * time.Hour), UpdatedAt: now}, PublicID: "workspace-a", UserID: 7, DeviceID: device.ID, RuntimeProfileID: profile.ID, Name: "A project", Status: "available", LastSeenAt: now}
	newerWorkspace := model.AgentWorkspace{ControlPlaneModel: model.ControlPlaneModel{CreatedAt: now.Add(-3 * time.Hour), UpdatedAt: now}, PublicID: "workspace-z", UserID: 7, DeviceID: device.ID, RuntimeProfileID: profile.ID, Name: "Z project", Status: "available", LastSeenAt: now}
	if err := database.Create(&olderWorkspace).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&newerWorkspace).Error; err != nil {
		t.Fatal(err)
	}
	conversations := []model.Conversation{
		{BaseModel: model.BaseModel{CreatedAt: now.Add(-2 * time.Hour), UpdatedAt: now.Add(-2 * time.Hour)}, UserID: 7, PublicID: "conversation-older", Title: "Older", LabelsJSON: "[]", ExecutionType: "gateway", ExecutionDeviceID: device.PublicID, ExecutionWorkspaceID: olderWorkspace.PublicID, SessionKey: "session-older", Status: "active", ContextPolicy: "{}"},
		{BaseModel: model.BaseModel{CreatedAt: now.Add(-time.Hour), UpdatedAt: now.Add(-time.Hour)}, UserID: 7, PublicID: "conversation-newer", Title: "Newer", LabelsJSON: "[]", ExecutionType: "gateway", ExecutionDeviceID: device.PublicID, ExecutionWorkspaceID: newerWorkspace.PublicID, SessionKey: "session-newer", Status: "active", ContextPolicy: "{}"},
		{BaseModel: model.BaseModel{CreatedAt: now, UpdatedAt: now}, UserID: 7, PublicID: "conversation-archived", Title: "Archived", LabelsJSON: "[]", ExecutionType: "gateway", ExecutionDeviceID: device.PublicID, ExecutionWorkspaceID: olderWorkspace.PublicID, SessionKey: "session-archived", Status: "archived", ContextPolicy: "{}"},
	}
	if err := database.Create(&conversations).Error; err != nil {
		t.Fatal(err)
	}

	items, err := NewRepo(database).ListWorkspaces(context.Background(), 7, device.PublicID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].PublicID != newerWorkspace.PublicID || items[1].PublicID != olderWorkspace.PublicID {
		t.Fatalf("workspace activity order = %#v", items)
	}
	if !items[0].LastActivityAt.Equal(now.Add(-time.Hour)) || !items[1].LastActivityAt.Equal(now.Add(-2*time.Hour)) {
		t.Fatalf("workspace activity timestamps = %#v", items)
	}
}

func TestThreadLifecycleProjectsDeleteAndArchiveStates(t *testing.T) {
	database := testutil.Postgres(t)
	if err := database.AutoMigrate(
		&model.Conversation{}, &model.AgentDevice{}, &model.AgentRuntimeProfile{}, &model.AgentWorkspace{},
		&model.AgentThread{}, &model.AgentTurn{}, &model.AgentEvent{}, &model.AgentCommand{}, &model.AgentBridgeFrame{}, &model.AgentIdempotencyRecord{},
	); err != nil {
		t.Fatalf("migrate thread delete tables: %v", err)
	}
	now := time.Date(2026, 8, 15, 4, 0, 0, 0, time.UTC)
	device := model.AgentDevice{
		PublicID: "agd_0123456789abcdef0123456789abcdef", UserID: 7, Name: "desktop", Platform: "windows",
		PublicKey: bytes.Repeat([]byte("k"), 32), PublicKeyFingerprint: strings.Repeat("a", 64),
		CredentialVersion: 1, Status: domainagent.DeviceStatusActive, NextServerSeq: 1,
	}
	if err := database.Create(&device).Error; err != nil {
		t.Fatal(err)
	}
	profile := model.AgentRuntimeProfile{
		PublicID: "codex-default", UserID: 7, DeviceID: device.ID, Provider: "codex", Status: domainagent.RuntimeStatusReady,
	}
	if err := database.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}
	workspace := model.AgentWorkspace{
		PublicID: "workspace-main", UserID: 7, DeviceID: device.ID, RuntimeProfileID: profile.ID,
		Name: "DEEIX-Chat", Status: "available", LastSeenAt: now,
	}
	if err := database.Create(&workspace).Error; err != nil {
		t.Fatal(err)
	}

	createThread := func(publicID, sourceRef, conversationPublicID string) (model.Conversation, model.AgentThread) {
		conversation := model.Conversation{
			UserID: 7, PublicID: conversationPublicID, Title: "Work", LabelsJSON: "[]", ExecutionType: "gateway",
			ExecutionDeviceID: device.PublicID, ExecutionProfileID: profile.PublicID, ExecutionWorkspaceID: workspace.PublicID,
			SessionKey: conversationPublicID, Status: "active", ContextPolicy: "{}",
		}
		if err := database.Create(&conversation).Error; err != nil {
			t.Fatal(err)
		}
		thread := model.AgentThread{
			PublicID: publicID, UserID: 7, DeviceID: device.ID, RuntimeProfileID: profile.ID, WorkspaceID: workspace.ID,
			ConversationID: conversation.ID, SourceThreadRef: &sourceRef, Title: "Work", Status: "active",
		}
		if err := database.Create(&thread).Error; err != nil {
			t.Fatal(err)
		}
		return conversation, thread
	}

	repo := NewRepo(database)
	conversation, thread := createThread("agth_0123456789abcdef0123456789abcdef", "source-thread-1", "0123456789abcdef0123456789abcdef")
	command, err := repo.QueueThreadLifecycle(
		context.Background(), "01234567-89ab-4def-8123-456789abcdef", strings.Repeat("1", 64), 7, thread.PublicID,
		"delete",
		&domainagent.Command{PublicID: "agcmd_0123456789abcdef0123456789abcdef", Kind: "thread.lifecycle"}, now,
	)
	if err != nil || command.State != "queued" || !jsonEqual(command.PayloadJSON, `{"kind":"thread.lifecycle","deviceId":"agd_0123456789abcdef0123456789abcdef","profileId":"codex-default","workspaceId":"workspace-main","threadId":"agth_0123456789abcdef0123456789abcdef","sourceThreadRef":"source-thread-1","action":"delete"}`) {
		t.Fatalf("queue thread delete: %#v %v", command, err)
	}
	if _, err := repo.QueueThreadLifecycle(
		context.Background(), "21234567-89ab-4def-8123-456789abcdef", strings.Repeat("5", 64), 7, thread.PublicID,
		"delete",
		&domainagent.Command{PublicID: "agcmd_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Kind: "thread.lifecycle"}, now,
	); err == nil {
		t.Fatal("duplicate pending thread delete was queued")
	}
	var visible model.Conversation
	if err := database.First(&visible, conversation.ID).Error; err != nil {
		t.Fatalf("queued thread delete hid the conversation before device confirmation: %v", err)
	}
	var deletingThread model.AgentThread
	if err := database.First(&deletingThread, thread.ID).Error; err != nil || deletingThread.Status != threadStatusDeletingActive {
		t.Fatalf("queued thread delete did not mark the thread deleting: %#v %v", deletingThread, err)
	}
	accepted := `{"kind":"result","result":{"kind":"accepted"}}`
	if ack, err := repo.ApplyTerminalFrame(context.Background(), device.ID, 1, command.ServerSeq, command.PublicID, strings.Repeat("2", 64), accepted, now.Add(time.Second)); err != nil || ack != 1 {
		t.Fatalf("apply delete terminal: ack=%d err=%v", ack, err)
	}
	var storedCommand model.AgentCommand
	if err := database.First(&storedCommand, command.ID).Error; err != nil || !jsonEqual(storedCommand.TerminalJSON, `{"kind":"result"}`) {
		t.Fatalf("successful terminal was not compacted: %#v %v", storedCommand, err)
	}
	var storedFrame model.AgentBridgeFrame
	if err := database.Where("device_id = ? AND bridge_seq = ?", device.ID, 1).First(&storedFrame).Error; err != nil || storedFrame.PayloadJSON != "{}" {
		t.Fatalf("terminal bridge payload was retained: %#v %v", storedFrame, err)
	}
	var deletedThread model.AgentThread
	if err := database.First(&deletedThread, thread.ID).Error; err != nil || deletedThread.Status != "deleted" {
		t.Fatalf("thread delete status was not projected: %#v %v", deletedThread, err)
	}
	var hidden model.Conversation
	if err := database.First(&hidden, conversation.ID).Error; err == nil {
		t.Fatal("confirmed thread delete left the conversation visible")
	}

	failedConversation, failedThread := createThread("agth_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "source-thread-2", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	failedCommand, err := repo.QueueThreadLifecycle(
		context.Background(), "11234567-89ab-4def-8123-456789abcdef", strings.Repeat("3", 64), 7, failedThread.PublicID,
		"delete",
		&domainagent.Command{PublicID: "agcmd_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Kind: "thread.lifecycle"}, now.Add(2*time.Second),
	)
	if err != nil {
		t.Fatal(err)
	}
	failed := `{"kind":"error","error":{"message":"device rejected delete"}}`
	if ack, err := repo.ApplyTerminalFrame(context.Background(), device.ID, 2, failedCommand.ServerSeq, failedCommand.PublicID, strings.Repeat("4", 64), failed, now.Add(3*time.Second)); err != nil || ack != 2 {
		t.Fatalf("apply failed delete terminal: ack=%d err=%v", ack, err)
	}
	var restored model.Conversation
	if err := database.First(&restored, failedConversation.ID).Error; err != nil {
		t.Fatalf("failed device delete hid the conversation: %v", err)
	}
	var restoredThread model.AgentThread
	if err := database.First(&restoredThread, failedThread.ID).Error; err != nil || restoredThread.Status != "active" {
		t.Fatalf("failed device delete did not restore thread: %#v %v", restoredThread, err)
	}

	pendingConversation, pendingThread := createThread("agth_cccccccccccccccccccccccccccccccc", "source-thread-3", "cccccccccccccccccccccccccccccccc")
	pendingEvent := model.AgentEvent{
		PublicID: "agev_0123456789abcdef0123456789abcdef", BridgeFrameID: 99, UserID: 7, DeviceID: device.ID,
		RuntimeProfileID: &profile.ID, WorkspaceID: &workspace.ID, ThreadID: &pendingThread.ID,
		Kind: "item/completed", SourceThreadRef: "source-thread-3", PayloadJSON: "{}", OccurredAt: now,
	}
	if err := database.Create(&pendingEvent).Error; err != nil {
		t.Fatal(err)
	}
	pendingCommand, err := repo.QueueThreadLifecycle(
		context.Background(), "31234567-89ab-4def-8123-456789abcdef", strings.Repeat("6", 64), 7, pendingThread.PublicID,
		"delete",
		&domainagent.Command{PublicID: "agcmd_cccccccccccccccccccccccccccccccc", Kind: "thread.lifecycle"}, now.Add(4*time.Second),
	)
	if err != nil {
		t.Fatal(err)
	}
	if ack, err := repo.ApplyTerminalFrame(context.Background(), device.ID, 3, pendingCommand.ServerSeq, pendingCommand.PublicID, strings.Repeat("7", 64), accepted, now.Add(5*time.Second)); err != nil || ack != 3 {
		t.Fatalf("apply pending-event delete terminal: ack=%d err=%v", ack, err)
	}
	var pendingVisible model.Conversation
	if err := database.First(&pendingVisible, pendingConversation.ID).Error; err != nil {
		t.Fatalf("delete finalized before pending event projection: %v", err)
	}
	if err := repo.MarkConversationEventProjected(context.Background(), pendingEvent.ID, now.Add(6*time.Second)); err != nil {
		t.Fatal(err)
	}
	var pendingHidden model.Conversation
	if err := database.First(&pendingHidden, pendingConversation.ID).Error; err == nil {
		t.Fatal("last event projection did not finalize the confirmed delete")
	}

	threadEventConversation, threadEventThread := createThread("agth_dddddddddddddddddddddddddddddddd", "source-thread-4", "dddddddddddddddddddddddddddddddd")
	threadEventCommand, err := repo.QueueThreadLifecycle(
		context.Background(), "41234567-89ab-4def-8123-456789abcdef", strings.Repeat("8", 64), 7, threadEventThread.PublicID,
		"delete",
		&domainagent.Command{PublicID: "agcmd_dddddddddddddddddddddddddddddddd", Kind: "thread.lifecycle"}, now.Add(7*time.Second),
	)
	if err != nil {
		t.Fatal(err)
	}
	threadEvent, err := repo.ApplyEventFrame(
		context.Background(), device.ID, profile.ID, 4, strings.Repeat("9", 64),
		&domainagent.Event{
			PublicID: "agev_dddddddddddddddddddddddddddddddd", UserID: 7, DeviceID: device.ID,
			RuntimeProfileID: &profile.ID, Kind: "thread/deleted", SourceThreadRef: "source-thread-4",
			PayloadJSON: "{}", OccurredAt: now.Add(8 * time.Second),
		},
		now.Add(8*time.Second),
	)
	if err != nil {
		t.Fatalf("thread-only event was left pending: %#v %v", threadEvent, err)
	}
	var storedThreadEvent model.AgentEvent
	if err := database.Where("public_id = ?", "agev_dddddddddddddddddddddddddddddddd").First(&storedThreadEvent).Error; err != nil || storedThreadEvent.ConversationProjectedAt == nil {
		t.Fatalf("thread-only event was not marked projected: %#v %v", storedThreadEvent, err)
	}
	if ack, err := repo.ApplyTerminalFrame(context.Background(), device.ID, 5, threadEventCommand.ServerSeq, threadEventCommand.PublicID, strings.Repeat("a", 64), accepted, now.Add(9*time.Second)); err != nil || ack != 5 {
		t.Fatalf("apply thread-event delete terminal: ack=%d err=%v", ack, err)
	}
	if err := database.First(&hidden, threadEventConversation.ID).Error; err == nil {
		t.Fatal("thread-only event blocked confirmed conversation deletion")
	}

	archiveConversation, archiveThread := createThread("agth_eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", "source-thread-5", "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee")
	archiveCommand, err := repo.QueueThreadLifecycle(
		context.Background(), "51234567-89ab-4def-8123-456789abcdef", strings.Repeat("b", 64), 7, archiveThread.PublicID,
		"archive",
		&domainagent.Command{PublicID: "agcmd_eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", Kind: "thread.lifecycle"}, now.Add(10*time.Second),
	)
	if err != nil {
		t.Fatal(err)
	}
	var archivedConversation model.Conversation
	var archivedThread model.AgentThread
	if err := database.First(&archivedConversation, archiveConversation.ID).Error; err != nil || archivedConversation.Status != "archived" {
		t.Fatalf("queued archive did not hide the conversation: %#v %v", archivedConversation, err)
	}
	if err := database.First(&archivedThread, archiveThread.ID).Error; err != nil || archivedThread.Status != "archived" {
		t.Fatalf("queued archive did not update the thread: %#v %v", archivedThread, err)
	}
	archiveFailed := `{"kind":"error","error":{"message":"device rejected archive"}}`
	if ack, err := repo.ApplyTerminalFrame(context.Background(), device.ID, 6, archiveCommand.ServerSeq, archiveCommand.PublicID, strings.Repeat("c", 64), archiveFailed, now.Add(11*time.Second)); err != nil || ack != 6 {
		t.Fatalf("apply failed archive terminal: ack=%d err=%v", ack, err)
	}
	if err := database.First(&archivedConversation, archiveConversation.ID).Error; err != nil || archivedConversation.Status != "active" {
		t.Fatalf("failed archive did not restore the conversation: %#v %v", archivedConversation, err)
	}

	archiveCommand, err = repo.QueueThreadLifecycle(
		context.Background(), "61234567-89ab-4def-8123-456789abcdef", strings.Repeat("d", 64), 7, archiveThread.PublicID,
		"archive",
		&domainagent.Command{PublicID: "agcmd_ffffffffffffffffffffffffffffffff", Kind: "thread.lifecycle"}, now.Add(12*time.Second),
	)
	if err != nil {
		t.Fatal(err)
	}
	if ack, err := repo.ApplyTerminalFrame(context.Background(), device.ID, 7, archiveCommand.ServerSeq, archiveCommand.PublicID, strings.Repeat("e", 64), accepted, now.Add(13*time.Second)); err != nil || ack != 7 {
		t.Fatalf("apply archive terminal: ack=%d err=%v", ack, err)
	}
	unarchiveCommand, err := repo.QueueThreadLifecycle(
		context.Background(), "71234567-89ab-4def-8123-456789abcdef", strings.Repeat("f", 64), 7, archiveThread.PublicID,
		"unarchive",
		&domainagent.Command{PublicID: "agcmd_11111111111111111111111111111111", Kind: "thread.lifecycle"}, now.Add(14*time.Second),
	)
	if err != nil {
		t.Fatal(err)
	}
	if ack, err := repo.ApplyTerminalFrame(context.Background(), device.ID, 8, unarchiveCommand.ServerSeq, unarchiveCommand.PublicID, strings.Repeat("1", 64), accepted, now.Add(15*time.Second)); err != nil || ack != 8 {
		t.Fatalf("apply unarchive terminal: ack=%d err=%v", ack, err)
	}
	if err := database.First(&archivedConversation, archiveConversation.ID).Error; err != nil || archivedConversation.Status != "active" {
		t.Fatalf("unarchive did not restore the conversation: %#v %v", archivedConversation, err)
	}

	if _, err := repo.ApplyEventFrame(
		context.Background(), device.ID, profile.ID, 9, strings.Repeat("2", 64),
		&domainagent.Event{
			PublicID: "agev_eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", UserID: 7, DeviceID: device.ID,
			RuntimeProfileID: &profile.ID, Kind: "thread/archived", SourceThreadRef: "source-thread-5",
			PayloadJSON: "{}", OccurredAt: now.Add(16 * time.Second),
		},
		now.Add(16*time.Second),
	); err != nil {
		t.Fatalf("apply local archive event: %v", err)
	}
	if err := database.First(&archivedConversation, archiveConversation.ID).Error; err != nil || archivedConversation.Status != "archived" {
		t.Fatalf("local archive event did not update the conversation: %#v %v", archivedConversation, err)
	}
	if _, err := repo.ApplyEventFrame(
		context.Background(), device.ID, profile.ID, 10, strings.Repeat("3", 64),
		&domainagent.Event{
			PublicID: "agev_ffffffffffffffffffffffffffffffff", UserID: 7, DeviceID: device.ID,
			RuntimeProfileID: &profile.ID, Kind: "thread/unarchived", SourceThreadRef: "source-thread-5",
			PayloadJSON: "{}", OccurredAt: now.Add(17 * time.Second),
		},
		now.Add(17*time.Second),
	); err != nil {
		t.Fatalf("apply local unarchive event: %v", err)
	}
	if err := database.First(&archivedConversation, archiveConversation.ID).Error; err != nil || archivedConversation.Status != "active" {
		t.Fatalf("local unarchive event did not update the conversation: %#v %v", archivedConversation, err)
	}
}

func TestThreadProjectionIsOrderedAndIdempotent(t *testing.T) {
	database := testutil.Postgres(t)
	if err := database.AutoMigrate(
		&model.FileObject{},
		&model.Conversation{}, &model.Message{}, &model.Attachment{}, &model.ConversationExecutionEvent{}, &model.ConversationRun{},
		&model.AgentDevice{}, &model.AgentCommand{}, &model.AgentBridgeFrame{},
		&model.AgentRuntimeProfile{}, &model.AgentWorkspace{}, &model.AgentArtifact{}, &model.AgentResourceSnapshot{}, &model.AgentThread{},
		&model.AgentTurn{}, &model.AgentItem{}, &model.AgentEvent{}, &model.AgentInteraction{},
		&model.AgentIdempotencyRecord{},
	); err != nil {
		t.Fatalf("migrate agent gateway tables: %v", err)
	}

	now := time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC)
	device := model.AgentDevice{
		PublicID: "agd_0123456789abcdef0123456789abcdef", UserID: 7,
		Name: "desktop", Platform: "windows",
		PublicKey: []byte(strings.Repeat("k", 32)), PublicKeyFingerprint: strings.Repeat("a", 64),
		CredentialVersion: 1, Status: domainagent.DeviceStatusActive, NextServerSeq: 1,
	}
	if err := database.Create(&device).Error; err != nil {
		t.Fatalf("create device: %v", err)
	}
	leaseExpiresAt := now.Add(10 * time.Minute)
	profile := model.AgentRuntimeProfile{
		PublicID: "codex-default", UserID: 7, DeviceID: device.ID, Provider: "codex",
		Status: domainagent.RuntimeStatusReady, LeaseExpiresAt: &leaseExpiresAt, PresenceExpiresAt: &leaseExpiresAt,
		ManifestJSON: `{"commands":["thread.read"],"threadSettings":{"model":true,"reasoningEffort":["high"],"approvalPolicy":["on-request"],"approvalsReviewer":["user","auto_review"],"sandboxPolicy":["workspace-write"]}}`,
	}
	if err := database.Create(&profile).Error; err != nil {
		t.Fatalf("create runtime profile: %v", err)
	}
	workspace := model.AgentWorkspace{
		PublicID: "workspace-main", UserID: 7, DeviceID: device.ID, RuntimeProfileID: profile.ID,
		Name: "DEEIX-Chat", Status: "available", LastSeenAt: now,
	}
	if err := database.Create(&workspace).Error; err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	conversation := model.Conversation{
		BaseModel: model.BaseModel{ID: 41}, UserID: 7,
		PublicID: "conversation-agent-work", Title: "Agent work", Status: "active",
		ExecutionType: "gateway", ExecutionDeviceID: device.PublicID,
		ExecutionProfileID: profile.PublicID, ExecutionWorkspaceID: workspace.PublicID,
		SessionKey: "agent-work-conversation-41",
	}
	if err := database.Create(&conversation).Error; err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	imageFile := model.FileObject{
		FileID: "file_0123456789abcdef0123456789abcdef", UserID: 7, Purpose: "agent_input",
		FileName: "fixture.png", MimeType: "image/png", DetectedMIME: "image/png",
		FileCategory: "image", SizeBytes: 12, SHA256: strings.Repeat("f", 64),
		StoragePath: "fixture.png", Status: "active",
	}
	if err := database.Create(&imageFile).Error; err != nil {
		t.Fatalf("create artifact file: %v", err)
	}
	videoFile := model.FileObject{
		FileID: "file_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", UserID: 7, Purpose: "agent_input",
		FileName: "fixture.mp4", MimeType: "video/mp4", DetectedMIME: "video/mp4",
		FileCategory: "video", SizeBytes: 12, SHA256: strings.Repeat("e", 64),
		StoragePath: "fixture.mp4", Status: "active",
	}
	if err := database.Create(&videoFile).Error; err != nil {
		t.Fatalf("create video artifact file: %v", err)
	}

	repo := NewRepo(database)
	artifact, err := repo.CreateArtifact(context.Background(), 7, workspace.PublicID, imageFile.FileID, &domainagent.Artifact{PublicID: "agart_0123456789abcdef0123456789abcdef"})
	if err != nil || artifact.FileID != imageFile.FileID || artifact.WorkspacePublicID != workspace.PublicID || artifact.MimeType != "image/png" {
		t.Fatalf("create artifact: %#v %v", artifact, err)
	}
	replayedArtifact, err := repo.CreateArtifact(context.Background(), 7, workspace.PublicID, imageFile.FileID, &domainagent.Artifact{PublicID: "agart_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"})
	if err != nil || replayedArtifact.PublicID != artifact.PublicID {
		t.Fatalf("artifact replay changed identity: %#v %v", replayedArtifact, err)
	}
	videoArtifact, err := repo.CreateArtifact(context.Background(), 7, workspace.PublicID, videoFile.FileID, &domainagent.Artifact{PublicID: "agart_cccccccccccccccccccccccccccccccc"})
	if err != nil || videoArtifact.FileID != videoFile.FileID || videoArtifact.MimeType != "video/mp4" {
		t.Fatalf("create video artifact: %#v %v", videoArtifact, err)
	}
	if _, err := repo.CreateArtifact(context.Background(), 8, workspace.PublicID, imageFile.FileID, &domainagent.Artifact{PublicID: "agart_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}); err == nil {
		t.Fatal("cross-user artifact creation succeeded")
	}
	if err := validateCommandArtifacts(database, 7, workspace.ID, `[{"kind":"artifact","artifactRef":"agart_0123456789abcdef0123456789abcdef"}]`); err != nil {
		t.Fatalf("valid command artifact rejected: %v", err)
	}
	if err := validateCommandArtifacts(database, 7, workspace.ID, `[{"kind":"artifact","artifactRef":"agart_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}]`); err == nil {
		t.Fatal("unknown command artifact accepted")
	}
	threadInput := &domainagent.Thread{PublicID: "agth_0123456789abcdef0123456789abcdef", UserID: 7, ConversationID: 41, Title: "Agent work", Status: "queued"}
	turnInput := &domainagent.Turn{
		PublicID: "agturn_0123456789abcdef0123456789abcdef", UserID: 7,
		RunID: "run_0123456789abcdef0123456789abcdef", Status: "awaiting_thread", InputJSON: `[{"kind":"text","text":"run tests"},{"kind":"artifact","artifactRef":"agart_0123456789abcdef0123456789abcdef"}]`, SettingsJSON: `{"model":"gpt-5.6-codex","reasoningEffort":"high","approvalPolicy":"on-request","approvalsReviewer":"auto_review","sandboxPolicy":"workspace-write"}`,
	}
	createCommand := &domainagent.Command{
		PublicID: "agcmd_0123456789abcdef0123456789abcdef", Kind: "thread.create",
		PayloadJSON: `{"kind":"thread.create","deviceId":"agd_0123456789abcdef0123456789abcdef","profileId":"codex-default","workspaceId":"workspace-main","settings":{"model":"gpt-5.6-codex","reasoningEffort":"high","approvalPolicy":"on-request","approvalsReviewer":"auto_review","sandboxPolicy":"workspace-write"}}`,
	}
	idempotencyKey, requestHash := "01234567-89ab-4def-8123-456789abcdef", strings.Repeat("1", 64)
	thread, turn, err := repo.StartThread(context.Background(), idempotencyKey, requestHash, threadInput, turnInput, createCommand, now)
	if err != nil || thread == nil || turn == nil {
		t.Fatalf("start thread: thread=%#v turn=%#v err=%v", thread, turn, err)
	}
	if err := database.First(&conversation, conversation.ID).Error; err != nil ||
		conversation.ApprovalPolicy != "on-request" || conversation.ApprovalsReviewer != "auto_review" || conversation.SandboxPolicy != "workspace-write" {
		t.Fatalf("conversation approval settings were not persisted: %#v %v", conversation, err)
	}
	replayedThread, replayedTurn, err := repo.StartThread(context.Background(), idempotencyKey, requestHash,
		&domainagent.Thread{PublicID: "agth_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", UserID: 7},
		&domainagent.Turn{PublicID: "agturn_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", UserID: 7},
		&domainagent.Command{PublicID: "agcmd_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, now)
	if err != nil || replayedThread.PublicID != thread.PublicID || replayedTurn.PublicID != turn.PublicID {
		t.Fatalf("idempotent thread replay changed result: %#v %#v %v", replayedThread, replayedTurn, err)
	}

	earlyCompleted := &domainagent.Event{
		PublicID: "agev_0123456789abcdef0123456789abcdef", Kind: "turn/completed",
		SourceThreadRef: "source-thread-1", SourceTurnRef: "source-turn-1",
		PayloadJSON: `{"turn":{"status":"completed","error":null}}`, OccurredAt: now.Add(time.Second),
	}
	if ack, err := repo.ApplyEventFrame(context.Background(), device.ID, profile.ID, 1, strings.Repeat("2", 64), earlyCompleted, now.Add(time.Second)); err != nil || ack.Acknowledged != 1 {
		t.Fatalf("apply early event: ack=%v err=%v", ack, err)
	}
	threadTerminal := `{"kind":"result","result":{"kind":"thread-created","sourceThreadRef":"source-thread-1"}}`
	if ack, err := repo.ApplyTerminalFrame(context.Background(), device.ID, 2, 1, createCommand.PublicID, strings.Repeat("3", 64), threadTerminal, now.Add(2*time.Second)); err != nil || ack != 2 {
		t.Fatalf("apply thread terminal: ack=%d err=%v", ack, err)
	}
	if ack, err := repo.ApplyTerminalFrame(context.Background(), device.ID, 2, 1, createCommand.PublicID, strings.Repeat("3", 64), threadTerminal, now.Add(2*time.Second)); err != nil || ack != 2 {
		t.Fatalf("replay thread terminal: ack=%d err=%v", ack, err)
	}

	var commands []model.AgentCommand
	if err := database.Order("server_seq ASC").Find(&commands).Error; err != nil || len(commands) != 2 {
		t.Fatalf("load commands after thread projection: count=%d err=%v", len(commands), err)
	}
	turnTerminal := `{"kind":"result","result":{"kind":"turn-started","sourceTurnRef":"source-turn-1"}}`
	if ack, err := repo.ApplyTerminalFrame(context.Background(), device.ID, 3, 2, commands[1].PublicID, strings.Repeat("4", 64), turnTerminal, now.Add(3*time.Second)); err != nil || ack != 3 {
		t.Fatalf("apply turn terminal: ack=%d err=%v", ack, err)
	}
	commandArtifacts, err := repo.ListArtifactsForCommand(context.Background(), device.ID, commands[1].ID, []string{artifact.PublicID})
	if err != nil || len(commandArtifacts) != 1 || commandArtifacts[0].FileID != imageFile.FileID {
		t.Fatalf("load command artifacts: %#v %v", commandArtifacts, err)
	}
	loadedArtifact, loadedCommand, err := repo.GetArtifactForCommand(context.Background(), artifact.PublicID, commands[1].PublicID)
	if err != nil || loadedArtifact.FileID != imageFile.FileID || loadedCommand.ID != commands[1].ID {
		t.Fatalf("load artifact grant binding: %#v %#v %v", loadedArtifact, loadedCommand, err)
	}

	var storedThread model.AgentThread
	var storedTurn model.AgentTurn
	var storedEvent model.AgentEvent
	if err := database.First(&storedThread, thread.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.First(&storedTurn, turn.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.First(&storedEvent, "public_id = ?", earlyCompleted.PublicID).Error; err != nil {
		t.Fatal(err)
	}
	if storedThread.SourceThreadRef == nil || *storedThread.SourceThreadRef != "source-thread-1" || storedThread.LastEventSeq != 1 ||
		storedTurn.SourceTurnRef == nil || *storedTurn.SourceTurnRef != "source-turn-1" || storedTurn.Status != "completed" ||
		storedEvent.ThreadID == nil || *storedEvent.ThreadID != storedThread.ID || storedEvent.TurnID == nil || *storedEvent.TurnID != storedTurn.ID {
		t.Fatalf("projected state mismatch: thread=%#v turn=%#v event=%#v", storedThread, storedTurn, storedEvent)
	}
	if err := database.Model(&storedTurn).Update("status", "running").Error; err != nil {
		t.Fatal(err)
	}

	interactionEvent := &domainagent.Event{
		PublicID: "agev_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Kind: "interaction.requested",
		SourceThreadRef: "source-thread-1", SourceTurnRef: "source-turn-1", SourceRequestRef: "request-1",
		PayloadJSON: `{"method":"item/tool/requestUserInput","request":{"questions":[{"questionRef":"question_known","required":true}]}}`, OccurredAt: now.Add(4 * time.Second),
	}
	if ack, err := repo.ApplyEventFrame(context.Background(), device.ID, profile.ID, 4, strings.Repeat("5", 64), interactionEvent, now.Add(4*time.Second)); err != nil || ack.Acknowledged != 4 {
		t.Fatalf("apply interaction event: ack=%v err=%v", ack, err)
	}
	var interaction model.AgentInteraction
	if err := database.First(&interaction, "source_request_ref = ?", "request-1").Error; err != nil {
		t.Fatal(err)
	}
	if interaction.Kind != "user_input" || !jsonEqual(interaction.RequestJSON, `{"questions":[{"questionRef":"question_known","required":true}]}`) {
		t.Fatalf("interaction projection mismatch: %#v", interaction)
	}
	wrongResponse := json.RawMessage(`{"kind":"user-input","answers":{"question_unknown":"yes"}}`)
	if _, err := repo.RespondInteraction(context.Background(), "10234567-89ab-4def-8123-456789abcdef", strings.Repeat("a", 64), 7, interaction.PublicID, wrongResponse, &domainagent.Command{PublicID: "agcmd_eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", Kind: "interaction.respond"}, now.Add(5*time.Second)); !errors.Is(err, repository.ErrInvalidInput) {
		t.Fatalf("interaction response validation error = %v, want ErrInvalidInput", err)
	}
	var invalidCommandCount int64
	if err := database.Model(&model.AgentCommand{}).Where("public_id = ?", "agcmd_eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee").Count(&invalidCommandCount).Error; err != nil || invalidCommandCount != 0 {
		t.Fatalf("invalid interaction response queued %d commands: %v", invalidCommandCount, err)
	}
	response := json.RawMessage(`{"kind":"user-input","answers":{"question_known":"yes"}}`)
	respondCommand := &domainagent.Command{PublicID: "agcmd_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Kind: "interaction.respond"}
	responded, err := repo.RespondInteraction(context.Background(), "11234567-89ab-4def-8123-456789abcdef", strings.Repeat("6", 64), 7, interaction.PublicID, response, respondCommand, now.Add(5*time.Second))
	if err != nil || responded.Status != "responding" {
		t.Fatalf("respond interaction: %#v %v", responded, err)
	}
	interactionTerminal := `{"kind":"result","result":{"kind":"interaction-responded"}}`
	if ack, err := repo.ApplyTerminalFrame(context.Background(), device.ID, 5, 3, respondCommand.PublicID, strings.Repeat("7", 64), interactionTerminal, now.Add(6*time.Second)); err != nil || ack != 5 {
		t.Fatalf("apply interaction terminal: ack=%d err=%v", ack, err)
	}
	if err := database.First(&interaction, interaction.ID).Error; err != nil || interaction.Status != "resolved" {
		t.Fatalf("interaction final state: %q %v", interaction.Status, err)
	}
	lateInteraction := model.AgentInteraction{
		PublicID: "agint_cccccccccccccccccccccccccccccccc", UserID: 7, ThreadID: storedThread.ID,
		TurnID: &storedTurn.ID, RuntimeProfileID: profile.ID, SourceRequestRef: "request-late",
		Kind: "command_approval", RequestJSON: `{"command":"pwd"}`, Status: "pending",
	}
	if err := database.Create(&lateInteraction).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Model(&storedTurn).Update("status", "completed").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := repo.RespondInteraction(
		context.Background(), "12234567-89ab-4def-8123-456789abcdef", strings.Repeat("b", 64), 7,
		lateInteraction.PublicID, response,
		&domainagent.Command{PublicID: "agcmd_ffffffffffffffffffffffffffffffff", Kind: "interaction.respond"}, now.Add(6*time.Second),
	); err == nil {
		t.Fatal("terminal turn accepted a new interaction response")
	}

	resourceCommand := &domainagent.Command{PublicID: "agcmd_cccccccccccccccccccccccccccccccc", Kind: "resource.refresh"}
	queued, err := repo.QueueResourceRefresh(
		context.Background(), "21234567-89ab-4def-8123-456789abcdef", strings.Repeat("8", 64), 7,
		device.PublicID, "", workspace.PublicID, "sessions", resourceCommand, now.Add(7*time.Second),
	)
	if err != nil || queued.ServerSeq != 4 {
		t.Fatalf("queue resource refresh: %#v %v", queued, err)
	}
	replayedResource, err := repo.QueueResourceRefresh(
		context.Background(), "21234567-89ab-4def-8123-456789abcdef", strings.Repeat("8", 64), 7,
		device.PublicID, "", workspace.PublicID, "sessions",
		&domainagent.Command{PublicID: "agcmd_dddddddddddddddddddddddddddddddd", Kind: "resource.refresh"}, now.Add(7*time.Second),
	)
	if err != nil || replayedResource.PublicID != queued.PublicID {
		t.Fatalf("idempotent resource replay changed result: %#v %v", replayedResource, err)
	}
	coalescedResource, err := repo.QueueResourceRefresh(
		context.Background(), "31234567-89ab-4def-8123-456789abcdef", strings.Repeat("a", 64), 7,
		device.PublicID, "", workspace.PublicID, "sessions",
		&domainagent.Command{PublicID: "agcmd_abbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Kind: "resource.refresh"}, now.Add(7*time.Second),
	)
	if err != nil || coalescedResource.PublicID != queued.PublicID {
		t.Fatalf("unfinished resource refresh was duplicated: %#v %v", coalescedResource, err)
	}
	resourceTerminal := `{"kind":"result","result":{"kind":"resource","resource":"sessions","data":{"data":[{"sourceThreadRef":"source-thread-1","name":"Local session","status":"active","historyLoaded":false},{"sourceThreadRef":"source-thread-2","name":"Imported session","modelProvider":"openai","status":"archived","createdAt":1786615200,"updatedAt":1786615260,"recencyAt":1786615360,"historyLoaded":false}]}}}`
	if ack, err := repo.ApplyTerminalFrame(context.Background(), device.ID, 6, 4, resourceCommand.PublicID, strings.Repeat("9", 64), resourceTerminal, now.Add(8*time.Second)); err != nil || ack != 6 {
		t.Fatalf("apply resource terminal: ack=%d err=%v", ack, err)
	}
	recentResource, err := repo.QueueResourceRefresh(
		context.Background(), "51234567-89ab-4def-8123-456789abcdef", strings.Repeat("c", 64), 7,
		device.PublicID, "", workspace.PublicID, "sessions",
		&domainagent.Command{PublicID: "agcmd_recent_refresh_0123456789abcdef", Kind: "resource.refresh"}, now.Add(9*time.Second),
	)
	if err != nil || recentResource.PublicID != queued.PublicID || recentResource.State != "completed" {
		t.Fatalf("recent successful resource refresh was not reused: %#v %v", recentResource, err)
	}
	snapshot, err := repo.GetResourceSnapshot(context.Background(), 7, device.PublicID, "", workspace.PublicID, "sessions")
	if err != nil || snapshot.WorkspacePublicID != workspace.PublicID || snapshot.ProfilePublicID != profile.PublicID ||
		!jsonEqual(snapshot.DataJSON, `{"data":[{"sourceThreadRef":"source-thread-1","name":"Local session","status":"active","historyLoaded":false},{"sourceThreadRef":"source-thread-2","name":"Imported session","modelProvider":"openai","status":"archived","createdAt":1786615200,"updatedAt":1786615260,"recencyAt":1786615360,"historyLoaded":false}]}`) {
		t.Fatalf("resource snapshot mismatch: %#v %v", snapshot, err)
	}
	var importedConversation model.Conversation
	if err := database.Where("execution_type = ? AND title = ?", "gateway", "Imported session").First(&importedConversation).Error; err != nil ||
		importedConversation.ExecutionDeviceID != device.PublicID || importedConversation.ExecutionProfileID != profile.PublicID ||
		importedConversation.ExecutionWorkspaceID != workspace.PublicID || importedConversation.MessageCount != 0 || importedConversation.Status != "archived" {
		t.Fatalf("imported conversation mismatch: %#v %v", importedConversation, err)
	}
	var importedThread model.AgentThread
	if err := database.Where("conversation_id = ?", importedConversation.ID).First(&importedThread).Error; err != nil ||
		importedThread.SourceThreadRef == nil || *importedThread.SourceThreadRef != "source-thread-2" || importedThread.Status != "archived" || importedThread.HistoryStatus != "unloaded" {
		t.Fatalf("imported thread mismatch: %#v %v", importedThread, err)
	}
	var importedMessages []model.Message
	if err := database.Where("conversation_id = ?", importedConversation.ID).Order("id ASC").Find(&importedMessages).Error; err != nil || len(importedMessages) != 0 {
		t.Fatalf("session summary eagerly imported messages: %#v %v", importedMessages, err)
	}
	historyThread, historyCommand, err := repo.QueueThreadHistory(context.Background(), 7, importedConversation.ID, &domainagent.Command{PublicID: "agcmd_ffffffffffffffffffffffffffffffff", Kind: "thread.read"}, now.Add(9*time.Second))
	if err != nil || historyCommand == nil || historyThread.HistoryStatus != "loading" || historyCommand.ServerSeq != 5 {
		t.Fatalf("queue thread history: %#v %#v %v", historyThread, historyCommand, err)
	}
	var storedHistoryCommand model.AgentCommand
	if err := database.Where("public_id = ?", historyCommand.PublicID).First(&storedHistoryCommand).Error; err != nil {
		t.Fatal(err)
	}
	historyFile := model.FileObject{
		FileID: "file_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", UserID: 7, Purpose: "conversation_history",
		FileName: "screenshot.png", MimeType: "image/png", DetectedMIME: "image/png", FileCategory: "image",
		SizeBytes: 16, SHA256: strings.Repeat("a", 64), StoragePath: "uploads/history.png", Status: "active",
		ProcessingStatus: "uploaded", ProcessingReady: true,
	}
	if err := database.Create(&historyFile).Error; err != nil {
		t.Fatal(err)
	}
	sourceTurnRef := "source-turn-1"
	if err := database.Create(&model.AgentTurn{
		PublicID: "agturn_history_0123456789abcdef", UserID: 7, ThreadID: importedThread.ID,
		RunID: "run_existing_history", SourceTurnRef: &sourceTurnRef, Status: "completed", InputJSON: "[]", SettingsJSON: "{}",
	}).Error; err != nil {
		t.Fatal(err)
	}
	historyTerminal := `{"kind":"result","result":{"kind":"thread-read","session":{"sourceThreadRef":"source-thread-2","name":"Imported session","model":"gpt-test","reasoningEffort":"high","historyLoaded":true,"historyProjectionVersion":11,"createdAt":1786615200,"updatedAt":1786615260,"recencyAt":1786615360,"messages":[{"role":"user","status":"success","content":"inspect the repository","sourceTurnRef":"source-turn-1","sourceMessageRef":"message-user-1","createdAt":1786615200,"attachments":[{"fileID":"file_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}]},{"role":"assistant","status":"success","content":"ready","reasoningContent":"checked files","sourceTurnRef":"source-turn-1","sourceMessageRef":"message-assistant-1","createdAt":1786615260,"executionEvents":[{"kind":"turn/started","sourceEventRef":"turn-started","payload":{"turn":{"status":"running"}}},{"kind":"item/completed","sourceEventRef":"item-source-item-1","payload":{"itemID":"source-item-1","item":{"itemID":"source-item-1","kind":"reasoning","summary":["checked files"],"status":"completed"}}},{"kind":"turn/completed","sourceEventRef":"turn-completed","payload":{"turn":{"status":"completed"}}}] }]}}}`
	if err := projectTerminalResult(database, &device, &model.AgentBridgeFrame{}, &storedHistoryCommand, historyTerminal, now.Add(10*time.Second)); err != nil {
		t.Fatalf("project thread history: %v", err)
	}
	if err := database.Model(&storedHistoryCommand).Updates(map[string]any{
		"state": "completed", "completed_at": now.Add(10 * time.Second),
	}).Error; err != nil {
		t.Fatalf("complete thread history command: %v", err)
	}
	if err := database.Where("conversation_id = ?", importedConversation.ID).Order("id ASC").Find(&importedMessages).Error; err != nil ||
		len(importedMessages) != 2 || importedMessages[0].Role != "user" || importedMessages[1].ReasoningContent != "checked files" ||
		importedMessages[0].RunID != "run_existing_history" || importedMessages[0].RunID != importedMessages[1].RunID ||
		importedMessages[1].ParentMessageID == nil || *importedMessages[1].ParentMessageID != importedMessages[0].ID {
		t.Fatalf("imported messages mismatch: %#v %v", importedMessages, err)
	}
	var importedExecutionEvents []model.ConversationExecutionEvent
	if err := database.Where("conversation_id = ?", importedConversation.ID).Order("seq ASC").Find(&importedExecutionEvents).Error; err != nil ||
		len(importedExecutionEvents) != 3 || importedExecutionEvents[1].Kind != "item/completed" || importedExecutionEvents[1].RunID != importedMessages[1].RunID {
		t.Fatalf("imported execution events mismatch: %#v %v", importedExecutionEvents, err)
	}
	var historyAttachment model.Attachment
	if err := database.Where("message_id = ?", importedMessages[0].ID).First(&historyAttachment).Error; err != nil ||
		historyAttachment.FileID != historyFile.FileID || historyAttachment.Kind != "image" || historyAttachment.StoragePath != historyFile.StoragePath {
		t.Fatalf("imported history attachment mismatch: %#v %v", historyAttachment, err)
	}
	if err := database.First(&importedConversation, importedConversation.ID).Error; err != nil ||
		importedConversation.Model != "gpt-test" || importedConversation.ReasoningEffort != "high" ||
		importedConversation.UpdatedAt.Unix() != 1786615360 {
		t.Fatalf("imported settings or recency mismatch: %#v %v", importedConversation, err)
	}
	blankSettingsHistory := `{"sourceThreadRef":"source-thread-2","name":"Imported session","historyLoaded":true,"historyProjectionVersion":11,"createdAt":1786615200,"updatedAt":1786615260,"recencyAt":1786615360,"messages":[{"role":"user","status":"success","content":"inspect the repository","sourceTurnRef":"source-turn-1","sourceMessageRef":"message-user-1","createdAt":1786615200},{"role":"assistant","status":"success","content":"ready","reasoningContent":"checked files","sourceTurnRef":"source-turn-1","sourceMessageRef":"message-assistant-1","createdAt":1786615260}]}`
	if err := syncThreadHistory(database, &storedHistoryCommand, json.RawMessage(blankSettingsHistory), now.Add(11*time.Second)); err != nil {
		t.Fatalf("refresh thread history without settings: %v", err)
	}
	if err := database.First(&importedConversation, importedConversation.ID).Error; err != nil ||
		importedConversation.Model != "gpt-test" || importedConversation.ReasoningEffort != "high" {
		t.Fatalf("blank history settings replaced known values: %#v %v", importedConversation, err)
	}
	updatedResourceTerminal := `{"kind":"result","result":{"kind":"resource","resource":"sessions","data":{"data":[{"sourceThreadRef":"source-thread-2","name":"Renamed imported session","modelProvider":"openai","status":"active","createdAt":1786615200,"updatedAt":1786615420,"recencyAt":1786615360,"historyLoaded":false}]}}}`
	if err := projectTerminalResult(database, &device, &model.AgentBridgeFrame{}, &model.AgentCommand{
		UserID: 7, RuntimeProfileID: &profile.ID, WorkspaceID: &workspace.ID, Kind: "resource.refresh",
		PayloadJSON: `{"resource":{"name":"sessions"}}`,
	}, updatedResourceTerminal, now.Add(11*time.Second)); err != nil {
		t.Fatalf("update session projection: %v", err)
	}
	if err := database.Where("conversation_id = ?", importedConversation.ID).Order("id ASC").Find(&importedMessages).Error; err != nil || len(importedMessages) != 2 {
		t.Fatalf("updated imported messages mismatch: %#v %v", importedMessages, err)
	}
	if err := database.First(&importedConversation, importedConversation.ID).Error; err != nil || importedConversation.Title != "Renamed imported session" || importedConversation.MessageCount != 2 || importedConversation.Status != "active" || importedConversation.UpdatedAt.Unix() != 1786615360 {
		t.Fatalf("updated imported conversation mismatch: %#v %v", importedConversation, err)
	}
	if err := database.First(&importedThread, importedThread.ID).Error; err != nil || importedThread.Status != "active" || importedThread.HistoryStatus != "loaded" || importedThread.HistoryVersion != historyProjectionVersion {
		t.Fatalf("updated imported thread mismatch: %#v %v", importedThread, err)
	}
	snapshotEvent := &domainagent.Event{
		PublicID: "agev_dddddddddddddddddddddddddddddddd", Kind: "workspace/sessions/updated",
		PayloadJSON: `{"workspaceId":"workspace-main","revision":"0123456789abcdef01234567","data":[{"sourceThreadRef":"source-thread-2","preview":"","name":"Renamed imported session","modelProvider":"openai","status":"active","createdAt":1786615200,"updatedAt":1786615320,"recencyAt":1786615420,"historyLoaded":false},{"sourceThreadRef":"source-thread-3","preview":"new desktop work","name":"Desktop session","modelProvider":"openai","status":"active","createdAt":1786615400,"updatedAt":1786615400,"recencyAt":1786615400,"historyLoaded":false}]}`,
		OccurredAt:  now.Add(12 * time.Second),
	}
	appliedSnapshot, err := repo.ApplyEventFrame(context.Background(), device.ID, profile.ID, 7, strings.Repeat("f", 64), snapshotEvent, now.Add(12*time.Second))
	if err != nil || appliedSnapshot.Acknowledged != 7 || len(appliedSnapshot.ConversationPublicIDs) != 2 || !slices.Contains(appliedSnapshot.ConversationPublicIDs, importedConversation.PublicID) {
		t.Fatalf("apply session snapshot: %#v %v", appliedSnapshot, err)
	}
	var desktopConversation model.Conversation
	if err := database.Where("execution_type = ? AND title = ?", "gateway", "Desktop session").First(&desktopConversation).Error; err != nil ||
		!slices.Contains(appliedSnapshot.ConversationPublicIDs, desktopConversation.PublicID) {
		t.Fatalf("desktop session was not imported: %#v %v", desktopConversation, err)
	}
	var desktopThread model.AgentThread
	if err := database.Where("conversation_id = ?", desktopConversation.ID).First(&desktopThread).Error; err != nil || desktopThread.HistoryStatus != "unloaded" ||
		desktopThread.SourceThreadRef == nil || *desktopThread.SourceThreadRef != "source-thread-3" {
		t.Fatalf("desktop session thread mismatch: %#v %v", desktopThread, err)
	}
	if err := database.First(&importedThread, importedThread.ID).Error; err != nil || importedThread.HistoryStatus != "unloaded" || importedThread.HistoryError != "" {
		t.Fatalf("session snapshot did not invalidate imported history: %#v %v", importedThread, err)
	}
	olderSnapshot := &domainagent.Event{
		PublicID: "agev_eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", Kind: "workspace/sessions/updated",
		PayloadJSON: `{"workspaceId":"workspace-main","revision":"1123456789abcdef01234567","data":[{"sourceThreadRef":"source-thread-2","preview":"","name":"Renamed imported session","modelProvider":"openai","status":"active","createdAt":1786615200,"updatedAt":1786615320,"recencyAt":1786615360,"historyLoaded":false},{"sourceThreadRef":"source-thread-3","preview":"new desktop work","name":"Desktop session","modelProvider":"openai","status":"active","createdAt":1786615400,"updatedAt":1786615400,"recencyAt":1786615400,"historyLoaded":false}]}`,
		OccurredAt:  now.Add(13 * time.Second),
	}
	olderApplied, err := repo.ApplyEventFrame(context.Background(), device.ID, profile.ID, 8, strings.Repeat("e", 64), olderSnapshot, now.Add(13*time.Second))
	if err != nil || olderApplied.Acknowledged != 8 || len(olderApplied.ConversationPublicIDs) != 0 {
		t.Fatalf("older session snapshot changed conversations: %#v %v", olderApplied, err)
	}
	refreshedHistoryThread, refreshedHistoryCommand, err := repo.QueueThreadHistory(context.Background(), 7, importedConversation.ID, &domainagent.Command{PublicID: "agcmd_eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", Kind: "thread.read"}, now.Add(13*time.Second))
	if err != nil || refreshedHistoryCommand == nil || refreshedHistoryThread.HistoryStatus != "loading" || refreshedHistoryCommand.ServerSeq != 6 {
		t.Fatalf("queue refreshed thread history: %#v %#v %v", refreshedHistoryThread, refreshedHistoryCommand, err)
	}
	if err := database.Model(&importedThread).Update("history_status", "unloaded").Error; err != nil {
		t.Fatalf("invalidate in-flight thread history: %v", err)
	}
	coalescedHistoryThread, coalescedHistoryCommand, err := repo.QueueThreadHistory(context.Background(), 7, importedConversation.ID, &domainagent.Command{PublicID: "agcmd_99999999999999999999999999999999", Kind: "thread.read"}, now.Add(14*time.Second))
	if err != nil || coalescedHistoryCommand == nil || coalescedHistoryCommand.PublicID != refreshedHistoryCommand.PublicID || coalescedHistoryThread.HistoryStatus != "unloaded" {
		t.Fatalf("coalesce invalidated in-flight thread history: %#v %#v %v", coalescedHistoryThread, coalescedHistoryCommand, err)
	}
	canonicalWorkspace := model.AgentWorkspace{
		PublicID: "workspace-canonical", UserID: 7, DeviceID: device.ID, RuntimeProfileID: profile.ID,
		Name: "source-repository", Status: "available", LastSeenAt: now,
	}
	if err := database.Create(&canonicalWorkspace).Error; err != nil {
		t.Fatalf("create canonical workspace: %v", err)
	}
	shortResourceTerminal := `{"kind":"result","result":{"kind":"resource","resource":"sessions","data":{"data":[{"sourceThreadRef":"source-thread-2","name":"Renamed imported session","status":"active","historyLoaded":false}]}}}`
	if err := projectTerminalResult(database, &device, &model.AgentBridgeFrame{}, &model.AgentCommand{
		UserID: 7, RuntimeProfileID: &profile.ID, WorkspaceID: &canonicalWorkspace.ID, Kind: "resource.refresh",
		PayloadJSON: `{"resource":{"name":"sessions"}}`,
	}, shortResourceTerminal, now.Add(10*time.Second)); err != nil {
		t.Fatalf("rebind worktree session: %v", err)
	}
	if err := database.First(&importedThread, importedThread.ID).Error; err != nil || importedThread.WorkspaceID != canonicalWorkspace.ID {
		t.Fatalf("thread workspace was not rebound: %#v %v", importedThread, err)
	}
	if err := database.First(&importedConversation, importedConversation.ID).Error; err != nil || importedConversation.ExecutionWorkspaceID != canonicalWorkspace.PublicID {
		t.Fatalf("conversation workspace was not rebound: %#v %v", importedConversation, err)
	}

	itemStarted := &domainagent.Event{
		PublicID: "agev_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Kind: "item/started",
		SourceThreadRef: "source-thread-1", SourceTurnRef: "source-turn-1", SourceItemRef: "source-item-1",
		PayloadJSON: `{"item":{"type":"agentMessage","text":""},"startedAtMs":1}`, OccurredAt: now.Add(13 * time.Second),
	}
	if ack, err := repo.ApplyEventFrame(context.Background(), device.ID, profile.ID, 9, strings.Repeat("0", 64), itemStarted, now.Add(13*time.Second)); err != nil || ack.Acknowledged != 9 ||
		!slices.Equal(ack.ConversationPublicIDs, []string{conversation.PublicID}) {
		t.Fatalf("apply item start: ack=%v err=%v", ack, err)
	}
	itemCompleted := &domainagent.Event{
		PublicID: "agev_cccccccccccccccccccccccccccccccc", Kind: "item/completed",
		SourceThreadRef: "source-thread-1", SourceTurnRef: "source-turn-1", SourceItemRef: "source-item-1",
		PayloadJSON: `{"item":{"type":"agentMessage","text":"done"},"completedAtMs":2}`, OccurredAt: now.Add(14 * time.Second),
	}
	if ack, err := repo.ApplyEventFrame(context.Background(), device.ID, profile.ID, 10, strings.Repeat("1", 64), itemCompleted, now.Add(14*time.Second)); err != nil || ack.Acknowledged != 10 {
		t.Fatalf("apply item completion: ack=%v err=%v", ack, err)
	}
	var storedItem model.AgentItem
	if err := database.Where("thread_id = ? AND source_item_ref = ?", thread.ID, "source-item-1").First(&storedItem).Error; err != nil ||
		storedItem.Status != "completed" || storedItem.Kind != "agentMessage" || storedItem.LastEventSeq != 4 ||
		storedItem.TurnID == nil || *storedItem.TurnID != turn.ID || !jsonEqual(storedItem.DataJSON, itemCompleted.PayloadJSON) {
		t.Fatalf("item projection mismatch: %#v %v", storedItem, err)
	}
	pending, err := repo.ListPendingConversationEvents(context.Background(), device.ID, 100)
	if err != nil || len(pending) != 4 {
		t.Fatalf("conversation outbox mismatch: %#v %v", pending, err)
	}
	for _, event := range pending {
		if event.ConversationID != threadInput.ConversationID || event.RunID != turnInput.RunID {
			t.Fatalf("conversation binding was lost: %#v", event)
		}
	}
	if err := repo.MarkConversationEventProjected(context.Background(), storedEvent.ID, now.Add(15*time.Second)); err != nil {
		t.Fatal(err)
	}
	replayed, err := repo.ApplyEventFrame(
		context.Background(), device.ID, profile.ID, 1, strings.Repeat("2", 64), earlyCompleted, now.Add(16*time.Second),
	)
	if err != nil || replayed.Acknowledged != 10 || replayed.ConversationID != 0 || replayed.RunID != "" {
		t.Fatalf("projected bridge replay was republished: %#v %v", replayed, err)
	}
}

func TestResolveTurnInteractionsClosesActiveStates(t *testing.T) {
	database := testutil.Postgres(t)
	if err := database.AutoMigrate(&model.AgentInteraction{}); err != nil {
		t.Fatal(err)
	}
	turnID := uint(91)
	items := []model.AgentInteraction{
		{PublicID: "agint_11111111111111111111111111111111", UserID: 7, ThreadID: 1, TurnID: &turnID, RuntimeProfileID: 1, SourceRequestRef: "request-1", Kind: "command_approval", RequestJSON: `{}`, Status: "pending"},
		{PublicID: "agint_22222222222222222222222222222222", UserID: 7, ThreadID: 1, TurnID: &turnID, RuntimeProfileID: 1, SourceRequestRef: "request-2", Kind: "file_approval", RequestJSON: `{}`, Status: "responding"},
		{PublicID: "agint_33333333333333333333333333333333", UserID: 7, ThreadID: 1, TurnID: &turnID, RuntimeProfileID: 1, SourceRequestRef: "request-3", Kind: "user_input", RequestJSON: `{}`, Status: "failed"},
	}
	if err := database.Create(&items).Error; err != nil {
		t.Fatal(err)
	}
	if err := resolveTurnInteractions(database, turnID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	var stored []model.AgentInteraction
	if err := database.Order("id ASC").Find(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored[0].Status != "resolved" || stored[1].Status != "resolved" || stored[2].Status != "failed" {
		t.Fatalf("terminal interaction states = %#v", stored)
	}
}

func jsonEqual(left, right string) bool {
	var leftValue, rightValue any
	return json.Unmarshal([]byte(left), &leftValue) == nil &&
		json.Unmarshal([]byte(right), &rightValue) == nil &&
		reflect.DeepEqual(leftValue, rightValue)
}

func TestInvalidResourceTerminalAdvancesBridgeCursor(t *testing.T) {
	database := testutil.Postgres(t)
	if err := database.AutoMigrate(
		&model.AgentDevice{}, &model.AgentRuntimeProfile{}, &model.AgentWorkspace{},
		&model.AgentCommand{}, &model.AgentBridgeFrame{}, &model.AgentResourceSnapshot{},
	); err != nil {
		t.Fatalf("migrate resource terminal tables: %v", err)
	}
	now := time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC)
	device := model.AgentDevice{
		PublicID: "agd_0123456789abcdef0123456789abcdef", UserID: 7, Name: "desktop", Platform: "windows",
		PublicKey: bytes.Repeat([]byte("k"), 32), PublicKeyFingerprint: strings.Repeat("a", 64),
		CredentialVersion: 1, Status: domainagent.DeviceStatusActive, NextServerSeq: 2,
	}
	if err := database.Create(&device).Error; err != nil {
		t.Fatal(err)
	}
	profile := model.AgentRuntimeProfile{
		PublicID: "codex-default", UserID: 7, DeviceID: device.ID, Provider: "codex", Status: domainagent.RuntimeStatusReady,
	}
	if err := database.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}
	workspace := model.AgentWorkspace{
		PublicID: "workspace-main", UserID: 7, DeviceID: device.ID, RuntimeProfileID: profile.ID,
		Name: "DEEIX-Chat", Status: "available", LastSeenAt: now,
	}
	if err := database.Create(&workspace).Error; err != nil {
		t.Fatal(err)
	}
	command := model.AgentCommand{
		PublicID: "agcmd_0123456789abcdef0123456789abcdef", UserID: 7, DeviceID: device.ID,
		RuntimeProfileID: &profile.ID, WorkspaceID: &workspace.ID, ServerSeq: 1, Kind: "resource.refresh",
		PayloadJSON: `{"resource":{"scope":"workspace","name":"sessions"}}`, State: "acked", TerminalJSON: "{}",
	}
	if err := database.Create(&command).Error; err != nil {
		t.Fatal(err)
	}

	terminal := `{"kind":"result","result":{"kind":"resource","resource":"sessions","data":{"data":[{"sourceThreadRef":"source-thread-1","status":"invalid","historyLoaded":false}]}}}`
	repo := NewRepo(database)
	ack, err := repo.ApplyTerminalFrame(
		context.Background(), device.ID, 1, 1, command.PublicID, strings.Repeat("b", 64), terminal, now.Add(time.Second),
	)
	if err != nil || ack != 1 {
		t.Fatalf("invalid resource terminal blocked bridge: ack=%d err=%v", ack, err)
	}
	if err := database.First(&device, device.ID).Error; err != nil || device.LastAckedBridgeSeq != 1 {
		t.Fatalf("bridge cursor did not advance: %#v %v", device, err)
	}
	if err := database.First(&command, command.ID).Error; err != nil || command.State != "completed" || command.CompletedAt == nil {
		t.Fatalf("resource command did not complete: %#v %v", command, err)
	}
	var snapshots int64
	if err := database.Model(&model.AgentResourceSnapshot{}).Count(&snapshots).Error; err != nil || snapshots != 0 {
		t.Fatalf("invalid resource snapshot was persisted: count=%d err=%v", snapshots, err)
	}

	missingWorkspaceID := workspace.ID + 1000
	missingTarget := model.AgentCommand{
		PublicID: "agcmd_1123456789abcdef0123456789abcdef", UserID: 7, DeviceID: device.ID,
		RuntimeProfileID: &profile.ID, WorkspaceID: &missingWorkspaceID, ServerSeq: 2, Kind: "resource.refresh",
		PayloadJSON: `{"resource":{"scope":"workspace","name":"sessions"}}`, State: "acked", TerminalJSON: "{}",
	}
	if err := database.Create(&missingTarget).Error; err != nil {
		t.Fatal(err)
	}
	validTerminal := `{"kind":"result","result":{"kind":"resource","resource":"sessions","data":{"data":[]}}}`
	ack, err = repo.ApplyTerminalFrame(
		context.Background(), device.ID, 2, 2, missingTarget.PublicID, strings.Repeat("c", 64), validTerminal, now.Add(2*time.Second),
	)
	if err != nil || ack != 2 {
		t.Fatalf("missing resource target blocked bridge: ack=%d err=%v", ack, err)
	}
	if err := database.First(&missingTarget, missingTarget.ID).Error; err != nil || missingTarget.State != "completed" || missingTarget.CompletedAt == nil {
		t.Fatalf("missing-target resource command did not complete: %#v %v", missingTarget, err)
	}
}

func TestQueueAgentUpdateRequiresCapabilityAndCoalesces(t *testing.T) {
	database := testutil.Postgres(t)
	if err := database.AutoMigrate(
		&model.AgentDevice{}, &model.AgentRuntimeProfile{}, &model.AgentCommand{}, &model.AgentIdempotencyRecord{},
	); err != nil {
		t.Fatalf("migrate Agent update tables: %v", err)
	}
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	device := model.AgentDevice{
		PublicID: "agd_0123456789abcdef0123456789abcdef", UserID: 7, Name: "desktop", Platform: "windows",
		PublicKey: bytes.Repeat([]byte("k"), 32), PublicKeyFingerprint: strings.Repeat("a", 64),
		CredentialVersion: 1, Status: domainagent.DeviceStatusActive, NextServerSeq: 1,
	}
	if err := database.Create(&device).Error; err != nil {
		t.Fatal(err)
	}
	leaseExpiresAt, presenceExpiresAt := now.Add(-time.Hour), now.Add(-time.Minute)
	profile := model.AgentRuntimeProfile{
		PublicID: "codex-default", UserID: 7, DeviceID: device.ID, Provider: "codex", Status: domainagent.RuntimeStatusReady,
		LeaseExpiresAt: &leaseExpiresAt, PresenceExpiresAt: &presenceExpiresAt,
		ManifestJSON: `{"agentVersion":"0.4.56","commands":["agent.update"]}`,
	}
	if err := database.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}
	repo := NewRepo(database)
	queued, err := repo.QueueAgentUpdate(
		context.Background(), "41234567-89ab-4def-8123-456789abcdef", strings.Repeat("c", 64), 7,
		device.PublicID, "0.4.57", &domainagent.Command{PublicID: "agcmd_0123456789abcdef0123456789abcdef", Kind: "agent.update"}, now,
	)
	if err != nil || queued.ServerSeq != 1 || queued.Kind != "agent.update" || !strings.Contains(queued.PayloadJSON, `"targetVersion":"0.4.57"`) {
		t.Fatalf("queue Agent update: %#v %v", queued, err)
	}
	coalesced, err := repo.QueueAgentUpdate(
		context.Background(), "51234567-89ab-4def-8123-456789abcdef", strings.Repeat("d", 64), 7,
		device.PublicID, "0.4.57", &domainagent.Command{PublicID: "agcmd_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Kind: "agent.update"}, now,
	)
	if err != nil || coalesced.PublicID != queued.PublicID {
		t.Fatalf("unfinished Agent update was duplicated: %#v %v", coalesced, err)
	}
	if err := database.Model(&profile).Update("manifest_json", `{"agentVersion":"0.4.56","commands":["thread.create"]}`).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Model(&model.AgentCommand{}).Where("public_id = ?", queued.PublicID).Update("completed_at", now).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := repo.QueueAgentUpdate(
		context.Background(), "61234567-89ab-4def-8123-456789abcdef", strings.Repeat("e", 64), 7,
		device.PublicID, "0.4.57", &domainagent.Command{PublicID: "agcmd_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Kind: "agent.update"}, now,
	); err == nil {
		t.Fatal("Agent update was queued without the declared capability")
	}
}

func TestQueueWorkspaceMutationSupportsDiscoveredWorkspaceAndRequiresCapability(t *testing.T) {
	database := testutil.Postgres(t)
	if err := database.AutoMigrate(
		&model.AgentDevice{}, &model.AgentRuntimeProfile{}, &model.AgentWorkspace{}, &model.AgentCommand{}, &model.AgentIdempotencyRecord{},
	); err != nil {
		t.Fatalf("migrate Workspace mutation tables: %v", err)
	}
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	device := model.AgentDevice{
		PublicID: "agd_0123456789abcdef0123456789abcdef", UserID: 7, Name: "desktop", Platform: "windows",
		PublicKey: bytes.Repeat([]byte("k"), 32), PublicKeyFingerprint: strings.Repeat("a", 64),
		CredentialVersion: 1, Status: domainagent.DeviceStatusActive, NextServerSeq: 1,
	}
	if err := database.Create(&device).Error; err != nil {
		t.Fatal(err)
	}
	leaseExpiresAt := now.Add(time.Hour)
	profile := model.AgentRuntimeProfile{
		PublicID: "codex-default", UserID: 7, DeviceID: device.ID, Provider: "codex", Status: domainagent.RuntimeStatusReady,
		LeaseExpiresAt: &leaseExpiresAt, ManifestJSON: `{"commands":["workspace.rename","workspace.unregister"]}`,
	}
	if err := database.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}
	workspace := model.AgentWorkspace{
		PublicID: "workspace-0123456789abcdef01234567", UserID: 7, DeviceID: device.ID,
		RuntimeProfileID: profile.ID, Name: "project", Status: "available", LastSeenAt: now,
	}
	if err := database.Create(&workspace).Error; err != nil {
		t.Fatal(err)
	}
	repo := NewRepo(database)
	firstQueued, err := repo.QueueWorkspaceMutation(
		context.Background(), "71234567-89ab-4def-8123-456789abcdef", strings.Repeat("7", 64), 7,
		device.PublicID, workspace.PublicID, "workspace.rename", "renamed",
		&domainagent.Command{PublicID: "agcmd_7123456789abcdef0123456789abcdef", Kind: "workspace.rename"}, now,
	)
	if err != nil || firstQueued.ServerSeq != 1 {
		t.Fatalf("queue discovered Workspace rename: %#v %v", firstQueued, err)
	}
	queued, err := repo.QueueWorkspaceMutation(
		context.Background(), "81234567-89ab-4def-8123-456789abcdef", strings.Repeat("8", 64), 7,
		device.PublicID, workspace.PublicID, "workspace.rename", "renamed",
		&domainagent.Command{PublicID: "agcmd_8123456789abcdef0123456789abcdef", Kind: "workspace.rename"}, now,
	)
	if err != nil || queued.ServerSeq != 2 || !strings.Contains(queued.PayloadJSON, `"name":"renamed"`) {
		t.Fatalf("queue Workspace rename: %#v %v", queued, err)
	}
	if err = database.Model(&profile).Update("manifest_json", `{"commands":["workspace.rename"]}`).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = repo.QueueWorkspaceMutation(
		context.Background(), "91234567-89ab-4def-8123-456789abcdef", strings.Repeat("9", 64), 7,
		device.PublicID, workspace.PublicID, "workspace.unregister", "",
		&domainagent.Command{PublicID: "agcmd_9123456789abcdef0123456789abcdef", Kind: "workspace.unregister"}, now,
	); err == nil {
		t.Fatal("Workspace removal was queued without the declared capability")
	}
}

func TestIsThreadOnlyEvent(t *testing.T) {
	threadID := uint(1)
	cases := []struct {
		name  string
		event model.AgentEvent
		want  bool
	}{
		{name: "unbound", event: model.AgentEvent{}, want: false},
		{name: "thread event", event: model.AgentEvent{ThreadID: &threadID}, want: true},
		{name: "turn pending binding", event: model.AgentEvent{ThreadID: &threadID, SourceTurnRef: "source-turn"}, want: false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := isThreadOnlyEvent(&test.event); got != test.want {
				t.Fatalf("isThreadOnlyEvent() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestManifestSupportsWorkspaceRegistration(t *testing.T) {
	if !manifestSupportsCommand(`{"commands":["thread.create","workspace.register"]}`, "workspace.register") {
		t.Fatal("workspace registration capability was not detected")
	}
	if manifestSupportsCommand(`{"commands":["thread.create"]}`, "workspace.register") {
		t.Fatal("missing workspace registration capability was accepted")
	}
}

func TestAgentTurnTerminalPayloads(t *testing.T) {
	cases := []struct {
		name, payload, status, code, message string
	}{
		{name: "completed", payload: `{"turn":{"status":"completed","error":null}}`, status: "completed"},
		{name: "interrupted", payload: `{"turn":{"status":"interrupted","error":null}}`, status: "interrupted"},
		{name: "failed", payload: `{"turn":{"status":"failed","error":{"message":"quota exhausted","codexErrorInfo":"usageLimitExceeded"}}}`, status: "failed", code: "usageLimitExceeded", message: "quota exhausted"},
		{name: "structured failure", payload: `{"turn":{"status":"failed","error":{"message":"upstream disconnected","codexErrorInfo":{"responseStreamDisconnected":{"httpStatusCode":503}}}}}`, status: "failed", code: "responseStreamDisconnected", message: "upstream disconnected"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			status, code, message, err := agentTurnTerminal(test.payload)
			if err != nil || status != test.status || code != test.code || message != test.message {
				t.Fatalf("agentTurnTerminal() = %q, %q, %q, %v", status, code, message, err)
			}
		})
	}
	if _, _, _, err := agentTurnTerminal(`{"turn":{"status":"inProgress"}}`); err == nil {
		t.Fatal("non-terminal turn status accepted")
	}
}

func TestInteractionRequestKinds(t *testing.T) {
	cases := map[string]string{
		"item/commandExecution/requestApproval": "command_approval",
		"item/fileChange/requestApproval":       "file_approval",
		"item/tool/requestUserInput":            "user_input",
		"item/permissions/requestApproval":      "permission",
		"mcpServer/elicitation/request":         "mcp_elicitation",
		"item/tool/call":                        "dynamic_tool",
	}
	for method, expectedKind := range cases {
		kind, request, err := projectInteractionRequest(`{"method":"` + method + `","request":{"title":"fixture"}}`)
		if err != nil || kind != expectedKind || !jsonEqual(request, `{"title":"fixture"}`) {
			t.Fatalf("projectInteractionRequest(%q) = %q, %s, %v", method, kind, request, err)
		}
	}
	if _, _, err := projectInteractionRequest(`{"method":"unknown","request":{}}`); err == nil {
		t.Fatal("unknown interaction method accepted")
	}
}

func TestInteractionResponseMatchesRequest(t *testing.T) {
	valid := []struct {
		name, kind, request, response string
	}{
		{name: "command approval", kind: "command_approval", request: `{}`, response: `{"kind":"approval","decision":"accept"}`},
		{name: "file approval", kind: "file_approval", request: `{}`, response: `{"kind":"approval","decision":"decline"}`},
		{name: "user input", kind: "user_input", request: `{"questions":[{"questionRef":"required","required":true},{"questionRef":"optional"}]}`, response: `{"kind":"user-input","answers":{"required":"yes"}}`},
		{name: "user input option", kind: "user_input", request: `{"questions":[{"questionRef":"choice","required":true,"options":[{"label":"yes"},{"label":"no"}]}]}`, response: `{"kind":"user-input","answers":{"choice":"yes"}}`},
		{name: "user input freeform", kind: "user_input", request: `{"questions":[{"questionRef":"choice","required":true,"allowFreeform":true,"options":[{"label":"yes"}]}]}`, response: `{"kind":"user-input","answers":{"choice":"custom"}}`},
		{name: "permission", kind: "permission", request: `{"allowedScopes":["session"]}`, response: `{"kind":"permission","decision":"accept","scope":"session"}`},
		{name: "permission decline", kind: "permission", request: `{"allowedScopes":["session"]}`, response: `{"kind":"permission","decision":"decline"}`},
		{name: "mcp elicitation", kind: "mcp_elicitation", request: `{"requestedSchema":{"properties":{"name":{"type":"string","enum":["Ada","Grace"]},"count":{"type":"integer","enum":[2,3]},"ratio":{"type":"number"},"enabled":{"type":"boolean"}},"required":["name","count","enabled"]}}`, response: `{"kind":"mcp-elicitation","decision":"accept","content":{"name":"Ada","count":3,"ratio":0.5,"enabled":true}}`},
		{name: "dynamic tool", kind: "dynamic_tool", request: `{"acceptedContentKinds":["text"]}`, response: `{"kind":"dynamic-tool","success":true,"content":[{"kind":"text","text":"done"}]}`},
	}
	for _, test := range valid {
		t.Run(test.name, func(t *testing.T) {
			if !interactionResponseMatchesRequest(test.kind, test.request, json.RawMessage(test.response)) {
				t.Fatal("valid interaction response was rejected")
			}
		})
	}

	invalid := []struct {
		name, kind, request, response string
	}{
		{name: "wrong response kind", kind: "user_input", request: `{"questions":[{"questionRef":"question"}]}`, response: `{"kind":"approval","decision":"accept"}`},
		{name: "unknown question ref", kind: "user_input", request: `{"questions":[{"questionRef":"known"}]}`, response: `{"kind":"user-input","answers":{"unknown":"yes"}}`},
		{name: "missing required answer", kind: "user_input", request: `{"questions":[{"questionRef":"required","required":true}]}`, response: `{"kind":"user-input","answers":{}}`},
		{name: "blank required answer", kind: "user_input", request: `{"questions":[{"questionRef":"required","required":true}]}`, response: `{"kind":"user-input","answers":{"required":"  "}}`},
		{name: "answer outside options", kind: "user_input", request: `{"questions":[{"questionRef":"choice","required":true,"options":[{"label":"yes"},{"label":"no"}]}]}`, response: `{"kind":"user-input","answers":{"choice":"custom"}}`},
		{name: "disallowed permission scope", kind: "permission", request: `{"allowedScopes":["turn"]}`, response: `{"kind":"permission","decision":"accept","scope":"session"}`},
		{name: "disallowed default permission scope", kind: "permission", request: `{"allowedScopes":["session"]}`, response: `{"kind":"permission","decision":"accept"}`},
		{name: "unknown mcp field", kind: "mcp_elicitation", request: `{"requestedSchema":{"properties":{"name":{"type":"string"}}}}`, response: `{"kind":"mcp-elicitation","decision":"accept","content":{"other":"Ada"}}`},
		{name: "missing required mcp field", kind: "mcp_elicitation", request: `{"requestedSchema":{"properties":{"name":{"type":"string"}},"required":["name"]}}`, response: `{"kind":"mcp-elicitation","decision":"accept","content":{}}`},
		{name: "wrong mcp type", kind: "mcp_elicitation", request: `{"requestedSchema":{"properties":{"count":{"type":"integer"}}}}`, response: `{"kind":"mcp-elicitation","decision":"accept","content":{"count":"3"}}`},
		{name: "fractional mcp integer", kind: "mcp_elicitation", request: `{"requestedSchema":{"properties":{"count":{"type":"integer"}}}}`, response: `{"kind":"mcp-elicitation","decision":"accept","content":{"count":2.5}}`},
		{name: "mcp enum mismatch", kind: "mcp_elicitation", request: `{"requestedSchema":{"properties":{"count":{"type":"integer","enum":[2,3]}}}}`, response: `{"kind":"mcp-elicitation","decision":"accept","content":{"count":4}}`},
		{name: "mcp decline with content", kind: "mcp_elicitation", request: `{}`, response: `{"kind":"mcp-elicitation","decision":"decline","content":{"name":"Ada"}}`},
		{name: "disallowed dynamic content", kind: "dynamic_tool", request: `{"acceptedContentKinds":["text"]}`, response: `{"kind":"dynamic-tool","success":true,"content":[{"kind":"image","url":"https://example.com/image.png"}]}`},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			if interactionResponseMatchesRequest(test.kind, test.request, json.RawMessage(test.response)) {
				t.Fatal("invalid interaction response was accepted")
			}
		})
	}
}
