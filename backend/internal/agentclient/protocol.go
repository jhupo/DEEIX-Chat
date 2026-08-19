package agentclient

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	bridgeVersion  = 2
	bridgeProtocol = "deeix.bridge.v2"
	// thread/read(includeTurns=true) carries the complete local transcript.
	bridgeMaxPayload = 64 << 20
)

type bridgeFrame struct {
	Version          int               `json:"version"`
	Type             string            `json:"type"`
	ProfileID        string            `json:"profileId,omitempty"`
	ChallengeID      string            `json:"challengeId,omitempty"`
	Challenge        string            `json:"challenge,omitempty"`
	ExpiresAt        string            `json:"expiresAt,omitempty"`
	Proof            string            `json:"proof,omitempty"`
	LeaseExpiresAt   string            `json:"leaseExpiresAt,omitempty"`
	Workspaces       []bridgeWorkspace `json:"workspaces,omitempty"`
	Manifest         *ProviderManifest `json:"manifest,omitempty"`
	AckServerSeq     uint64            `json:"ackServerSeq,omitempty"`
	AckBridgeSeq     uint64            `json:"ackBridgeSeq,omitempty"`
	DeviceID         string            `json:"deviceId,omitempty"`
	HeartbeatSeconds int               `json:"heartbeatSeconds,omitempty"`
	ServerSeq        uint64            `json:"serverSeq,omitempty"`
	BridgeSeq        uint64            `json:"bridgeSeq,omitempty"`
	CommandID        string            `json:"commandId,omitempty"`
	Command          json.RawMessage   `json:"command,omitempty"`
	Outcome          json.RawMessage   `json:"outcome,omitempty"`
	Event            json.RawMessage   `json:"event,omitempty"`
	Artifacts        *[]ArtifactGrant  `json:"artifacts,omitempty"`
	ErrorCode        string            `json:"errorCode,omitempty"`
	ErrorMessage     string            `json:"errorMessage,omitempty"`
}

type bridgeWorkspace struct {
	WorkspaceID string `json:"workspaceId"`
	Name        string `json:"name"`
	Managed     bool   `json:"managed,omitempty"`
	Hidden      bool   `json:"hidden,omitempty"`
	Revision    string `json:"revision"`
}

type ArtifactGrant struct {
	ArtifactRef string `json:"artifactRef"`
	FileName    string `json:"fileName"`
	MimeType    string `json:"mimeType"`
	SizeBytes   int64  `json:"sizeBytes"`
	SHA256      string `json:"sha256"`
	ExpiresAt   string `json:"expiresAt"`
	Grant       string `json:"grant"`
}

type Settings struct {
	Model           string `json:"model,omitempty"`
	ReasoningEffort string `json:"reasoningEffort,omitempty"`
	ApprovalPolicy  string `json:"approvalPolicy,omitempty"`
	SandboxPolicy   string `json:"sandboxPolicy,omitempty"`
}

type AgentInput struct {
	Kind        string `json:"kind"`
	Text        string `json:"text,omitempty"`
	ArtifactRef string `json:"artifactRef,omitempty"`
	ResourceRef string `json:"resourceRef,omitempty"`
}

type AgentCommand struct {
	Kind             string             `json:"kind"`
	DeviceID         string             `json:"deviceId"`
	ProfileID        string             `json:"profileId"`
	WorkspaceID      string             `json:"workspaceId,omitempty"`
	ThreadID         string             `json:"threadId,omitempty"`
	SourceThreadRef  string             `json:"sourceThreadRef,omitempty"`
	TurnID           string             `json:"turnId,omitempty"`
	SourceTurnRef    string             `json:"sourceTurnRef,omitempty"`
	Action           string             `json:"action,omitempty"`
	Name             string             `json:"name,omitempty"`
	Path             string             `json:"path,omitempty"`
	Create           bool               `json:"create,omitempty"`
	GitInfo          map[string]*string `json:"gitInfo,omitempty"`
	Target           map[string]any     `json:"target,omitempty"`
	Input            []AgentInput       `json:"input,omitempty"`
	Settings         *Settings          `json:"settings,omitempty"`
	Scope            string             `json:"scope,omitempty"`
	InteractionID    string             `json:"interactionId,omitempty"`
	SourceRequestRef string             `json:"sourceRequestRef,omitempty"`
	Response         json.RawMessage    `json:"response,omitempty"`
	TargetVersion    string             `json:"targetVersion,omitempty"`
	Resource         *struct {
		Scope string `json:"scope"`
		Name  string `json:"name"`
	} `json:"resource,omitempty"`
}

type ProviderManifest struct {
	AgentVersion    string   `json:"agentVersion,omitempty"`
	Provider        string   `json:"provider"`
	RuntimeVersion  string   `json:"runtimeVersion"`
	ProtocolVersion string   `json:"protocolVersion"`
	SchemaHash      string   `json:"schemaHash"`
	Commands        []string `json:"commands"`
	Resources       struct {
		Profile   []string `json:"profile"`
		Workspace []string `json:"workspace"`
	} `json:"resources"`
	InputKinds     []string `json:"inputKinds"`
	ThreadSettings struct {
		Model           bool     `json:"model"`
		ReasoningEffort []string `json:"reasoningEffort"`
		ApprovalPolicy  []string `json:"approvalPolicy"`
		SandboxPolicy   []string `json:"sandboxPolicy"`
	} `json:"threadSettings"`
	InteractionKinds []string `json:"interactionKinds"`
}

func parseAgentCommand(raw json.RawMessage) (AgentCommand, error) {
	var command AgentCommand
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&command); err != nil || requireEOF(decoder) != nil {
		return AgentCommand{}, errors.New("gateway command is invalid")
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return AgentCommand{}, errors.New("gateway command is invalid")
	}
	if !validRef(command.DeviceID, 256) || !validRef(command.ProfileID, 256) {
		return AgentCommand{}, errors.New("gateway command target is invalid")
	}
	workspace := []string{"kind", "deviceId", "profileId", "workspaceId"}
	thread := append(append([]string{}, workspace...), "threadId", "sourceThreadRef")
	turn := append(append([]string{}, thread...), "turnId", "sourceTurnRef")
	var allowed []string
	switch command.Kind {
	case "agent.update":
		allowed = []string{"kind", "deviceId", "profileId", "targetVersion"}
		if !validAgentVersion(command.TargetVersion) {
			return AgentCommand{}, errors.New("agent update version is invalid")
		}
	case "workspace.register":
		allowed = []string{"kind", "deviceId", "profileId", "path", "create"}
		if !validText(command.Path, 4096) || strings.ContainsRune(command.Path, 0) {
			return AgentCommand{}, errors.New("workspace path is invalid")
		}
	case "workspace.rename":
		allowed = append(workspace, "name")
		if !validText(command.Name, 512) || utf8.RuneCountInString(command.Name) > 128 {
			return AgentCommand{}, errors.New("workspace name is invalid")
		}
	case "workspace.unregister":
		allowed = workspace
	case "thread.create":
		allowed = append(workspace, "settings")
		if command.Settings == nil || !validSettings(*command.Settings) {
			return AgentCommand{}, errors.New("thread settings are invalid")
		}
	case "thread.lifecycle":
		allowed = append(thread, "action")
		if !containsString([]string{"resume", "fork", "archive", "unarchive", "delete"}, command.Action) {
			return AgentCommand{}, errors.New("thread lifecycle action is invalid")
		}
	case "thread.rename":
		allowed = append(thread, "name")
		if !validText(command.Name, 256) {
			return AgentCommand{}, errors.New("thread name is invalid")
		}
	case "thread.metadata.update":
		allowed = append(thread, "gitInfo")
		if len(command.GitInfo) == 0 || len(command.GitInfo) > 3 {
			return AgentCommand{}, errors.New("thread git metadata is invalid")
		}
		limits := map[string]int{"sha": 64, "branch": 256, "originUrl": 2048}
		for key, value := range command.GitInfo {
			limit, ok := limits[key]
			if !ok || value != nil && !validText(*value, limit) {
				return AgentCommand{}, errors.New("thread git metadata is invalid")
			}
		}
	case "thread.compact":
		allowed = thread
	case "thread.read":
		allowed = thread
	case "review.start":
		allowed = append(thread, "target")
		if err := validateReviewTarget(command.Target); err != nil {
			return AgentCommand{}, err
		}
	case "turn.start":
		allowed = append(thread, "input", "settings")
		if command.Settings == nil || !validSettings(*command.Settings) || !validInputs(command.Input) {
			return AgentCommand{}, errors.New("turn input or settings are invalid")
		}
	case "turn.steer":
		allowed = append(turn, "input")
		if !validInputs(command.Input) {
			return AgentCommand{}, errors.New("turn input is invalid")
		}
	case "turn.interrupt":
		allowed = turn
	case "interaction.respond":
		allowed = append(thread, "scope", "interactionId", "sourceRequestRef", "response")
		if command.Scope == "turn" {
			allowed = append(allowed, "turnId", "sourceTurnRef")
		} else if command.Scope != "thread" {
			return AgentCommand{}, errors.New("interaction scope is invalid")
		}
		if !validRef(command.InteractionID, 256) || !validRef(command.SourceRequestRef, 256) || !validInteractionResponse(command.Response) {
			return AgentCommand{}, errors.New("interaction response is invalid")
		}
	case "resource.refresh":
		if command.Resource == nil {
			return AgentCommand{}, errors.New("resource refresh target is invalid")
		}
		if command.Resource.Scope == "profile" {
			allowed = []string{"kind", "deviceId", "profileId", "resource"}
			if !containsString(profileResources, command.Resource.Name) {
				return AgentCommand{}, errors.New("profile resource is invalid")
			}
		} else if command.Resource.Scope == "workspace" {
			allowed = append(workspace, "resource")
			if !containsString(workspaceResources, command.Resource.Name) {
				return AgentCommand{}, errors.New("workspace resource is invalid")
			}
		} else {
			return AgentCommand{}, errors.New("resource scope is invalid")
		}
	default:
		return AgentCommand{}, fmt.Errorf("unsupported gateway command: %s", command.Kind)
	}
	if !exactJSONFields(fields, allowed) || !validateTargets(command) {
		return AgentCommand{}, errors.New("gateway command fields are invalid")
	}
	return command, nil
}

func validAgentVersion(value string) bool {
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return false
	}
	for _, part := range parts {
		if part == "" || len(part) > 9 {
			return false
		}
		for _, character := range part {
			if character < '0' || character > '9' {
				return false
			}
		}
	}
	return true
}

var profileResources = []string{"models", "model-capabilities", "permission-profiles", "apps", "mcp", "plugins", "auth-status"}
var workspaceResources = []string{"sessions", "skills", "hooks"}

var (
	base64URL43Pattern       = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)
	mimeTypePattern          = regexp.MustCompile(`^(?:image|audio)/[A-Za-z0-9.+-]{1,100}$`)
	artifactExtensionPattern = regexp.MustCompile(`^\.[A-Za-z0-9]{1,15}$`)
)

func validateTargets(command AgentCommand) bool {
	if command.Kind == "agent.update" || command.Kind == "workspace.register" {
		return true
	}
	if command.Kind == "resource.refresh" && command.Resource != nil && command.Resource.Scope == "profile" {
		return true
	}
	if !validRef(command.WorkspaceID, 256) {
		return false
	}
	if command.Kind == "workspace.rename" || command.Kind == "workspace.unregister" || command.Kind == "thread.create" || command.Kind == "resource.refresh" {
		return true
	}
	if !validRef(command.ThreadID, 256) || !validRef(command.SourceThreadRef, 256) {
		return false
	}
	if command.Kind == "turn.steer" || command.Kind == "turn.interrupt" || command.Kind == "interaction.respond" && command.Scope == "turn" {
		return validRef(command.TurnID, 256) && validRef(command.SourceTurnRef, 256)
	}
	return true
}

func validSettings(settings Settings) bool {
	return (settings.Model == "" || validText(settings.Model, 256)) &&
		(settings.ReasoningEffort == "" || containsString([]string{"low", "medium", "high", "xhigh"}, settings.ReasoningEffort)) &&
		(settings.ApprovalPolicy == "" || containsString([]string{"untrusted", "on-request", "never"}, settings.ApprovalPolicy)) &&
		(settings.SandboxPolicy == "" || containsString([]string{"read-only", "workspace-write"}, settings.SandboxPolicy))
}

func validInputs(inputs []AgentInput) bool {
	if len(inputs) == 0 || len(inputs) > 64 {
		return false
	}
	total := 0
	for _, input := range inputs {
		switch input.Kind {
		case "text":
			if input.ArtifactRef != "" || input.ResourceRef != "" || !validText(input.Text, 1<<20) {
				return false
			}
			total += len(input.Text)
		case "artifact":
			if input.Text != "" || input.ResourceRef != "" || !validRef(input.ArtifactRef, 256) {
				return false
			}
		case "skill", "app-mention":
			if input.Text != "" || input.ArtifactRef != "" || !validRef(input.ResourceRef, 256) {
				return false
			}
		default:
			return false
		}
	}
	return total <= 1<<20
}

func validInteractionResponse(raw json.RawMessage) bool {
	if len(raw) == 0 || !json.Valid(raw) {
		return false
	}
	var response map[string]json.RawMessage
	if json.Unmarshal(raw, &response) != nil {
		return false
	}
	var kind string
	if json.Unmarshal(response["kind"], &kind) != nil {
		return false
	}
	switch kind {
	case "approval":
		return exactJSONFields(response, []string{"kind", "decision"}) && validDecision(response["decision"])
	case "user-input":
		var answers map[string]string
		if !exactJSONFields(response, []string{"kind", "answers"}) || json.Unmarshal(response["answers"], &answers) != nil || len(answers) > 128 {
			return false
		}
		for key, answer := range answers {
			if !validRef(key, 256) || len(answer) > 64*1024 {
				return false
			}
		}
		return true
	case "permission":
		if !exactJSONFieldsOptional(response, []string{"kind", "decision"}, []string{"scope"}) || !validDecision(response["decision"]) {
			return false
		}
		if rawScope := response["scope"]; len(rawScope) > 0 {
			var scope string
			return json.Unmarshal(rawScope, &scope) == nil && containsString([]string{"turn", "session"}, scope)
		}
		return true
	case "dynamic-tool":
		var success bool
		var content []map[string]json.RawMessage
		if !exactJSONFields(response, []string{"kind", "success", "content"}) || json.Unmarshal(response["success"], &success) != nil || json.Unmarshal(response["content"], &content) != nil || len(content) > 64 {
			return false
		}
		for _, item := range content {
			var itemKind string
			if json.Unmarshal(item["kind"], &itemKind) != nil || !containsString([]string{"text", "image", "audio"}, itemKind) {
				return false
			}
			field := "url"
			if itemKind == "text" {
				field = "text"
			}
			if !exactJSONFields(item, []string{"kind", field}) {
				return false
			}
			var value string
			if json.Unmarshal(item[field], &value) != nil || !validText(value, 1<<20) {
				return false
			}
		}
		return true
	case "mcp-elicitation":
		if !exactJSONFieldsOptional(response, []string{"kind", "decision"}, []string{"content"}) || !validDecision(response["decision"]) {
			return false
		}
		if rawContent := response["content"]; len(rawContent) > 0 {
			var content map[string]string
			return json.Unmarshal(rawContent, &content) == nil && len(content) <= 128
		}
		return true
	default:
		return false
	}
}

func validateReviewTarget(target map[string]any) error {
	kind, _ := target["kind"].(string)
	switch kind {
	case "working-tree":
		if len(target) != 1 {
			return errors.New("review target is invalid")
		}
	case "base-branch":
		branch, ok := target["branch"].(string)
		if len(target) != 2 || !ok || !validText(branch, 256) {
			return errors.New("review target is invalid")
		}
	case "commit":
		sha, ok := target["sha"].(string)
		if len(target) != 2 || !ok || !validText(sha, 64) {
			return errors.New("review target is invalid")
		}
	default:
		return errors.New("review target is invalid")
	}
	return nil
}

func validateArtifactGrant(grant ArtifactGrant) error {
	if !validRef(grant.ArtifactRef, 256) || grant.FileName == "" || len(grant.FileName) > 255 ||
		!mimeTypePattern.MatchString(grant.MimeType) ||
		grant.SizeBytes < 1 || grant.SizeBytes > 100*1024*1024 || !validHex(grant.SHA256, 64) ||
		!base64URL43Pattern.MatchString(grant.Grant) {
		return errors.New("artifact grant is invalid")
	}
	if _, err := time.Parse(time.RFC3339Nano, grant.ExpiresAt); err != nil {
		return errors.New("artifact grant expiry is invalid")
	}
	return nil
}

func validTerminalOutcome(raw json.RawMessage) bool {
	var outcome map[string]json.RawMessage
	if json.Unmarshal(raw, &outcome) != nil {
		return false
	}
	var kind string
	if json.Unmarshal(outcome["kind"], &kind) != nil {
		return false
	}
	if kind == "result" {
		var result map[string]any
		return exactJSONFields(outcome, []string{"kind", "result"}) && json.Unmarshal(outcome["result"], &result) == nil
	}
	if kind == "error" {
		var value struct{ Code, Message string }
		return exactJSONFields(outcome, []string{"kind", "error"}) && json.Unmarshal(outcome["error"], &value) == nil && value.Code != "" && len(value.Code) <= 128 && value.Message != "" && len(value.Message) <= 4096
	}
	return false
}

func validProviderEvent(raw json.RawMessage) bool {
	var event map[string]json.RawMessage
	if json.Unmarshal(raw, &event) != nil {
		return false
	}
	var kind, occurredAt string
	if json.Unmarshal(event["kind"], &kind) != nil || kind == "" || len(kind) > 256 || json.Unmarshal(event["occurredAt"], &occurredAt) != nil {
		return false
	}
	if _, err := time.Parse(time.RFC3339Nano, occurredAt); err != nil {
		return false
	}
	var payload map[string]any
	return json.Unmarshal(event["payload"], &payload) == nil
}

func validText(value string, maxBytes int) bool {
	if value == "" || len(value) > maxBytes || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if character < 32 && character != '\n' && character != '\r' && character != '\t' {
			return false
		}
	}
	return true
}

func validDecision(raw json.RawMessage) bool {
	var value string
	return json.Unmarshal(raw, &value) == nil && (value == "accept" || value == "decline")
}

func exactJSONFields(fields map[string]json.RawMessage, required []string) bool {
	return exactJSONFieldsOptional(fields, required, nil)
}

func exactJSONFieldsOptional(fields map[string]json.RawMessage, required, optional []string) bool {
	allowed := make(map[string]bool, len(required)+len(optional))
	for _, key := range required {
		allowed[key] = true
		if _, ok := fields[key]; !ok {
			return false
		}
	}
	for _, key := range optional {
		allowed[key] = true
	}
	for key := range fields {
		if !allowed[key] {
			return false
		}
	}
	return true
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
