package agentgateway

import "time"

type ArtifactDoc struct {
	ArtifactID  string `json:"artifactId"`
	WorkspaceID string `json:"workspaceId"`
	FileName    string `json:"fileName"`
	MimeType    string `json:"mimeType"`
	SizeBytes   int64  `json:"sizeBytes"`
	SHA256      string `json:"sha256"`
}

type RuntimeProfileDoc struct {
	ProfileID      string              `json:"profileId"`
	DeviceID       string              `json:"deviceId"`
	Provider       string              `json:"provider"`
	Status         string              `json:"status"`
	VerifiedAt     *time.Time          `json:"verifiedAt"`
	LeaseExpiresAt *time.Time          `json:"leaseExpiresAt"`
	Manifest       ProviderManifestDoc `json:"manifest"`
}

type ProviderManifestDoc struct {
	Provider         string                    `json:"provider"`
	RuntimeVersion   string                    `json:"runtimeVersion"`
	ProtocolVersion  string                    `json:"protocolVersion"`
	SchemaHash       string                    `json:"schemaHash"`
	Commands         []string                  `json:"commands"`
	Resources        ProviderResourcesDoc      `json:"resources"`
	InputKinds       []string                  `json:"inputKinds"`
	ThreadSettings   ProviderThreadSettingsDoc `json:"threadSettings"`
	InteractionKinds []string                  `json:"interactionKinds"`
}

type ProviderResourcesDoc struct {
	Profile   []string `json:"profile"`
	Workspace []string `json:"workspace"`
}

type ProviderThreadSettingsDoc struct {
	Model           bool     `json:"model"`
	ReasoningEffort []string `json:"reasoningEffort"`
	ApprovalPolicy  []string `json:"approvalPolicy"`
	SandboxPolicy   []string `json:"sandboxPolicy"`
}

type WorkspaceDoc struct {
	WorkspaceID string    `json:"workspaceId"`
	DeviceID    string    `json:"deviceId"`
	ProfileID   string    `json:"profileId"`
	Name        string    `json:"name"`
	Status      string    `json:"status"`
	LastSeenAt  time.Time `json:"lastSeenAt"`
}

type ResourceSnapshotDoc struct {
	Resource    string      `json:"resource"`
	Scope       string      `json:"scope"`
	DeviceID    string      `json:"deviceId"`
	ProfileID   string      `json:"profileId"`
	WorkspaceID string      `json:"workspaceId,omitempty"`
	Data        interface{} `json:"data"`
	RefreshedAt time.Time   `json:"refreshedAt"`
}

type CommandDoc struct {
	CommandID string `json:"commandId"`
	Status    string `json:"status"`
}

type EnrollmentChallengeResponseDoc struct {
	ErrorMsg string `json:"errorMsg"`
	Data     struct {
		ChallengeID string `json:"challengeId"`
		Canonical   string `json:"canonical"`
		ExpiresAt   string `json:"expiresAt"`
	} `json:"data"`
}

type DeviceResponseDoc struct {
	ErrorMsg string `json:"errorMsg"`
	Data     struct {
		DeviceID   string  `json:"deviceId"`
		UserID     string  `json:"userId"`
		Name       string  `json:"name"`
		Platform   string  `json:"platform"`
		Status     string  `json:"status"`
		Online     bool    `json:"online"`
		CreatedAt  string  `json:"createdAt"`
		UpdatedAt  string  `json:"updatedAt"`
		LastSeenAt *string `json:"lastSeenAt"`
	} `json:"data"`
}

type DeviceListResponseDoc struct {
	ErrorMsg string                  `json:"errorMsg"`
	Data     []DeviceResponseDocData `json:"data"`
}

type DeviceResponseDocData struct {
	DeviceID   string  `json:"deviceId"`
	UserID     string  `json:"userId"`
	Name       string  `json:"name"`
	Platform   string  `json:"platform"`
	Status     string  `json:"status"`
	Online     bool    `json:"online"`
	CreatedAt  string  `json:"createdAt"`
	UpdatedAt  string  `json:"updatedAt"`
	LastSeenAt *string `json:"lastSeenAt"`
}

type DeviceRevokeResponseDoc struct {
	ErrorMsg string `json:"errorMsg"`
	Data     struct {
		Revoked bool `json:"revoked"`
	} `json:"data"`
}

type RuntimeProfileListResponseDoc struct {
	ErrorMsg string              `json:"errorMsg"`
	Data     []RuntimeProfileDoc `json:"data"`
}

type WorkspaceListResponseDoc struct {
	ErrorMsg string         `json:"errorMsg"`
	Data     []WorkspaceDoc `json:"data"`
}

type ArtifactResponseDoc struct {
	ErrorMsg string      `json:"errorMsg"`
	Data     ArtifactDoc `json:"data"`
}

type ResourceSnapshotResponseDoc struct {
	ErrorMsg string              `json:"errorMsg"`
	Data     ResourceSnapshotDoc `json:"data"`
}

type CommandResponseDoc struct {
	ErrorMsg string     `json:"errorMsg"`
	Data     CommandDoc `json:"data"`
}
