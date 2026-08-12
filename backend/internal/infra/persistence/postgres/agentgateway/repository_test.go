package agentgateway

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	domainagent "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/agentgateway"
	model "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/models"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/testutil"
)

func TestThreadProjectionIsOrderedAndIdempotent(t *testing.T) {
	database := testutil.Postgres(t)
	if err := database.AutoMigrate(
		&model.AgentDevice{}, &model.AgentCommand{}, &model.AgentBridgeFrame{},
		&model.AgentRuntimeProfile{}, &model.AgentWorkspace{}, &model.AgentResourceSnapshot{}, &model.AgentThread{},
		&model.AgentTurn{}, &model.AgentEvent{}, &model.AgentInteraction{},
		&model.AgentIdempotencyRecord{},
	); err != nil {
		t.Fatalf("migrate agent gateway tables: %v", err)
	}

	now := time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC)
	device := model.AgentDevice{
		PublicID: "agd_0123456789abcdef0123456789abcdef", UserID: 7,
		EnrollmentCredentialID: 1, Name: "desktop", Platform: "windows",
		PublicKey: []byte(strings.Repeat("k", 32)), PublicKeyFingerprint: strings.Repeat("a", 64),
		CredentialVersion: 1, Status: domainagent.DeviceStatusActive, NextServerSeq: 1,
	}
	if err := database.Create(&device).Error; err != nil {
		t.Fatalf("create device: %v", err)
	}
	leaseExpiresAt := now.Add(10 * time.Minute)
	profile := model.AgentRuntimeProfile{
		PublicID: "codex-default", UserID: 7, DeviceID: device.ID, Provider: "codex",
		Status: domainagent.RuntimeStatusReady, LeaseExpiresAt: &leaseExpiresAt,
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

	repo := NewRepo(database)
	threadInput := &domainagent.Thread{PublicID: "agth_0123456789abcdef0123456789abcdef", UserID: 7, Title: "Agent work", Status: "queued"}
	turnInput := &domainagent.Turn{
		PublicID: "agturn_0123456789abcdef0123456789abcdef", UserID: 7,
		Status: "awaiting_thread", InputJSON: `[{"kind":"text","text":"run tests"}]`, SettingsJSON: `{}`,
	}
	createCommand := &domainagent.Command{
		PublicID: "agcmd_0123456789abcdef0123456789abcdef", Kind: "thread.create",
		PayloadJSON: `{"kind":"thread.create","deviceId":"agd_0123456789abcdef0123456789abcdef","profileId":"codex-default","workspaceId":"workspace-main","settings":{}}`,
	}
	idempotencyKey, requestHash := "01234567-89ab-4def-8123-456789abcdef", strings.Repeat("1", 64)
	thread, turn, err := repo.StartThread(context.Background(), idempotencyKey, requestHash, threadInput, turnInput, createCommand, now)
	if err != nil || thread == nil || turn == nil {
		t.Fatalf("start thread: thread=%#v turn=%#v err=%v", thread, turn, err)
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
		PayloadJSON: `{}`, OccurredAt: now.Add(time.Second),
	}
	if ack, err := repo.ApplyEventFrame(context.Background(), device.ID, profile.ID, 1, strings.Repeat("2", 64), earlyCompleted, now.Add(time.Second)); err != nil || ack != 1 {
		t.Fatalf("apply early event: ack=%d err=%v", ack, err)
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

	interactionEvent := &domainagent.Event{
		PublicID: "agev_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Kind: "interaction.requested",
		SourceThreadRef: "source-thread-1", SourceTurnRef: "source-turn-1", SourceRequestRef: "request-1",
		PayloadJSON: `{"method":"item/commandExecution/requestApproval"}`, OccurredAt: now.Add(4 * time.Second),
	}
	if ack, err := repo.ApplyEventFrame(context.Background(), device.ID, profile.ID, 4, strings.Repeat("5", 64), interactionEvent, now.Add(4*time.Second)); err != nil || ack != 4 {
		t.Fatalf("apply interaction event: ack=%d err=%v", ack, err)
	}
	var interaction model.AgentInteraction
	if err := database.First(&interaction, "source_request_ref = ?", "request-1").Error; err != nil {
		t.Fatal(err)
	}
	response := json.RawMessage(`{"kind":"approval","decision":"accept"}`)
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
	resourceTerminal := `{"kind":"result","result":{"kind":"resource","resource":"sessions","data":{"data":[{"sourceThreadRef":"source-thread-1","name":"Local session"}]}}}`
	if ack, err := repo.ApplyTerminalFrame(context.Background(), device.ID, 6, 4, resourceCommand.PublicID, strings.Repeat("9", 64), resourceTerminal, now.Add(8*time.Second)); err != nil || ack != 6 {
		t.Fatalf("apply resource terminal: ack=%d err=%v", ack, err)
	}
	snapshot, err := repo.GetResourceSnapshot(context.Background(), 7, device.PublicID, "", workspace.PublicID, "sessions")
	if err != nil || snapshot.WorkspacePublicID != workspace.PublicID || snapshot.ProfilePublicID != profile.PublicID || snapshot.DataJSON != `{"data":[{"sourceThreadRef":"source-thread-1","name":"Local session"}]}` {
		t.Fatalf("resource snapshot mismatch: %#v %v", snapshot, err)
	}

	renameCommand := &domainagent.Command{PublicID: "agcmd_eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", Kind: "thread.rename"}
	queuedRename, err := repo.QueueThreadCommand(
		context.Background(), "31234567-89ab-4def-8123-456789abcdef", strings.Repeat("a", 64), 7,
		thread.PublicID, "", json.RawMessage(`{"name":"Renamed thread"}`), renameCommand, now.Add(9*time.Second),
	)
	if err != nil || queuedRename.ServerSeq != 5 {
		t.Fatalf("queue thread rename: %#v %v", queuedRename, err)
	}
	if err := database.First(&storedThread, thread.ID).Error; err != nil || storedThread.Title == "Renamed thread" {
		t.Fatalf("rename applied before provider result: %q %v", storedThread.Title, err)
	}
	renameTerminal := `{"kind":"result","result":{"kind":"accepted"}}`
	if ack, err := repo.ApplyTerminalFrame(context.Background(), device.ID, 7, 5, renameCommand.PublicID, strings.Repeat("b", 64), renameTerminal, now.Add(10*time.Second)); err != nil || ack != 7 {
		t.Fatalf("apply rename terminal: ack=%d err=%v", ack, err)
	}
	if err := database.First(&storedThread, thread.ID).Error; err != nil || storedThread.Title != "Renamed thread" {
		t.Fatalf("rename final state: %q %v", storedThread.Title, err)
	}
	failedRename := &domainagent.Command{PublicID: "agcmd_abababababababababababababababab", Kind: "thread.rename"}
	queuedFailedRename, err := repo.QueueThreadCommand(
		context.Background(), "36234567-89ab-4def-8123-456789abcdef", strings.Repeat("e", 64), 7,
		thread.PublicID, "", json.RawMessage(`{"name":"Must not apply"}`), failedRename, now.Add(10*time.Second),
	)
	if err != nil {
		t.Fatal(err)
	}
	failedTerminal := `{"kind":"error","error":{"code":"provider_error","message":"rename failed"}}`
	if ack, err := repo.ApplyTerminalFrame(context.Background(), device.ID, 8, queuedFailedRename.ServerSeq, failedRename.PublicID, strings.Repeat("f", 64), failedTerminal, now.Add(11*time.Second)); err != nil || ack != 8 {
		t.Fatalf("apply failed rename terminal: ack=%d err=%v", ack, err)
	}
	if err := database.First(&storedThread, thread.ID).Error; err != nil || storedThread.Status != "active" || storedThread.Title != "Renamed thread" {
		t.Fatalf("failed rename damaged thread: %#v %v", storedThread, err)
	}

	forkCommand := &domainagent.Command{PublicID: "agcmd_ffffffffffffffffffffffffffffffff", Kind: "thread.lifecycle"}
	forked, err := repo.ForkThread(
		context.Background(), "41234567-89ab-4def-8123-456789abcdef", strings.Repeat("c", 64), 7,
		thread.PublicID, &domainagent.Thread{PublicID: "agth_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", UserID: 7}, forkCommand, now.Add(11*time.Second),
	)
	if err != nil || forked.SourceThreadRef != nil || forked.Status != "queued" {
		t.Fatalf("queue thread fork: %#v %v", forked, err)
	}
	var forkRow model.AgentCommand
	if err := database.First(&forkRow, "public_id = ?", forkCommand.PublicID).Error; err != nil || forkRow.ServerSeq != 7 || forkRow.ThreadID == nil || *forkRow.ThreadID != forked.ID {
		t.Fatalf("fork command ownership: %#v %v", forkRow, err)
	}
	forkTerminal := `{"kind":"result","result":{"kind":"thread-forked","sourceThreadRef":"source-thread-fork"}}`
	if ack, err := repo.ApplyTerminalFrame(context.Background(), device.ID, 9, 7, forkCommand.PublicID, strings.Repeat("d", 64), forkTerminal, now.Add(12*time.Second)); err != nil || ack != 9 {
		t.Fatalf("apply fork terminal: ack=%d err=%v", ack, err)
	}
	var storedFork model.AgentThread
	if err := database.First(&storedFork, forked.ID).Error; err != nil || storedFork.SourceThreadRef == nil || *storedFork.SourceThreadRef != "source-thread-fork" || storedFork.Status != "active" {
		t.Fatalf("fork final state: %#v %v", storedFork, err)
	}
}
