package agentgateway

import "time"

type ErrorDoc struct {
	ErrorMsg  string      `json:"errorMsg"`
	ErrorCode string      `json:"errorCode,omitempty"`
	Details   interface{} `json:"details,omitempty"`
	RequestID string      `json:"requestId,omitempty"`
	Data      interface{} `json:"data"`
}

type AgentTextInputDoc struct {
	Kind string `json:"kind" enums:"text"`
	Text string `json:"text"`
}

type AgentSettingsDoc struct {
	Model           string `json:"model,omitempty"`
	ReasoningEffort string `json:"reasoningEffort,omitempty" enums:"low,medium,high,xhigh"`
	ApprovalPolicy  string `json:"approvalPolicy,omitempty" enums:"untrusted,on-request,never"`
	SandboxPolicy   string `json:"sandboxPolicy,omitempty" enums:"read-only,workspace-write"`
}

type StartThreadRequestDoc struct {
	DeviceID    string              `json:"deviceId"`
	ProfileID   string              `json:"profileId"`
	WorkspaceID string              `json:"workspaceId"`
	Title       string              `json:"title,omitempty"`
	Settings    AgentSettingsDoc    `json:"settings"`
	Input       []AgentTextInputDoc `json:"input,omitempty"`
}

type StartTurnRequestDoc struct {
	Input    []AgentTextInputDoc `json:"input"`
	Settings AgentSettingsDoc    `json:"settings"`
}

type SteerTurnRequestDoc struct {
	Input []AgentTextInputDoc `json:"input"`
}

type RenameThreadRequestDoc struct {
	Name string `json:"name"`
}

type ReviewTargetDoc struct {
	Kind   string `json:"kind" enums:"working-tree,base-branch,commit"`
	Branch string `json:"branch,omitempty"`
	SHA    string `json:"sha,omitempty"`
}

type StartReviewRequestDoc struct {
	Target ReviewTargetDoc `json:"target"`
}

type InteractionContentDoc struct {
	Kind string `json:"kind" enums:"text,image,audio"`
	Text string `json:"text,omitempty"`
	URL  string `json:"url,omitempty"`
}

type RespondInteractionRequestDoc struct {
	Response struct {
		Kind     string            `json:"kind" enums:"approval,user-input,permission,mcp-elicitation,dynamic-tool"`
		Decision string            `json:"decision,omitempty" enums:"accept,decline"`
		Answers  map[string]string `json:"answers,omitempty"`
		Scope    string            `json:"scope,omitempty" enums:"turn,session"`
		Success  *bool             `json:"success,omitempty"`
		Content  interface{}       `json:"content,omitempty"`
	} `json:"response"`
}

type DeviceDoc struct {
	DeviceID   string  `json:"deviceId"`
	UserID     string  `json:"userId"`
	Name       string  `json:"name"`
	Platform   string  `json:"platform"`
	Status     string  `json:"status"`
	LastSeenAt *string `json:"lastSeenAt" extensions:"x-nullable,!x-omitempty"`
	CreatedAt  string  `json:"createdAt"`
	UpdatedAt  string  `json:"updatedAt"`
}

type RuntimeProfileDoc struct {
	ProfileID      string     `json:"profileId"`
	DeviceID       string     `json:"deviceId"`
	Provider       string     `json:"provider"`
	Status         string     `json:"status"`
	VerifiedAt     *time.Time `json:"verifiedAt" extensions:"x-nullable,!x-omitempty"`
	LeaseExpiresAt *time.Time `json:"leaseExpiresAt" extensions:"x-nullable,!x-omitempty"`
}

type WorkspaceDoc struct {
	WorkspaceID string    `json:"workspaceId"`
	DeviceID    string    `json:"deviceId"`
	ProfileID   string    `json:"profileId"`
	Name        string    `json:"name"`
	Status      string    `json:"status"`
	LastSeenAt  time.Time `json:"lastSeenAt"`
}

type ThreadDoc struct {
	ThreadID    string    `json:"threadId"`
	DeviceID    string    `json:"deviceId"`
	ProfileID   string    `json:"profileId"`
	WorkspaceID string    `json:"workspaceId"`
	Title       string    `json:"title"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type TurnDoc struct {
	TurnID    string    `json:"turnId"`
	ThreadID  string    `json:"threadId"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type EventDoc struct {
	EventID    string      `json:"eventId"`
	ThreadID   string      `json:"threadId"`
	TurnID     string      `json:"turnId,omitempty"`
	Seq        uint64      `json:"seq"`
	Kind       string      `json:"kind"`
	Payload    interface{} `json:"payload"`
	OccurredAt time.Time   `json:"occurredAt"`
}

type InteractionDoc struct {
	InteractionID string      `json:"interactionId"`
	ThreadID      string      `json:"threadId"`
	TurnID        string      `json:"turnId,omitempty"`
	Kind          string      `json:"kind"`
	Status        string      `json:"status"`
	Request       interface{} `json:"request"`
	CreatedAt     time.Time   `json:"createdAt"`
}

type ResourceSnapshotDoc struct {
	Resource    string      `json:"resource"`
	Scope       string      `json:"scope" enums:"profile,workspace"`
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

type DeviceRevokeDoc struct {
	Revoked bool `json:"revoked"`
}

type StartThreadDataDoc struct {
	Thread ThreadDoc `json:"thread"`
	Turn   *TurnDoc  `json:"turn,omitempty" extensions:"x-nullable"`
}

type EnrollmentDataDoc struct {
	EnrollmentCode string `json:"enrollmentCode"`
	ExpiresAt      string `json:"expiresAt"`
}

type EnrollmentResponseDoc struct {
	ErrorMsg string            `json:"errorMsg"`
	Data     EnrollmentDataDoc `json:"data"`
}
type DeviceResponseDoc struct {
	ErrorMsg string    `json:"errorMsg"`
	Data     DeviceDoc `json:"data"`
}
type DeviceRevokeResponseDoc struct {
	ErrorMsg string          `json:"errorMsg"`
	Data     DeviceRevokeDoc `json:"data"`
}
type DeviceListResponseDoc struct {
	ErrorMsg string      `json:"errorMsg"`
	Data     []DeviceDoc `json:"data"`
}
type RuntimeProfileListResponseDoc struct {
	ErrorMsg string              `json:"errorMsg"`
	Data     []RuntimeProfileDoc `json:"data"`
}
type WorkspaceListResponseDoc struct {
	ErrorMsg string         `json:"errorMsg"`
	Data     []WorkspaceDoc `json:"data"`
}
type ResourceSnapshotResponseDoc struct {
	ErrorMsg string              `json:"errorMsg"`
	Data     ResourceSnapshotDoc `json:"data"`
}
type CommandResponseDoc struct {
	ErrorMsg string     `json:"errorMsg"`
	Data     CommandDoc `json:"data"`
}
type ThreadResponseDoc struct {
	ErrorMsg string    `json:"errorMsg"`
	Data     ThreadDoc `json:"data"`
}
type ThreadListResponseDoc struct {
	ErrorMsg string      `json:"errorMsg"`
	Data     []ThreadDoc `json:"data"`
}
type StartThreadResponseDoc struct {
	ErrorMsg string             `json:"errorMsg"`
	Data     StartThreadDataDoc `json:"data"`
}
type TurnResponseDoc struct {
	ErrorMsg string  `json:"errorMsg"`
	Data     TurnDoc `json:"data"`
}
type TurnListResponseDoc struct {
	ErrorMsg string    `json:"errorMsg"`
	Data     []TurnDoc `json:"data"`
}
type EventListResponseDoc struct {
	ErrorMsg string     `json:"errorMsg"`
	Data     []EventDoc `json:"data"`
}
type InteractionResponseDoc struct {
	ErrorMsg string         `json:"errorMsg"`
	Data     InteractionDoc `json:"data"`
}
type InteractionListResponseDoc struct {
	ErrorMsg string           `json:"errorMsg"`
	Data     []InteractionDoc `json:"data"`
}
