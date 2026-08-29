package conversation

import (
	"testing"

	model "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/conversation"
)

func TestConversationProjectFieldsUsesExecutionWorkspaceForGateway(t *testing.T) {
	projectID, projectName := conversationProjectFields(&model.Conversation{
		ExecutionType:        model.ExecutionTypeGateway,
		ExecutionWorkspaceID: "workspace_a",
		ProjectPublicID:      "cloud_project",
		ProjectName:          "Cloud project",
	})
	if projectID != "workspace_a" || projectName != "Cloud project" {
		t.Fatalf("conversationProjectFields() = (%q, %q)", projectID, projectName)
	}
}

func TestConversationResponseIncludesGatewayApprovalSettings(t *testing.T) {
	response := toConversationResponse(&model.Conversation{
		ExecutionType: "gateway", ApprovalPolicy: "on-request", ApprovalsReviewer: "auto_review", SandboxPolicy: "workspace-write",
	})
	if response.ApprovalPolicy != "on-request" || response.ApprovalsReviewer != "auto_review" || response.SandboxPolicy != "workspace-write" {
		t.Fatalf("approval settings were not projected: %#v", response)
	}
}

func TestMessageResponseRedactsBlockedAssistantContent(t *testing.T) {
	response := toMessageResponseWithRun(model.Message{
		Role: "assistant", Status: "blocked", Content: "sensitive response", Attachments: `[{"file_id":"secret"}]`,
		ModerationEventID: "event_1", ModerationCategoriesJSON: `["policy"]`,
		KnowledgeSources: []model.MessageKnowledgeSource{{FileID: "file_1", Preview: "private excerpt"}},
	}, model.Run{ModerationState: "blocked"})

	if response.Content != "" || response.Attachments != "[]" || len(response.KnowledgeSources) != 0 {
		t.Fatalf("blocked assistant content was not redacted: %#v", response)
	}
	if response.Moderation == nil || response.Moderation.EventID != "event_1" ||
		len(response.Moderation.Categories) != 1 || response.Moderation.Categories[0] != "policy" {
		t.Fatalf("moderation metadata was not preserved: %#v", response.Moderation)
	}
}
