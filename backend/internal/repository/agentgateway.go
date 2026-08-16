package repository

import (
	"context"
	"encoding/json"
	"time"

	domainagent "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/agentgateway"
)

type AgentGatewayRepository interface {
	CreateEnrollmentChallenge(context.Context, *domainagent.DeviceEnrollmentChallenge) error
	GetEnrollmentChallenge(context.Context, string) (*domainagent.DeviceEnrollmentChallenge, error)
	ConsumeEnrollmentChallengeAndEnroll(context.Context, uint, *domainagent.Device, time.Time) (*domainagent.Device, error)
	ListDevices(context.Context, uint) ([]domainagent.Device, error)
	GetDevice(context.Context, uint, string) (*domainagent.Device, error)
	GetDeviceByPublicID(context.Context, string) (*domainagent.Device, error)
	RenameDevice(context.Context, uint, string, string) (*domainagent.Device, error)
	RevokeDevice(context.Context, uint, string, time.Time) error
	CreateDeviceCredential(context.Context, uint, *domainagent.Credential) error
	GetCredential(context.Context, string, string) (*domainagent.Credential, error)
	ConsumeChallengeAndCreateConnection(context.Context, uint, uint, *domainagent.Credential, time.Time) (*domainagent.Credential, error)
	ConsumeConnection(context.Context, string, time.Time) (*domainagent.Device, error)
	ListCommandsForDelivery(context.Context, uint, uint64, int) ([]domainagent.Command, error)
	GetCommand(context.Context, uint, string) (*domainagent.Command, error)
	MarkCommandDelivered(context.Context, uint, uint, time.Time) error
	AckServerCommands(context.Context, uint, uint64, time.Time) error
	ApplyTerminalFrame(context.Context, uint, uint64, uint64, string, string, string, time.Time) (uint64, error)
	ApplyEventFrame(context.Context, uint, uint, uint64, string, *domainagent.Event, time.Time) (*domainagent.AppliedEventFrame, error)
	ListPendingConversationEvents(context.Context, uint, int) ([]domainagent.AppliedEventFrame, error)
	MarkConversationEventProjected(context.Context, uint, time.Time) error
	BeginRuntimeProof(context.Context, uint, string, *domainagent.RuntimeProfile, *domainagent.RuntimeProofChallenge, time.Time) (*domainagent.RuntimeProfile, *domainagent.RuntimeProofChallenge, error)
	CompleteRuntimeProof(context.Context, uint, uint, uint, int64, string, string, time.Time, time.Time) error
	TouchRuntimePresence(context.Context, uint, uint, time.Time, time.Time) error
	SyncWorkspaces(context.Context, uint, uint, uint, []domainagent.Workspace, time.Time) error
	ListRuntimeProfiles(context.Context, uint, string) ([]domainagent.RuntimeProfile, error)
	ListWorkspaces(context.Context, uint, string) ([]domainagent.Workspace, error)
	ResolveExecutionTarget(context.Context, uint, string, string, string, time.Time) (string, error)
	CreateArtifact(context.Context, uint, string, string, *domainagent.Artifact) (*domainagent.Artifact, error)
	ListArtifactsForCommand(context.Context, uint, uint, []string) ([]domainagent.Artifact, error)
	GetArtifactForCommand(context.Context, string, string) (*domainagent.Artifact, *domainagent.Command, error)
	QueueResourceRefresh(context.Context, string, string, uint, string, string, string, string, *domainagent.Command, time.Time) (*domainagent.Command, error)
	QueueWorkspaceRegistration(context.Context, string, string, uint, string, string, string, bool, *domainagent.Command, time.Time) (*domainagent.Command, error)
	GetResourceSnapshot(context.Context, uint, string, string, string, string) (*domainagent.ResourceSnapshot, error)
	QueueTurnInterrupt(context.Context, string, string, uint, string, *domainagent.Command, time.Time) (*domainagent.Command, error)
	QueueThreadLifecycle(context.Context, string, string, uint, string, string, *domainagent.Command, time.Time) (*domainagent.Command, error)
	QueueThreadHistory(context.Context, uint, uint, *domainagent.Command, time.Time) (*domainagent.Thread, *domainagent.Command, error)
	StartThread(context.Context, string, string, *domainagent.Thread, *domainagent.Turn, *domainagent.Command, time.Time) (*domainagent.Thread, *domainagent.Turn, error)
	GetThreadByConversation(context.Context, uint, uint) (*domainagent.Thread, error)
	StartTurn(context.Context, string, string, *domainagent.Turn, *domainagent.Command, time.Time) (*domainagent.Turn, error)
	GetTurnByRunID(context.Context, uint, string) (*domainagent.Turn, error)
	ListInteractions(context.Context, uint, string, string, int) ([]domainagent.Interaction, error)
	RespondInteraction(context.Context, string, string, uint, string, json.RawMessage, *domainagent.Command, time.Time) (*domainagent.Interaction, error)
}
