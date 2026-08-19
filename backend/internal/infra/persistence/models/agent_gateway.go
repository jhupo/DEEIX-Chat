package model

import "time"

type AgentDevice struct {
	ControlPlaneModel
	PublicID             string     `gorm:"size:64;not null;uniqueIndex:uk_agent_devices_public_id"`
	UserID               uint       `gorm:"not null;index:idx_agent_devices_user_status,priority:1"`
	Name                 string     `gorm:"size:128;not null"`
	Platform             string     `gorm:"size:32;not null"`
	PublicKey            []byte     `gorm:"type:bytea;not null"`
	PublicKeyFingerprint string     `gorm:"size:64;not null;uniqueIndex:uk_agent_devices_public_key_fingerprint"`
	CredentialVersion    uint       `gorm:"not null;default:1"`
	Status               string     `gorm:"size:32;not null;index:idx_agent_devices_user_status,priority:2"`
	NextServerSeq        uint64     `gorm:"not null;default:1"`
	LastAckedServerSeq   uint64     `gorm:"not null;default:0"`
	LastAckedBridgeSeq   uint64     `gorm:"not null;default:0"`
	LastSeenAt           *time.Time `gorm:"index"`
	RevokedAt            *time.Time `gorm:"index"`
}

func (AgentDevice) TableName() string { return "agent_devices" }

type AgentDeviceEnrollmentChallenge struct {
	ControlPlaneModel
	PublicID             string     `gorm:"size:64;not null;uniqueIndex:uk_agent_device_enrollment_challenges_public_id"`
	UserID               uint       `gorm:"not null;index"`
	UserPublicID         string     `gorm:"size:64;not null"`
	RemoteUserID         int64      `gorm:"not null"`
	Name                 string     `gorm:"size:128;not null"`
	Platform             string     `gorm:"size:32;not null"`
	PublicKey            []byte     `gorm:"type:bytea;not null"`
	PublicKeyFingerprint string     `gorm:"size:64;not null;index"`
	Nonce                string     `gorm:"size:64;not null"`
	ExpiresAt            time.Time  `gorm:"not null;index"`
	ConsumedAt           *time.Time `gorm:"index"`
}

func (AgentDeviceEnrollmentChallenge) TableName() string {
	return "agent_device_enrollment_challenges"
}

// AgentCredential stores only hashes and derivation inputs. Raw challenge and
// connection bearer values are never persisted.
type AgentCredential struct {
	ControlPlaneModel
	PublicID                string     `gorm:"size:64;not null;uniqueIndex:uk_agent_credentials_public_id"`
	UserID                  uint       `gorm:"not null;index:idx_agent_credentials_user_kind,priority:1"`
	DeviceID                *uint      `gorm:"index:idx_agent_credentials_device_kind,priority:1"`
	ParentCredentialID      *uint      `gorm:"uniqueIndex:uk_agent_credentials_parent"`
	Kind                    string     `gorm:"size:32;not null;index:idx_agent_credentials_user_kind,priority:2;index:idx_agent_credentials_device_kind,priority:2"`
	TokenHash               string     `gorm:"size:64;not null;uniqueIndex:uk_agent_credentials_token_hash"`
	DerivationKeyVersion    uint       `gorm:"not null"`
	DeviceCredentialVersion uint       `gorm:"not null;default:0"`
	ExpiresAt               time.Time  `gorm:"not null;index:idx_agent_credentials_expires_at"`
	ConsumedAt              *time.Time `gorm:"index"`
}

func (AgentCredential) TableName() string { return "agent_credentials" }

type AgentCommand struct {
	ControlPlaneModel
	PublicID         string     `gorm:"size:64;not null;uniqueIndex:uk_agent_commands_public_id"`
	UserID           uint       `gorm:"not null;index:idx_agent_commands_user"`
	DeviceID         uint       `gorm:"not null;uniqueIndex:uk_agent_commands_device_seq,priority:1;index:idx_agent_commands_device_state,priority:1"`
	RuntimeProfileID *uint      `gorm:"index"`
	WorkspaceID      *uint      `gorm:"index"`
	ThreadID         *uint      `gorm:"index"`
	TurnID           *uint      `gorm:"index"`
	InteractionID    *uint      `gorm:"index"`
	ServerSeq        uint64     `gorm:"not null;uniqueIndex:uk_agent_commands_device_seq,priority:2"`
	Kind             string     `gorm:"size:64;not null"`
	PayloadJSON      string     `gorm:"type:jsonb;not null"`
	State            string     `gorm:"size:32;not null;index:idx_agent_commands_device_state,priority:2"`
	DeliveredAt      *time.Time `gorm:"index"`
	AckedAt          *time.Time `gorm:"index"`
	TerminalJSON     string     `gorm:"type:jsonb;not null;default:'{}'"`
	CompletedAt      *time.Time `gorm:"index"`
}

func (AgentCommand) TableName() string { return "agent_commands" }

type AgentBridgeFrame struct {
	ControlPlaneModel
	DeviceID    uint      `gorm:"not null;uniqueIndex:uk_agent_bridge_frames_device_seq,priority:1"`
	BridgeSeq   uint64    `gorm:"not null;uniqueIndex:uk_agent_bridge_frames_device_seq,priority:2"`
	Kind        string    `gorm:"size:32;not null"`
	CommandID   *uint     `gorm:"uniqueIndex:uk_agent_bridge_frames_command"`
	PayloadHash string    `gorm:"size:64;not null"`
	PayloadJSON string    `gorm:"type:jsonb;not null"`
	ReceivedAt  time.Time `gorm:"not null;index"`
}

func (AgentBridgeFrame) TableName() string { return "agent_bridge_frames" }

type AgentRuntimeProfile struct {
	ControlPlaneModel
	PublicID          string     `gorm:"size:64;not null;uniqueIndex:uk_agent_runtime_profiles_device_public,priority:2"`
	UserID            uint       `gorm:"not null;index:idx_agent_runtime_profiles_user"`
	DeviceID          uint       `gorm:"not null;uniqueIndex:uk_agent_runtime_profiles_device_public,priority:1"`
	Provider          string     `gorm:"size:32;not null"`
	Status            string     `gorm:"size:32;not null;index"`
	RemoteKeyID       *int64     `gorm:"index"`
	CredentialHash    string     `gorm:"size:64;not null;default:''"`
	ManifestJSON      string     `gorm:"type:jsonb;not null;default:'{}'"`
	VerifiedAt        *time.Time `gorm:"index"`
	LeaseExpiresAt    *time.Time `gorm:"index"`
	PresenceExpiresAt *time.Time `gorm:"index"`
}

func (AgentRuntimeProfile) TableName() string { return "agent_runtime_profiles" }

type AgentRuntimeProofChallenge struct {
	ControlPlaneModel
	PublicID   string     `gorm:"size:64;not null;uniqueIndex:uk_agent_runtime_proof_challenges_public_id"`
	UserID     uint       `gorm:"not null;index"`
	DeviceID   uint       `gorm:"not null;index"`
	ProfileID  uint       `gorm:"not null;index"`
	Nonce      string     `gorm:"size:64;not null"`
	ExpiresAt  time.Time  `gorm:"not null;index"`
	ConsumedAt *time.Time `gorm:"index"`
}

func (AgentRuntimeProofChallenge) TableName() string { return "agent_runtime_proof_challenges" }

type AgentWorkspace struct {
	ControlPlaneModel
	PublicID         string    `gorm:"size:64;not null;uniqueIndex:uk_agent_workspaces_device_public,priority:2"`
	UserID           uint      `gorm:"not null;index"`
	DeviceID         uint      `gorm:"not null;uniqueIndex:uk_agent_workspaces_device_public,priority:1;index:idx_agent_workspaces_device_status,priority:1"`
	RuntimeProfileID uint      `gorm:"not null;index"`
	Name             string    `gorm:"size:128;not null"`
	Managed          bool      `gorm:"not null;default:false;index"`
	Hidden           bool      `gorm:"not null;default:false;index"`
	Status           string    `gorm:"size:32;not null;index:idx_agent_workspaces_device_status,priority:2"`
	LastSeenAt       time.Time `gorm:"not null;index"`
}

func (AgentWorkspace) TableName() string { return "agent_workspaces" }

type AgentArtifact struct {
	ControlPlaneModel
	PublicID     string `gorm:"size:64;not null;uniqueIndex:uk_agent_artifacts_public_id"`
	UserID       uint   `gorm:"not null;index"`
	WorkspaceID  uint   `gorm:"not null;uniqueIndex:uk_agent_artifacts_workspace_file,priority:1"`
	FileObjectID uint   `gorm:"not null;uniqueIndex:uk_agent_artifacts_workspace_file,priority:2"`
	FileName     string `gorm:"size:255;not null"`
	MimeType     string `gorm:"size:128;not null"`
	SizeBytes    int64  `gorm:"not null"`
	SHA256       string `gorm:"size:64;not null"`
	Status       string `gorm:"size:32;not null;index"`
}

func (AgentArtifact) TableName() string { return "agent_artifacts" }

type AgentResourceSnapshot struct {
	ControlPlaneModel
	PublicID         string    `gorm:"size:64;not null;uniqueIndex:uk_agent_resource_snapshots_public_id"`
	UserID           uint      `gorm:"not null;index"`
	DeviceID         uint      `gorm:"not null;index"`
	RuntimeProfileID uint      `gorm:"not null;uniqueIndex:uk_agent_resource_snapshots_target,priority:1"`
	WorkspaceID      uint      `gorm:"not null;default:0;uniqueIndex:uk_agent_resource_snapshots_target,priority:2"`
	Name             string    `gorm:"size:64;not null;uniqueIndex:uk_agent_resource_snapshots_target,priority:3"`
	DataJSON         string    `gorm:"type:jsonb;not null"`
	RefreshedAt      time.Time `gorm:"not null;index"`
}

func (AgentResourceSnapshot) TableName() string { return "agent_resource_snapshots" }

type AgentThread struct {
	ControlPlaneModel
	PublicID         string  `gorm:"size:64;not null;uniqueIndex:uk_agent_threads_public_id"`
	UserID           uint    `gorm:"not null;index:idx_agent_threads_user_updated,priority:1"`
	DeviceID         uint    `gorm:"not null;index"`
	RuntimeProfileID uint    `gorm:"not null;uniqueIndex:uk_agent_threads_profile_source,priority:1,where:source_thread_ref IS NOT NULL"`
	WorkspaceID      uint    `gorm:"not null;index"`
	ConversationID   uint    `gorm:"not null;uniqueIndex:uk_agent_threads_conversation;check:chk_agent_threads_conversation_id,conversation_id > 0"`
	SourceThreadRef  *string `gorm:"size:256;uniqueIndex:uk_agent_threads_profile_source,priority:2,where:source_thread_ref IS NOT NULL"`
	Title            string  `gorm:"size:256;not null;default:''"`
	Status           string  `gorm:"size:32;not null;index"`
	HistoryStatus    string  `gorm:"size:16;not null;default:'loaded';index"`
	HistoryError     string  `gorm:"size:4096;not null;default:''"`
	GitSHA           *string `gorm:"size:64"`
	GitBranch        *string `gorm:"size:256"`
	GitOriginURL     *string `gorm:"size:2048"`
	LastEventSeq     uint64  `gorm:"not null;default:0"`
}

func (AgentThread) TableName() string { return "agent_threads" }

type AgentTurn struct {
	ControlPlaneModel
	PublicID      string  `gorm:"size:64;not null;uniqueIndex:uk_agent_turns_public_id"`
	UserID        uint    `gorm:"not null;index"`
	ThreadID      uint    `gorm:"not null;index:idx_agent_turns_thread_created,priority:1;uniqueIndex:uk_agent_turns_thread_source,priority:1,where:source_turn_ref IS NOT NULL"`
	RunID         string  `gorm:"size:64;not null;uniqueIndex:uk_agent_turns_run_id;check:chk_agent_turns_run_id,run_id <> ''"`
	SourceTurnRef *string `gorm:"size:256;uniqueIndex:uk_agent_turns_thread_source,priority:2,where:source_turn_ref IS NOT NULL"`
	Status        string  `gorm:"size:32;not null;index"`
	ErrorCode     string  `gorm:"size:128;not null;default:''"`
	ErrorMessage  string  `gorm:"size:4096;not null;default:''"`
	InputJSON     string  `gorm:"type:jsonb;not null"`
	SettingsJSON  string  `gorm:"type:jsonb;not null"`
}

func (AgentTurn) TableName() string { return "agent_turns" }

type AgentItem struct {
	ControlPlaneModel
	PublicID         string `gorm:"size:64;not null;uniqueIndex:uk_agent_items_public_id"`
	UserID           uint   `gorm:"not null;index"`
	ThreadID         uint   `gorm:"not null;index:idx_agent_items_thread_event,priority:1"`
	TurnID           *uint  `gorm:"index"`
	RuntimeProfileID uint   `gorm:"not null;uniqueIndex:uk_agent_items_profile_source,priority:1"`
	SourceItemRef    string `gorm:"size:256;not null;uniqueIndex:uk_agent_items_profile_source,priority:2"`
	Kind             string `gorm:"size:64;not null;index"`
	Status           string `gorm:"size:32;not null;index"`
	DataJSON         string `gorm:"type:jsonb;not null"`
	LastEventSeq     uint64 `gorm:"not null;index:idx_agent_items_thread_event,priority:2"`
}

func (AgentItem) TableName() string { return "agent_items" }

type AgentEvent struct {
	ControlPlaneModel
	PublicID                string     `gorm:"size:64;not null;uniqueIndex:uk_agent_events_public_id"`
	BridgeFrameID           uint       `gorm:"not null;uniqueIndex:uk_agent_events_bridge_frame"`
	UserID                  uint       `gorm:"not null;index"`
	DeviceID                uint       `gorm:"not null;index"`
	RuntimeProfileID        *uint      `gorm:"index"`
	WorkspaceID             *uint      `gorm:"index"`
	ThreadID                *uint      `gorm:"uniqueIndex:uk_agent_events_thread_seq,priority:1,where:thread_id IS NOT NULL AND thread_seq IS NOT NULL"`
	TurnID                  *uint      `gorm:"index"`
	ThreadSeq               *uint64    `gorm:"uniqueIndex:uk_agent_events_thread_seq,priority:2,where:thread_id IS NOT NULL AND thread_seq IS NOT NULL"`
	Kind                    string     `gorm:"size:256;not null;index"`
	SourceThreadRef         string     `gorm:"size:256;not null;default:'';index"`
	SourceTurnRef           string     `gorm:"size:256;not null;default:'';index"`
	SourceItemRef           string     `gorm:"size:256;not null;default:''"`
	SourceRequestRef        string     `gorm:"size:256;not null;default:'';index"`
	PayloadJSON             string     `gorm:"type:jsonb;not null"`
	OccurredAt              time.Time  `gorm:"not null;index"`
	ConversationProjectedAt *time.Time `gorm:"index"`
}

func (AgentEvent) TableName() string { return "agent_events" }

type AgentInteraction struct {
	ControlPlaneModel
	PublicID         string `gorm:"size:64;not null;uniqueIndex:uk_agent_interactions_public_id"`
	UserID           uint   `gorm:"not null;index:idx_agent_interactions_user_status,priority:1"`
	ThreadID         uint   `gorm:"not null;index"`
	TurnID           *uint  `gorm:"index"`
	RuntimeProfileID uint   `gorm:"not null;uniqueIndex:uk_agent_interactions_profile_source,priority:1"`
	SourceRequestRef string `gorm:"size:256;not null;uniqueIndex:uk_agent_interactions_profile_source,priority:2"`
	Kind             string `gorm:"size:128;not null"`
	RequestJSON      string `gorm:"type:jsonb;not null"`
	Status           string `gorm:"size:32;not null;index:idx_agent_interactions_user_status,priority:2"`
}

func (AgentInteraction) TableName() string { return "agent_interactions" }

type AgentIdempotencyRecord struct {
	ControlPlaneModel
	UserID            uint   `gorm:"not null;uniqueIndex:uk_agent_idempotency_user_operation_key,priority:1"`
	Operation         string `gorm:"size:64;not null;uniqueIndex:uk_agent_idempotency_user_operation_key,priority:2"`
	Key               string `gorm:"size:36;not null;uniqueIndex:uk_agent_idempotency_user_operation_key,priority:3"`
	RequestHash       string `gorm:"size:64;not null"`
	ResultPublicID    string `gorm:"size:64;not null;default:''"`
	SecondaryPublicID string `gorm:"size:64;not null;default:''"`
}

func (AgentIdempotencyRecord) TableName() string { return "agent_idempotency_records" }
