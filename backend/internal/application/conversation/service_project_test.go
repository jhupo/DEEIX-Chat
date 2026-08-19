package conversation

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	domainconversation "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/conversation"
	domainmcp "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/mcp"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/config"
)

type gatewayProjectListerStub struct {
	projects []GatewayProject
	err      error
}

func (s gatewayProjectListerStub) ListProjects(context.Context, uint, string) ([]GatewayProject, error) {
	return s.projects, s.err
}

func TestListExecutionProjectsMapsGatewayProjects(t *testing.T) {
	now := time.Date(2026, time.August, 15, 10, 0, 0, 0, time.UTC)
	service := &Service{gatewayProjects: gatewayProjectListerStub{projects: []GatewayProject{
		{ProjectID: " workspace_deeix ", ProfileID: "profile_codex", Name: " DEEIX ", Managed: true, UpdatedAt: now},
	}}}

	projects, err := service.ListExecutionProjects(
		context.Background(),
		7,
		"active",
		domainconversation.ExecutionTypeGateway,
		"agd_device",
	)
	if err != nil {
		t.Fatalf("ListExecutionProjects() error = %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("ListExecutionProjects() returned %d projects, want 1", len(projects))
	}
	project := projects[0]
	if project.PublicID != "workspace_deeix" || project.Name != "DEEIX" || project.Status != "active" || !project.Managed {
		t.Fatalf("gateway project = %#v", project)
	}
	if !project.UpdatedAt.Equal(now) || project.MCPDefaultMode != domainconversation.ConversationProjectMCPDefaultModeInherit {
		t.Fatalf("gateway project metadata = %#v", project)
	}
	resolved, err := service.resolveGatewayProject(context.Background(), 7, "agd_device", "workspace_deeix")
	if err != nil {
		t.Fatalf("resolveGatewayProject() error = %v", err)
	}
	if resolved.ProfileID != "profile_codex" {
		t.Fatalf("resolved gateway project = %#v", resolved)
	}
	conversations := []domainconversation.Conversation{{ExecutionWorkspaceID: "workspace_deeix"}, {ExecutionWorkspaceID: "workspace_recent"}}
	if err = service.applyGatewayProjectFields(context.Background(), 7, "agd_device", conversations); err != nil {
		t.Fatalf("applyGatewayProjectFields() error = %v", err)
	}
	if conversations[0].ProjectPublicID != "workspace_deeix" || conversations[0].ProjectName != "DEEIX" {
		t.Fatalf("gateway conversation project = %#v", conversations[0])
	}
	if conversations[1].ProjectPublicID != "" || conversations[1].ProjectName != "" {
		t.Fatalf("hidden gateway workspace leaked as project = %#v", conversations[1])
	}
}

func TestListExecutionProjectsRequiresGatewayDevice(t *testing.T) {
	service := &Service{gatewayProjects: gatewayProjectListerStub{}}
	_, err := service.ListExecutionProjects(
		context.Background(),
		7,
		"active",
		domainconversation.ExecutionTypeGateway,
		"",
	)
	if !errors.Is(err, ErrInvalidExecutionTarget) {
		t.Fatalf("ListExecutionProjects() error = %v, want ErrInvalidExecutionTarget", err)
	}
}

func TestResolveGatewayProjectNormalizesMissingBinding(t *testing.T) {
	service := &Service{gatewayProjects: gatewayProjectListerStub{err: ErrExecutionBindingNotFound}}
	_, err := service.resolveGatewayProject(context.Background(), 7, "agd_device", "workspace_deeix")
	if !errors.Is(err, ErrInvalidExecutionTarget) {
		t.Fatalf("resolveGatewayProject() error = %v, want ErrInvalidExecutionTarget", err)
	}
}

func TestNormalizeConversationProjectInputInheritClearsMCPDefaults(t *testing.T) {
	input, err := normalizeConversationProjectInput(ConversationProjectInput{
		Name:              " Project ",
		MCPDefaultMode:    domainconversation.ConversationProjectMCPDefaultModeInherit,
		DefaultMCPToolIDs: []uint{3, 3, 2},
		DefaultSkillIDs:   []uint{5, 0, 5, 4},
	})
	if err != nil {
		t.Fatalf("normalizeConversationProjectInput() error = %v", err)
	}
	if input.Name != "Project" || len(input.DefaultMCPToolIDs) != 0 {
		t.Fatalf("normalized project = %#v", input)
	}
	if !reflect.DeepEqual(input.DefaultSkillIDs, []uint{5, 4}) {
		t.Fatalf("default Skill IDs = %v, want [5 4]", input.DefaultSkillIDs)
	}
}

func TestNewProjectDefaultIDs(t *testing.T) {
	got := newProjectDefaultIDs([]uint{4, 2, 3}, []uint{2, 4})
	if !reflect.DeepEqual(got, []uint{3}) {
		t.Fatalf("newProjectDefaultIDs() = %v, want [3]", got)
	}
}

func TestValidateConversationProjectDefaultsPreservesUnavailableExistingSelections(t *testing.T) {
	service := &Service{cfg: config.NewRuntime(config.Config{MCPMaxSelectedToolsPerMessage: 1})}
	current := &domainconversation.ConversationProject{
		MCPDefaultMode:    domainconversation.ConversationProjectMCPDefaultModeCustom,
		DefaultMCPToolIDs: []uint{3, 2},
		DefaultSkillIDs:   []uint{5, 4},
	}
	err := service.validateConversationProjectDefaults(
		context.Background(),
		1,
		current.MCPDefaultMode,
		current.DefaultMCPToolIDs,
		current.DefaultSkillIDs,
		current,
	)
	if err != nil {
		t.Fatalf("validateConversationProjectDefaults() error = %v", err)
	}
}

func TestValidateConversationProjectDefaultsRejectsMultipleImageProcessors(t *testing.T) {
	service := &Service{
		cfg: config.NewRuntime(config.Config{MCPMaxSelectedToolsPerMessage: 4}),
		mcpRepo: selectedToolRuntimeMCPRepositoryStub{
			listToolsByIDs: func(context.Context, []uint) ([]domainmcp.Tool, error) {
				return []domainmcp.Tool{
					{ID: 1, AttachmentInputMode: domainmcp.AttachmentInputModeImage},
					{ID: 2, AttachmentInputMode: domainmcp.AttachmentInputModeImage},
				}, nil
			},
		},
	}
	err := service.validateConversationProjectDefaults(
		context.Background(),
		1,
		domainconversation.ConversationProjectMCPDefaultModeCustom,
		[]uint{1, 2},
		nil,
		nil,
	)
	if !errors.Is(err, ErrInvalidConversationProject) {
		t.Fatalf("expected multiple image processors to be rejected, got %v", err)
	}
}
