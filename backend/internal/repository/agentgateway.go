package repository

import (
	"context"
	"encoding/json"
	"time"

	domainagent "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/agentgateway"
)

type AgentGatewayRepository interface {
	CreateCredential(context.Context, *domainagent.Credential) error
	EnrollDevice(context.Context, string, *domainagent.Device, time.Time) (*domainagent.Device, error)
	ListDevices(context.Context, uint) ([]domainagent.Device, error)
	GetDevice(context.Context, uint, string) (*domainagent.Device, error)
	GetDeviceByPublicID(context.Context, string) (*domainagent.Device, error)
	RenameDevice(context.Context, uint, string, string) (*domainagent.Device, error)
	RevokeDevice(context.Context, uint, string, time.Time) error
	CreateDeviceCredential(context.Context, uint, *domainagent.Credential) error
	GetCredential(context.Context, string, string) (*domainagent.Credential, error)
	ConsumeChallengeAndCreateConnection(context.Context, uint, uint, *domainagent.Credential, time.Time) (*domainagent.Credential, error)
	ConsumeConnection(context.Context, string, time.Time) (*domainagent.Device, error)
	EnqueueCommand(context.Context, uint, string, *domainagent.Command) (*domainagent.Command, error)
	ListCommandsForDelivery(context.Context, uint, uint64, int) ([]domainagent.Command, error)
	MarkCommandDelivered(context.Context, uint, uint, time.Time) error
	AckServerCommands(context.Context, uint, uint64, time.Time) error
	ApplyTerminalFrame(context.Context, uint, uint64, uint64, string, string, string, time.Time) (uint64, error)
	ApplyEventFrame(context.Context, uint, uint, uint64, string, *domainagent.Event, time.Time) (uint64, error)
	BeginRuntimeProof(context.Context, uint, string, *domainagent.RuntimeProfile, *domainagent.RuntimeProofChallenge, time.Time) (*domainagent.RuntimeProfile, *domainagent.RuntimeProofChallenge, error)
	CompleteRuntimeProof(context.Context, uint, uint, uint, int64, string, time.Time, time.Time) error
	SyncWorkspaces(context.Context, uint, uint, uint, []domainagent.Workspace, time.Time) error
	ListRuntimeProfiles(context.Context, uint, string) ([]domainagent.RuntimeProfile, error)
	ListWorkspaces(context.Context, uint, string) ([]domainagent.Workspace, error)
	QueueResourceRefresh(context.Context, string, string, uint, string, string, string, string, *domainagent.Command, time.Time) (*domainagent.Command, error)
	GetResourceSnapshot(context.Context, uint, string, string, string, string) (*domainagent.ResourceSnapshot, error)
	QueueThreadCommand(context.Context, string, string, uint, string, string, json.RawMessage, *domainagent.Command, time.Time) (*domainagent.Command, error)
	ForkThread(context.Context, string, string, uint, string, *domainagent.Thread, *domainagent.Command, time.Time) (*domainagent.Thread, error)
	StartThread(context.Context, string, string, *domainagent.Thread, *domainagent.Turn, *domainagent.Command, time.Time) (*domainagent.Thread, *domainagent.Turn, error)
	ListThreads(context.Context, uint, int) ([]domainagent.Thread, error)
	GetThread(context.Context, uint, string) (*domainagent.Thread, error)
	StartTurn(context.Context, string, string, *domainagent.Turn, *domainagent.Command, time.Time) (*domainagent.Turn, error)
	ListTurns(context.Context, uint, string, int) ([]domainagent.Turn, error)
	ListEvents(context.Context, uint, string, uint64, int) ([]domainagent.Event, error)
	ListInteractions(context.Context, uint, string, string, int) ([]domainagent.Interaction, error)
	RespondInteraction(context.Context, string, string, uint, string, json.RawMessage, *domainagent.Command, time.Time) (*domainagent.Interaction, error)
}
