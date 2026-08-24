package schema

import (
	"testing"

	model "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/models"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/testutil"
)

func TestBackfillGatewayConversationSettingsUsesLatestValidTurn(t *testing.T) {
	db := testutil.Postgres(t)
	if err := db.AutoMigrate(&model.Conversation{}, &model.AgentThread{}, &model.AgentTurn{}); err != nil {
		t.Fatal(err)
	}
	conversation := model.Conversation{
		UserID: 7, PublicID: "conversation-approval-backfill", Title: "Work", ExecutionType: "gateway",
		SessionKey: "conversation-approval-backfill-session", Status: "active", ContextPolicy: "{}",
	}
	if err := db.Create(&conversation).Error; err != nil {
		t.Fatal(err)
	}
	thread := model.AgentThread{
		PublicID: "agth_approval_backfill", UserID: 7, ConversationID: conversation.ID, Status: "active",
	}
	if err := db.Create(&thread).Error; err != nil {
		t.Fatal(err)
	}
	turns := []model.AgentTurn{
		{PublicID: "agturn_approval_manual", UserID: 7, ThreadID: thread.ID, RunID: "run_approval_manual", Status: "completed", InputJSON: "[]", SettingsJSON: `{"approvalPolicy":"on-request","approvalsReviewer":"user","sandboxPolicy":"workspace-write"}`},
		{PublicID: "agturn_approval_auto", UserID: 7, ThreadID: thread.ID, RunID: "run_approval_auto", Status: "completed", InputJSON: "[]", SettingsJSON: `{"approvalPolicy":"on-request","approvalsReviewer":"auto_review","sandboxPolicy":"workspace-write"}`},
		{PublicID: "agturn_approval_invalid", UserID: 7, ThreadID: thread.ID, RunID: "run_approval_invalid", Status: "failed", InputJSON: "[]", SettingsJSON: `{}`},
	}
	if err := db.Create(&turns).Error; err != nil {
		t.Fatal(err)
	}
	if err := backfillGatewayConversationSettings(db); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&conversation, conversation.ID).Error; err != nil {
		t.Fatal(err)
	}
	if conversation.ApprovalPolicy != "on-request" || conversation.ApprovalsReviewer != "auto_review" || conversation.SandboxPolicy != "workspace-write" {
		t.Fatalf("unexpected backfilled settings: %#v", conversation)
	}
}
