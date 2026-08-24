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
