package agentgateway

import "time"

const (
	DeviceStatusActive  = "active"
	DeviceStatusRevoked = "revoked"

	CredentialKindChallenge  = "challenge"
	CredentialKindConnection = "connection"

	RuntimeStatusProving = "proving"
	RuntimeStatusReady   = "ready"
)

type Device struct {
	ID                   uint
	PublicID             string
	UserID               uint
	Name                 string
	Platform             string
	AgentVersion         string
	PublicKey            []byte
	PublicKeyFingerprint string
	CredentialVersion    uint
	Status               string
	Online               bool
	NextServerSeq        uint64
	LastAckedServerSeq   uint64
	LastAckedBridgeSeq   uint64
	LastSeenAt           *time.Time
	RevokedAt            *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

type DeviceEnrollmentChallenge struct {
	ID                   uint
	PublicID             string
	UserID               uint
	UserPublicID         string
	RemoteUserID         int64
	Name                 string
	Platform             string
	PublicKey            []byte
	PublicKeyFingerprint string
	Nonce                string
	ExpiresAt            time.Time
	ConsumedAt           *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

type Command struct {
	ID                   uint
	PublicID             string
	ConversationPublicID string
	UserID               uint
	DeviceID             uint
	RuntimeProfileID     *uint
	WorkspaceID          *uint
	ThreadID             *uint
	TurnID               *uint
	InteractionID        *uint
	ServerSeq            uint64
	Kind                 string
	PayloadJSON          string
	State                string
	DeliveredAt          *time.Time
	AckedAt              *time.Time
	TerminalJSON         string
	CompletedAt          *time.Time
	DeviceOnline         bool
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

type Workspace struct {
	ID               uint
	PublicID         string
	UserID           uint
	DeviceID         uint
	RuntimeProfileID uint
	DevicePublicID   string
	ProfilePublicID  string
	Name             string
	Managed          bool
	Hidden           bool
	Status           string
	LastSeenAt       time.Time
	LastActivityAt   time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type ResourceSnapshot struct {
	ID                uint
	PublicID          string
	UserID            uint
	DeviceID          uint
	RuntimeProfileID  uint
	WorkspaceID       uint
	DevicePublicID    string
	ProfilePublicID   string
	WorkspacePublicID string
	Name              string
	DataJSON          string
	RefreshedAt       time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type Artifact struct {
	ID                uint
	PublicID          string
	UserID            uint
	WorkspaceID       uint
	WorkspacePublicID string
	FileObjectID      uint
	FileID            string
	FileName          string
	MimeType          string
	SizeBytes         int64
	SHA256            string
	Status            string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type Thread struct {
	ID                uint
	PublicID          string
	UserID            uint
	DeviceID          uint
	RuntimeProfileID  uint
	WorkspaceID       uint
	DevicePublicID    string
	ProfilePublicID   string
	WorkspacePublicID string
	ConversationID    uint
	SourceThreadRef   *string
	Title             string
	Status            string
	HistoryStatus     string
	HistoryError      string
	GitSHA            *string
	GitBranch         *string
	GitOriginURL      *string
	LastEventSeq      uint64
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type Turn struct {
	ID             uint
	PublicID       string
	UserID         uint
	ThreadID       uint
	ThreadPublicID string
	RunID          string
	SourceTurnRef  *string
	Status         string
	ErrorCode      string
	ErrorMessage   string
	InputJSON      string
	SettingsJSON   string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type Item struct {
	ID               uint
	PublicID         string
	UserID           uint
	ThreadID         uint
	TurnID           *uint
	ThreadPublicID   string
	TurnPublicID     string
	RuntimeProfileID uint
	SourceItemRef    string
	Kind             string
	Status           string
	DataJSON         string
	LastEventSeq     uint64
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type Event struct {
	ID               uint
	PublicID         string
	UserID           uint
	DeviceID         uint
	RuntimeProfileID *uint
	WorkspaceID      *uint
	ThreadID         *uint
	TurnID           *uint
	TurnPublicID     string
	ThreadSeq        *uint64
	Kind             string
	SourceThreadRef  string
	SourceTurnRef    string
	SourceItemRef    string
	SourceRequestRef string
	PayloadJSON      string
	OccurredAt       time.Time
	CreatedAt        time.Time
}

// AppliedEventFrame carries the durable provider event and its conversation binding.
type AppliedEventFrame struct {
	Acknowledged          uint64
	Event                 Event
	ConversationID        uint
	ConversationPublicIDs []string
	RunID                 string
}

type Interaction struct {
	ID               uint
	PublicID         string
	UserID           uint
	ThreadID         uint
	TurnID           *uint
	TurnPublicID     string
	RunID            string
	ThreadPublicID   string
	RuntimeProfileID uint
	SourceRequestRef string
	Kind             string
	RequestJSON      string
	Status           string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type Credential struct {
	ID                      uint
	PublicID                string
	UserID                  uint
	DeviceID                *uint
	ParentCredentialID      *uint
	Kind                    string
	TokenHash               string
	DerivationKeyVersion    uint
	DeviceCredentialVersion uint
	ExpiresAt               time.Time
	ConsumedAt              *time.Time
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

type RuntimeProfile struct {
	ID                uint
	PublicID          string
	UserID            uint
	DeviceID          uint
	Provider          string
	Status            string
	RemoteKeyID       *int64
	CredentialHash    string
	ManifestJSON      string
	VerifiedAt        *time.Time
	LeaseExpiresAt    *time.Time
	PresenceExpiresAt *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type RuntimeProofChallenge struct {
	ID         uint
	PublicID   string
	UserID     uint
	DeviceID   uint
	ProfileID  uint
	Nonce      string
	ExpiresAt  time.Time
	ConsumedAt *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}
