package agentgateway

import "time"

const (
	DeviceStatusActive  = "active"
	DeviceStatusRevoked = "revoked"

	CredentialKindEnrollment = "enrollment"
	CredentialKindChallenge  = "challenge"
	CredentialKindConnection = "connection"

	RuntimeStatusProving = "proving"
	RuntimeStatusReady   = "ready"
)

type Device struct {
	ID                     uint
	PublicID               string
	UserID                 uint
	EnrollmentCredentialID uint
	Name                   string
	Platform               string
	PublicKey              []byte
	PublicKeyFingerprint   string
	CredentialVersion      uint
	Status                 string
	NextServerSeq          uint64
	LastAckedServerSeq     uint64
	LastAckedBridgeSeq     uint64
	LastSeenAt             *time.Time
	RevokedAt              *time.Time
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

type Command struct {
	ID               uint
	PublicID         string
	UserID           uint
	DeviceID         uint
	RuntimeProfileID *uint
	WorkspaceID      *uint
	ThreadID         *uint
	TurnID           *uint
	InteractionID    *uint
	ServerSeq        uint64
	Kind             string
	PayloadJSON      string
	State            string
	DeliveredAt      *time.Time
	AckedAt          *time.Time
	TerminalJSON     string
	CompletedAt      *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
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
	Status           string
	LastSeenAt       time.Time
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
	SourceThreadRef   *string
	Title             string
	Status            string
	IsPinned          bool
	LabelsJSON        string
	SharePolicy       string
	GitSHA            *string
	GitBranch         *string
	GitOriginURL      *string
	LastEventSeq      uint64
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type ThreadMetadataPatch struct {
	IsPinned    *bool
	LabelsJSON  *string
	SharePolicy *string
}

type ThreadSnapshot struct {
	Thread       Thread
	Turns        []Turn
	Items        []Item
	Interactions []Interaction
	SnapshotSeq  uint64
}

type Turn struct {
	ID             uint
	PublicID       string
	UserID         uint
	ThreadID       uint
	ThreadPublicID string
	SourceTurnRef  *string
	Status         string
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

type Interaction struct {
	ID               uint
	PublicID         string
	UserID           uint
	ThreadID         uint
	TurnID           *uint
	TurnPublicID     string
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
	ID             uint
	PublicID       string
	UserID         uint
	DeviceID       uint
	Provider       string
	Status         string
	RemoteKeyID    *int64
	CredentialHash string
	VerifiedAt     *time.Time
	LeaseExpiresAt *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
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
