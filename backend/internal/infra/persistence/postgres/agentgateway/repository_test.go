package agentgateway

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	domainagent "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/agentgateway"
	model "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/models"
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
	if err := database.AutoMigrate(&model.AgentDevice{}, &model.AgentRuntimeProfile{}, &model.AgentWorkspace{}); err != nil {
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
		ManifestJSON: `{"commands":["thread.read"]}`,
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
	if err := database.AutoMigrate(&model.AgentDevice{}, &model.AgentRuntimeProfile{}, &model.AgentWorkspace{}); err != nil {
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
		&model.Conversation{}, &model.Message{},
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
		ManifestJSON: `{"commands":["thread.read"]}`,
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
	imageFile := model.FileObject{
		FileID: "file_0123456789abcdef0123456789abcdef", UserID: 7, Purpose: "agent_input",
		FileName: "fixture.png", MimeType: "image/png", DetectedMIME: "image/png",
		FileCategory: "image", SizeBytes: 12, SHA256: strings.Repeat("f", 64),
		StoragePath: "fixture.png", Status: "active",
	}
	if err := database.Create(&imageFile).Error; err != nil {
		t.Fatalf("create artifact file: %v", err)
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
		RunID: "run_0123456789abcdef0123456789abcdef", Status: "awaiting_thread", InputJSON: `[{"kind":"text","text":"run tests"},{"kind":"artifact","artifactRef":"agart_0123456789abcdef0123456789abcdef"}]`, SettingsJSON: `{}`,
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

	interactionEvent := &domainagent.Event{
		PublicID: "agev_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Kind: "interaction.requested",
		SourceThreadRef: "source-thread-1", SourceTurnRef: "source-turn-1", SourceRequestRef: "request-1",
		PayloadJSON: `{"method":"item/commandExecution/requestApproval","request":{"command":"git status"}}`, OccurredAt: now.Add(4 * time.Second),
	}
	if ack, err := repo.ApplyEventFrame(context.Background(), device.ID, profile.ID, 4, strings.Repeat("5", 64), interactionEvent, now.Add(4*time.Second)); err != nil || ack.Acknowledged != 4 {
		t.Fatalf("apply interaction event: ack=%v err=%v", ack, err)
	}
	var interaction model.AgentInteraction
	if err := database.First(&interaction, "source_request_ref = ?", "request-1").Error; err != nil {
		t.Fatal(err)
	}
	if interaction.Kind != "command_approval" || !jsonEqual(interaction.RequestJSON, `{"command":"git status"}`) {
		t.Fatalf("interaction projection mismatch: %#v", interaction)
	}
	wrongResponse := json.RawMessage(`{"kind":"user-input","answers":{"question":"yes"}}`)
	if _, err := repo.RespondInteraction(context.Background(), "10234567-89ab-4def-8123-456789abcdef", strings.Repeat("a", 64), 7, interaction.PublicID, wrongResponse, &domainagent.Command{PublicID: "agcmd_eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", Kind: "interaction.respond"}, now.Add(5*time.Second)); err == nil {
		t.Fatal("interaction accepted a response for a different semantic kind")
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
	coalescedResource, err := repo.QueueResourceRefresh(
		context.Background(), "31234567-89ab-4def-8123-456789abcdef", strings.Repeat("a", 64), 7,
		device.PublicID, "", workspace.PublicID, "sessions",
		&domainagent.Command{PublicID: "agcmd_abbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Kind: "resource.refresh"}, now.Add(7*time.Second),
	)
	if err != nil || coalescedResource.PublicID != queued.PublicID {
		t.Fatalf("unfinished resource refresh was duplicated: %#v %v", coalescedResource, err)
	}
	resourceTerminal := `{"kind":"result","result":{"kind":"resource","resource":"sessions","data":{"data":[{"sourceThreadRef":"source-thread-1","name":"Local session","status":"active","historyLoaded":false},{"sourceThreadRef":"source-thread-2","name":"Imported session","modelProvider":"openai","status":"archived","createdAt":1786615200,"updatedAt":1786615260,"historyLoaded":false}]}}}`
	if ack, err := repo.ApplyTerminalFrame(context.Background(), device.ID, 6, 4, resourceCommand.PublicID, strings.Repeat("9", 64), resourceTerminal, now.Add(8*time.Second)); err != nil || ack != 6 {
		t.Fatalf("apply resource terminal: ack=%d err=%v", ack, err)
	}
	snapshot, err := repo.GetResourceSnapshot(context.Background(), 7, device.PublicID, "", workspace.PublicID, "sessions")
	if err != nil || snapshot.WorkspacePublicID != workspace.PublicID || snapshot.ProfilePublicID != profile.PublicID ||
		!jsonEqual(snapshot.DataJSON, `{"data":[{"sourceThreadRef":"source-thread-1","name":"Local session","status":"active","historyLoaded":false},{"sourceThreadRef":"source-thread-2","name":"Imported session","modelProvider":"openai","status":"archived","createdAt":1786615200,"updatedAt":1786615260,"historyLoaded":false}]}`) {
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
	historyTerminal := `{"kind":"result","result":{"kind":"thread-read","session":{"sourceThreadRef":"source-thread-2","name":"Imported session","historyLoaded":true,"createdAt":1786615200,"updatedAt":1786615260,"messages":[{"role":"user","content":"inspect the repository","createdAt":1786615200},{"role":"assistant","content":"ready","reasoningContent":"checked files","createdAt":1786615260}]}}}`
	if err := projectTerminalResult(database, &device, &model.AgentBridgeFrame{}, &storedHistoryCommand, historyTerminal, now.Add(10*time.Second)); err != nil {
		t.Fatalf("project thread history: %v", err)
	}
	if err := database.Where("conversation_id = ?", importedConversation.ID).Order("id ASC").Find(&importedMessages).Error; err != nil ||
		len(importedMessages) != 2 || importedMessages[0].Role != "user" || importedMessages[1].ReasoningContent != "checked files" ||
		importedMessages[1].ParentMessageID == nil || *importedMessages[1].ParentMessageID != importedMessages[0].ID {
		t.Fatalf("imported messages mismatch: %#v %v", importedMessages, err)
	}
	updatedResourceTerminal := `{"kind":"result","result":{"kind":"resource","resource":"sessions","data":{"data":[{"sourceThreadRef":"source-thread-2","name":"Renamed imported session","modelProvider":"openai","status":"active","createdAt":1786615200,"updatedAt":1786615320,"historyLoaded":false}]}}}`
	if err := projectTerminalResult(database, &device, &model.AgentBridgeFrame{}, &model.AgentCommand{
		UserID: 7, RuntimeProfileID: &profile.ID, WorkspaceID: &workspace.ID, Kind: "resource.refresh",
		PayloadJSON: `{"resource":{"name":"sessions"}}`,
	}, updatedResourceTerminal, now.Add(11*time.Second)); err != nil {
		t.Fatalf("update session projection: %v", err)
	}
	if err := database.Where("conversation_id = ?", importedConversation.ID).Order("id ASC").Find(&importedMessages).Error; err != nil || len(importedMessages) != 2 {
		t.Fatalf("updated imported messages mismatch: %#v %v", importedMessages, err)
	}
	if err := database.First(&importedConversation, importedConversation.ID).Error; err != nil || importedConversation.Title != "Renamed imported session" || importedConversation.MessageCount != 2 || importedConversation.Status != "active" {
		t.Fatalf("updated imported conversation mismatch: %#v %v", importedConversation, err)
	}
	if err := database.First(&importedThread, importedThread.ID).Error; err != nil || importedThread.Status != "active" || importedThread.HistoryStatus != "loaded" {
		t.Fatalf("updated imported thread mismatch: %#v %v", importedThread, err)
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
	if ack, err := repo.ApplyEventFrame(context.Background(), device.ID, profile.ID, 7, strings.Repeat("0", 64), itemStarted, now.Add(13*time.Second)); err != nil || ack.Acknowledged != 7 {
		t.Fatalf("apply item start: ack=%v err=%v", ack, err)
	}
	itemCompleted := &domainagent.Event{
		PublicID: "agev_cccccccccccccccccccccccccccccccc", Kind: "item/completed",
		SourceThreadRef: "source-thread-1", SourceTurnRef: "source-turn-1", SourceItemRef: "source-item-1",
		PayloadJSON: `{"item":{"type":"agentMessage","text":"done"},"completedAtMs":2}`, OccurredAt: now.Add(14 * time.Second),
	}
	if ack, err := repo.ApplyEventFrame(context.Background(), device.ID, profile.ID, 8, strings.Repeat("1", 64), itemCompleted, now.Add(14*time.Second)); err != nil || ack.Acknowledged != 8 {
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
	if err != nil || replayed.Acknowledged != 8 || replayed.ConversationID != 0 || replayed.RunID != "" {
		t.Fatalf("projected bridge replay was republished: %#v %v", replayed, err)
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

func TestQueueWorkspaceMutationRequiresManagedWorkspaceAndCapability(t *testing.T) {
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
	if _, err := repo.QueueWorkspaceMutation(
		context.Background(), "71234567-89ab-4def-8123-456789abcdef", strings.Repeat("7", 64), 7,
		device.PublicID, workspace.PublicID, "workspace.rename", "renamed",
		&domainagent.Command{PublicID: "agcmd_7123456789abcdef0123456789abcdef", Kind: "workspace.rename"}, now,
	); err == nil {
		t.Fatal("unmanaged Workspace mutation was queued")
	}
	if err := database.Model(&workspace).Update("managed", true).Error; err != nil {
		t.Fatal(err)
	}
	queued, err := repo.QueueWorkspaceMutation(
		context.Background(), "81234567-89ab-4def-8123-456789abcdef", strings.Repeat("8", 64), 7,
		device.PublicID, workspace.PublicID, "workspace.rename", "renamed",
		&domainagent.Command{PublicID: "agcmd_8123456789abcdef0123456789abcdef", Kind: "workspace.rename"}, now,
	)
	if err != nil || queued.ServerSeq != 1 || !strings.Contains(queued.PayloadJSON, `"name":"renamed"`) {
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
	responses := map[string]string{
		"command_approval": `{"kind":"approval","decision":"accept"}`,
		"file_approval":    `{"kind":"approval","decision":"decline"}`,
		"user_input":       `{"kind":"user-input","answers":{"question":"yes"}}`,
		"permission":       `{"kind":"permission","decision":"accept"}`,
		"mcp_elicitation":  `{"kind":"mcp-elicitation","decision":"decline"}`,
		"dynamic_tool":     `{"kind":"dynamic-tool","success":true,"content":[]}`,
	}
	for method, expectedKind := range cases {
		kind, request, err := projectInteractionRequest(`{"method":"` + method + `","request":{"title":"fixture"}}`)
		if err != nil || kind != expectedKind || !jsonEqual(request, `{"title":"fixture"}`) {
			t.Fatalf("projectInteractionRequest(%q) = %q, %s, %v", method, kind, request, err)
		}
		wrong := json.RawMessage(`{"kind":"approval"}`)
		if kind == "command_approval" || kind == "file_approval" {
			wrong = json.RawMessage(`{"kind":"user-input"}`)
		}
		if !interactionResponseMatchesKind(kind, json.RawMessage(responses[kind])) || interactionResponseMatchesKind(kind, wrong) {
			t.Fatalf("response kind validation failed for %q", kind)
		}
	}
	if _, _, err := projectInteractionRequest(`{"method":"unknown","request":{}}`); err == nil {
		t.Fatal("unknown interaction method accepted")
	}
}
