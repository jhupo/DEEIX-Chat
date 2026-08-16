package agentclient

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
)

const codexSchemaHash = "f72b2caa3cbfa4298de9e85c62dda6dfbaf2266ffeb916fed30615ca69ff8c74"

var codexVersionPattern = regexp.MustCompile(`(?m)^codex-cli\s+(\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?)\s*$`)
var codexAppIDPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,512}$`)

var codexInteractiveSourceKinds = []string{"cli", "vscode", "appServer", "unknown"}

var mappedServerRequests = map[string]bool{
	"item/commandExecution/requestApproval": true,
	"item/fileChange/requestApproval":       true,
	"item/tool/requestUserInput":            true,
	"mcpServer/elicitation/request":         true,
	"item/permissions/requestApproval":      true,
	"item/tool/call":                        true,
}

var mappedNotifications = map[string]bool{
	"error": true, "thread/started": true, "thread/status/changed": true, "thread/archived": true,
	"thread/deleted": true, "thread/unarchived": true, "thread/closed": true, "skills/changed": true,
	"thread/name/updated": true, "thread/goal/updated": true, "thread/goal/cleared": true, "thread/tokenUsage/updated": true,
	"turn/started": true, "hook/started": true, "turn/completed": true, "hook/completed": true,
	"turn/diff/updated": true, "turn/plan/updated": true, "item/started": true, "item/completed": true,
	"item/agentMessage/delta": true, "item/plan/delta": true, "item/commandExecution/outputDelta": true,
	"item/fileChange/patchUpdated": true, "serverRequest/resolved": true, "mcpServer/oauthLogin/completed": true,
	"mcpServer/startupStatus/updated": true, "account/updated": true, "account/rateLimits/updated": true,
	"app/list/updated": true, "fs/changed": true, "item/reasoning/summaryTextDelta": true,
	"item/reasoning/summaryPartAdded": true, "item/reasoning/textDelta": true, "thread/compacted": true,
	"model/rerouted": true, "model/verification": true, "model/safetyBuffering/updated": true,
	"warning": true, "configWarning": true, "account/login/completed": true,
}

var dispatchedClientRequests = map[string]bool{
	"initialize": true, "thread/start": true, "thread/resume": true, "thread/fork": true,
	"thread/archive": true, "thread/delete": true, "thread/name/set": true, "thread/metadata/update": true,
	"thread/unarchive": true, "thread/compact/start": true, "thread/list": true, "thread/read": true,
	"skills/list": true, "hooks/list": true, "plugin/list": true, "app/list": true, "turn/start": true,
	"turn/steer": true, "turn/interrupt": true, "review/start": true, "model/list": true,
	"modelProvider/capabilities/read": true, "permissionProfile/list": true, "mcpServerStatus/list": true,
	"getAuthStatus": true,
}

type LocalArtifact struct {
	Path     string
	MimeType string
}

type pendingInteraction struct {
	Method     string
	Params     map[string]any
	AnswerKeys map[string]string
	Response   chan any
}

type CodexAdapter struct {
	profileID   string
	state       *StateStore
	authMu      sync.RWMutex
	authSummary string
	codexHome   string
	workspaceMu sync.RWMutex
	workspaces  map[string]Workspace
	rpc         *RPCClient
	command     *exec.Cmd
	version     string
	onEvent     func(json.RawMessage) error
	done        chan struct{}

	mu      sync.Mutex
	pending map[string]*pendingInteraction
	active  map[string]bool
	closed  bool
}

func ResolveCodex(ctx context.Context, executable string) (string, string, error) {
	if executable == "" || len(executable) > 2048 || strings.ContainsRune(executable, 0) {
		return "", "", errors.New("Codex executable is invalid")
	}
	path, err := exec.LookPath(executable)
	if err != nil {
		return "", "", errors.New("Codex CLI was not found; install the official standalone Codex CLI and ensure it is on PATH")
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return "", "", err
	}
	versionContext, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	versionCommand := exec.CommandContext(versionContext, path, "--version")
	cleanup, err := configureCodexCommand(versionCommand)
	if err != nil {
		return "", "", err
	}
	output, err := versionCommand.CombinedOutput()
	cleanup()
	if err != nil {
		if runtime.GOOS == "windows" && strings.Contains(strings.ToLower(path), `\windowsapps\openai.codex_`) {
			return "", "", errors.New("the Codex Desktop internal executable is not a standalone CLI; install the official Codex CLI")
		}
		return "", "", fmt.Errorf("run Codex CLI: %w", err)
	}
	match := codexVersionPattern.FindStringSubmatch(string(output))
	if len(match) != 2 {
		return "", "", errors.New("Codex CLI version output is invalid")
	}
	return path, match[1], nil
}

func StartCodexAdapter(ctx context.Context, config Config, state *StateStore, stderr io.Writer, onEvent func(json.RawMessage) error) (*CodexAdapter, error) {
	path, version, err := ResolveCodex(ctx, config.CodexExecutable)
	if err != nil {
		return nil, err
	}
	command := exec.Command(path, "app-server")
	stdin, err := command.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, err
	}
	command.Stderr = stderr
	cleanup, err := configureCodexCommand(command)
	if err != nil {
		return nil, err
	}
	if err = command.Start(); err != nil {
		cleanup()
		return nil, fmt.Errorf("start Codex app-server: %w", err)
	}
	cleanup()
	adapter := &CodexAdapter{
		profileID: config.ProfileID, state: state, rpc: NewRPCClient(stdin, stdout), command: command,
		version: version, onEvent: onEvent, pending: make(map[string]*pendingInteraction), active: make(map[string]bool),
		workspaces: make(map[string]Workspace, len(config.Workspaces)), done: make(chan struct{}),
	}
	for _, workspace := range config.Workspaces {
		adapter.workspaces[workspace.WorkspaceID] = workspace
	}
	adapter.rpc.SetHandlers(adapter.notification, adapter.serverRequest)
	go func() {
		_ = command.Wait()
		close(adapter.done)
		adapter.rpc.closeWithError(errors.New("Codex app-server exited"))
	}()
	initializeContext, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var initialized struct {
		CodexHome string `json:"codexHome"`
	}
	if err = adapter.rpc.Request(initializeContext, "initialize", map[string]any{
		"clientInfo":   map[string]any{"name": "deeix-agent", "title": "DEEIX Agent", "version": version},
		"capabilities": map[string]any{"experimentalApi": false, "requestAttestation": false, "mcpServerOpenaiFormElicitation": true},
	}, &initialized); err != nil {
		_ = adapter.Close()
		return nil, fmt.Errorf("initialize Codex app-server: %w", err)
	}
	adapter.codexHome = sanitizeDiagnosticValue(initialized.CodexHome, 1024)
	if err = adapter.rpc.Notify("initialized", nil); err != nil {
		_ = adapter.Close()
		return nil, err
	}
	return adapter, nil
}

func (adapter *CodexAdapter) Manifest() ProviderManifest {
	manifest := ProviderManifest{
		Provider: "codex", RuntimeVersion: adapter.version, ProtocolVersion: adapter.version + "/stable", SchemaHash: codexSchemaHash,
		Commands:         []string{"workspace.register", "thread.create", "thread.lifecycle", "thread.rename", "thread.metadata.update", "thread.compact", "thread.read", "review.start", "turn.start", "turn.steer", "turn.interrupt", "interaction.respond", "resource.refresh"},
		InputKinds:       []string{"text", "artifact", "skill", "app-mention"},
		InteractionKinds: []string{"command_approval", "file_approval", "user_input", "permission", "mcp_elicitation", "dynamic_tool"},
	}
	manifest.Resources.Profile = append([]string(nil), profileResources...)
	manifest.Resources.Workspace = append([]string(nil), workspaceResources...)
	manifest.ThreadSettings.Model = true
	manifest.ThreadSettings.ReasoningEffort = []string{"low", "medium", "high", "xhigh"}
	manifest.ThreadSettings.ApprovalPolicy = []string{"untrusted", "on-request", "never"}
	manifest.ThreadSettings.SandboxPolicy = []string{"read-only", "workspace-write"}
	return manifest
}

func (adapter *CodexAdapter) DiscoverWorkspaces(ctx context.Context) ([]Workspace, error) {
	adapter.workspaceMu.RLock()
	configuredWorkspaces := make([]Workspace, 0, len(adapter.workspaces))
	for _, workspace := range adapter.workspaces {
		configuredWorkspaces = append(configuredWorkspaces, workspace)
	}
	adapter.workspaceMu.RUnlock()
	byID := make(map[string]Workspace, len(configuredWorkspaces))
	for _, configured := range configuredWorkspaces {
		workspace, err := codexProjectWorkspace(configured.Root)
		if err != nil {
			workspace, err = CanonicalWorkspace(configured.Root)
			workspace.SessionRoots = []string{workspace.Root}
		}
		if err != nil {
			continue
		}
		mergeWorkspace(byID, workspace)
	}
	for _, archived := range []bool{false, true} {
		seenCursors := make(map[string]struct{})
		cursor := ""
		for len(byID) < 128 {
			params := map[string]any{
				"limit": 100, "archived": archived, "sortKey": "recency_at",
				"sourceKinds": codexInteractiveSourceKinds,
			}
			if cursor != "" {
				params["cursor"] = cursor
			}
			page, err := adapter.requestMap(ctx, "thread/list", params)
			if err != nil {
				return nil, fmt.Errorf("list Codex threads: %w", err)
			}
			items, ok := page["data"].([]any)
			if !ok {
				return nil, errors.New("Codex thread catalog is invalid")
			}
			for _, raw := range items {
				thread, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				root, _ := thread["cwd"].(string)
				root = strings.TrimSpace(root)
				if root == "" {
					continue
				}
				workspace, workspaceErr := codexProjectWorkspace(root)
				if workspaceErr != nil {
					continue
				}
				mergeWorkspace(byID, workspace)
				if len(byID) == 128 {
					break
				}
			}
			nextCursor, _ := page["nextCursor"].(string)
			nextCursor = strings.TrimSpace(nextCursor)
			if nextCursor == "" {
				break
			}
			if _, duplicate := seenCursors[nextCursor]; duplicate {
				return nil, errors.New("Codex thread catalog cursor repeated")
			}
			seenCursors[nextCursor] = struct{}{}
			cursor = nextCursor
		}
	}
	workspaces := make([]Workspace, 0, len(byID))
	for _, workspace := range byID {
		sort.Strings(workspace.SessionRoots)
		workspaces = append(workspaces, workspace)
	}
	sort.Slice(workspaces, func(left, right int) bool {
		leftName, rightName := strings.ToLower(workspaces[left].Name), strings.ToLower(workspaces[right].Name)
		if leftName == rightName {
			return workspaces[left].WorkspaceID < workspaces[right].WorkspaceID
		}
		return leftName < rightName
	})
	return workspaces, nil
}

func mergeWorkspace(workspaces map[string]Workspace, incoming Workspace) {
	existing, ok := workspaces[incoming.WorkspaceID]
	if !ok {
		workspaces[incoming.WorkspaceID] = incoming
		return
	}
	for _, root := range incoming.SessionRoots {
		if !slices.Contains(existing.SessionRoots, root) {
			existing.SessionRoots = append(existing.SessionRoots, root)
		}
	}
	workspaces[incoming.WorkspaceID] = existing
}

func (adapter *CodexAdapter) replaceWorkspaces(workspaces []Workspace) {
	replacement := make(map[string]Workspace, len(workspaces))
	for _, workspace := range workspaces {
		replacement[workspace.WorkspaceID] = workspace
	}
	adapter.workspaceMu.Lock()
	adapter.workspaces = replacement
	adapter.workspaceMu.Unlock()
}

func (adapter *CodexAdapter) ProveRuntimeAuth(ctx context.Context, challenge string) (string, error) {
	lineCount := strings.Count(challenge, "\n") + 1
	if len(challenge) > 1024 || !(strings.HasPrefix(challenge, "deeix-runtime-auth-proof-v1\n") && lineCount == 7) && !(strings.HasPrefix(challenge, "deeix-device-enrollment-v1\n") && lineCount == 6) {
		return "", errors.New("runtime authentication challenge is invalid")
	}
	var auth struct {
		AuthMethod string `json:"authMethod"`
		AuthToken  string `json:"authToken"`
	}
	if err := adapter.rpc.Request(ctx, "getAuthStatus", map[string]any{"includeToken": true, "refreshToken": false}, &auth); err != nil {
		return "", err
	}
	token := strings.TrimSpace(auth.AuthToken)
	if auth.AuthMethod != "apikey" || token == "" {
		return "", errors.New("Codex must be signed in with an API key")
	}
	digest := sha256.Sum256([]byte(token))
	adapter.authMu.Lock()
	adapter.authSummary = fmt.Sprintf(
		"method=apikey key=%s fingerprint=sha256:%s codexHome=%q",
		maskCredential(token), hex.EncodeToString(digest[:])[:12], adapter.codexHome,
	)
	adapter.authMu.Unlock()
	mac := hmac.New(sha256.New, []byte(token))
	_, _ = mac.Write([]byte(challenge))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func (adapter *CodexAdapter) RuntimeAuthDiagnostic() string {
	adapter.authMu.RLock()
	defer adapter.authMu.RUnlock()
	return adapter.authSummary
}

func maskCredential(value string) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) < 9 {
		return "<redacted>"
	}
	return string(runes[:4]) + "..." + string(runes[len(runes)-4:])
}

func sanitizeDiagnosticValue(value string, limit int) string {
	value = strings.TrimSpace(value)
	clean := make([]rune, 0, len(value))
	for _, character := range value {
		if character >= 32 && character != 127 {
			clean = append(clean, character)
		}
		if len(clean) == limit {
			break
		}
	}
	return string(clean)
}

func sanitizeSessionMessage(value string, limit int) string {
	value = strings.TrimSpace(value)
	clean := make([]rune, 0, len(value))
	for _, character := range value {
		if character >= 32 || character == '\n' || character == '\t' {
			clean = append(clean, character)
		}
		if len(clean) == limit {
			break
		}
	}
	return string(clean)
}

func (adapter *CodexAdapter) Execute(ctx context.Context, command AgentCommand, artifacts map[string]LocalArtifact) (map[string]any, error) {
	if command.ProfileID != adapter.profileID {
		return nil, errors.New("gateway command profile does not match this runtime")
	}
	if command.Kind == "resource.refresh" && command.Resource.Scope == "profile" {
		return adapter.resource(ctx, command, Workspace{})
	}
	adapter.workspaceMu.RLock()
	workspace, ok := adapter.workspaces[command.WorkspaceID]
	adapter.workspaceMu.RUnlock()
	if !ok {
		return nil, errors.New("gateway command workspace is not registered")
	}
	cwd := workspace.Root
	if command.Kind == "thread.create" {
		params := map[string]any{"cwd": cwd, "threadSource": "deeix-web"}
		applyThreadSettings(params, *command.Settings)
		response, err := adapter.requestMap(ctx, "thread/start", params)
		if err != nil {
			return nil, err
		}
		threadID, err := nestedID(response, "thread")
		if err != nil {
			return nil, err
		}
		adapter.setActive(threadID, true)
		sourceRef, err := adapter.state.PublishSource(adapter.profileID, "thread", threadID)
		return map[string]any{"kind": "thread-created", "sourceThreadRef": sourceRef}, err
	}
	if command.Kind == "resource.refresh" {
		return adapter.resource(ctx, command, workspace)
	}
	providerThreadID, err := adapter.state.ResolveSource(adapter.profileID, "thread", command.SourceThreadRef)
	if err != nil {
		return nil, err
	}
	switch command.Kind {
	case "thread.lifecycle":
		return adapter.threadLifecycle(ctx, command, providerThreadID, cwd)
	case "thread.rename":
		err = adapter.rpc.Request(ctx, "thread/name/set", map[string]any{"threadId": providerThreadID, "name": command.Name}, nil)
	case "thread.metadata.update":
		err = adapter.rpc.Request(ctx, "thread/metadata/update", map[string]any{"threadId": providerThreadID, "gitInfo": command.GitInfo}, nil)
	case "thread.compact":
		err = adapter.rpc.Request(ctx, "thread/compact/start", map[string]any{"threadId": providerThreadID}, nil)
	case "thread.read":
		detail, requestErr := adapter.requestMap(ctx, "thread/read", map[string]any{"threadId": providerThreadID, "includeTurns": true})
		if requestErr != nil {
			return nil, requestErr
		}
		session, requestErr := adapter.projectSessionDetail(detail, providerThreadID)
		if requestErr != nil {
			return nil, requestErr
		}
		return map[string]any{"kind": "thread-read", "session": session}, nil
	case "review.start":
		response, requestErr := adapter.requestMap(ctx, "review/start", map[string]any{"threadId": providerThreadID, "target": codexReviewTarget(command.Target), "delivery": "inline"})
		if requestErr != nil {
			return nil, requestErr
		}
		turnID, requestErr := nestedID(response, "turn")
		if requestErr != nil {
			return nil, requestErr
		}
		sourceRef, requestErr := adapter.state.PublishSource(adapter.profileID, "turn", turnID)
		return map[string]any{"kind": "turn-started", "sourceTurnRef": sourceRef}, requestErr
	case "turn.start":
		if !adapter.isActive(providerThreadID) {
			if err = adapter.rpc.Request(ctx, "thread/resume", map[string]any{"threadId": providerThreadID, "cwd": cwd}, nil); err != nil {
				return nil, err
			}
			adapter.setActive(providerThreadID, true)
		}
		input, inputErr := adapter.codexInput(command.Input, artifacts)
		if inputErr != nil {
			return nil, inputErr
		}
		params := map[string]any{"threadId": providerThreadID, "input": input, "cwd": cwd}
		applyTurnSettings(params, *command.Settings, cwd)
		response, requestErr := adapter.requestMap(ctx, "turn/start", params)
		if requestErr != nil {
			return nil, requestErr
		}
		turnID, requestErr := nestedID(response, "turn")
		if requestErr != nil {
			return nil, requestErr
		}
		sourceRef, requestErr := adapter.state.PublishSource(adapter.profileID, "turn", turnID)
		return map[string]any{"kind": "turn-started", "sourceTurnRef": sourceRef}, requestErr
	case "turn.steer":
		providerTurnID, resolveErr := adapter.state.ResolveSource(adapter.profileID, "turn", command.SourceTurnRef)
		if resolveErr != nil {
			return nil, resolveErr
		}
		input, inputErr := adapter.codexInput(command.Input, artifacts)
		if inputErr != nil {
			return nil, inputErr
		}
		err = adapter.rpc.Request(ctx, "turn/steer", map[string]any{"threadId": providerThreadID, "expectedTurnId": providerTurnID, "input": input}, nil)
	case "turn.interrupt":
		providerTurnID, resolveErr := adapter.state.ResolveSource(adapter.profileID, "turn", command.SourceTurnRef)
		if resolveErr != nil {
			return nil, resolveErr
		}
		err = adapter.rpc.Request(ctx, "turn/interrupt", map[string]any{"threadId": providerThreadID, "turnId": providerTurnID}, nil)
	case "interaction.respond":
		err = adapter.respond(command)
	default:
		return nil, fmt.Errorf("unsupported gateway command: %s", command.Kind)
	}
	if err != nil {
		return nil, err
	}
	return map[string]any{"kind": "accepted"}, nil
}

func (adapter *CodexAdapter) threadLifecycle(ctx context.Context, command AgentCommand, threadID, cwd string) (map[string]any, error) {
	if command.Action == "fork" {
		response, err := adapter.requestMap(ctx, "thread/fork", map[string]any{"threadId": threadID, "cwd": cwd, "threadSource": "deeix-web"})
		if err != nil {
			return nil, err
		}
		forkID, err := nestedID(response, "thread")
		if err != nil {
			return nil, err
		}
		adapter.setActive(forkID, true)
		sourceRef, err := adapter.state.PublishSource(adapter.profileID, "thread", forkID)
		return map[string]any{"kind": "thread-forked", "sourceThreadRef": sourceRef}, err
	}
	method := map[string]string{"resume": "thread/resume", "archive": "thread/archive", "unarchive": "thread/unarchive", "delete": "thread/delete"}[command.Action]
	params := map[string]any{"threadId": threadID}
	if command.Action == "resume" {
		params["cwd"] = cwd
	}
	if err := adapter.rpc.Request(ctx, method, params, nil); err != nil {
		return nil, err
	}
	adapter.setActive(threadID, command.Action == "resume" || command.Action == "unarchive")
	return map[string]any{"kind": "accepted"}, nil
}

func (adapter *CodexAdapter) resource(ctx context.Context, command AgentCommand, workspace Workspace) (map[string]any, error) {
	name := command.Resource.Name
	cwd := workspace.Root
	method := map[string]string{
		"models": "model/list", "model-capabilities": "modelProvider/capabilities/read", "permission-profiles": "permissionProfile/list",
		"apps": "app/list", "mcp": "mcpServerStatus/list", "plugins": "plugin/list", "auth-status": "getAuthStatus",
		"sessions": "thread/list", "skills": "skills/list", "hooks": "hooks/list",
	}[name]
	params := map[string]any{}
	switch name {
	case "mcp":
		params["detail"] = "full"
	case "plugins":
		params["forceRefetch"] = true
	case "auth-status":
		params["includeToken"], params["refreshToken"] = false, false
	case "sessions":
		sessionRoots := workspace.SessionRoots
		if len(sessionRoots) == 0 {
			sessionRoots = []string{cwd}
		}
		params["cwd"], params["limit"], params["archived"] = sessionRoots, 100, false
		params["sortKey"], params["sourceKinds"] = "recency_at", codexInteractiveSourceKinds
	case "skills":
		params["cwds"], params["forceReload"] = []string{cwd}, true
	case "hooks":
		params["cwds"] = []string{cwd}
	}
	var data any
	var err error
	if name == "sessions" {
		data, err = adapter.listSessions(ctx, params)
	} else if name == "apps" {
		data, err = adapter.listApps(ctx)
	} else {
		data, err = adapter.requestAny(ctx, method, params)
	}
	if err != nil {
		return nil, err
	}
	if name == "auth-status" {
		source, _ := data.(map[string]any)
		data = map[string]any{"authMethod": source["authMethod"], "requiresOpenaiAuth": source["requiresOpenaiAuth"]}
	} else if name == "sessions" {
		data, err = adapter.projectSessions(ctx, data)
		if err != nil {
			return nil, err
		}
	} else if name == "skills" || name == "apps" {
		data, err = adapter.projectInputResources(name, data)
		if err != nil {
			return nil, err
		}
	} else {
		data = sanitizeResource(data, "")
	}
	return map[string]any{"kind": "resource", "resource": name, "data": data}, nil
}

func (adapter *CodexAdapter) listApps(ctx context.Context) (any, error) {
	items := make([]any, 0, 100)
	cursor := ""
	seenCursors := make(map[string]struct{})
	for len(items) < 500 {
		params := map[string]any{"limit": 100}
		if cursor != "" {
			params["cursor"] = cursor
		}
		page, err := adapter.requestMap(ctx, "app/list", params)
		if err != nil {
			return nil, err
		}
		pageItems, ok := page["data"].([]any)
		if !ok {
			return nil, errors.New("Codex app catalog is invalid")
		}
		remaining := 500 - len(items)
		if len(pageItems) > remaining {
			pageItems = pageItems[:remaining]
		}
		items = append(items, pageItems...)
		nextCursor, _ := page["nextCursor"].(string)
		nextCursor = strings.TrimSpace(nextCursor)
		if nextCursor == "" || len(items) == 500 {
			break
		}
		if _, duplicate := seenCursors[nextCursor]; duplicate {
			return nil, errors.New("Codex app catalog cursor repeated")
		}
		seenCursors[nextCursor] = struct{}{}
		cursor = nextCursor
	}
	return map[string]any{"data": items}, nil
}

func (adapter *CodexAdapter) listSessions(ctx context.Context, params map[string]any) (any, error) {
	const maxSessionsPerStatus = 500
	items := make([]any, 0, 200)
	for _, archived := range []bool{false, true} {
		status := "active"
		if archived {
			status = "archived"
		}
		cursor := ""
		seenCursors := make(map[string]struct{})
		count := 0
		for count < maxSessionsPerStatus {
			request := make(map[string]any, len(params)+1)
			for key, value := range params {
				request[key] = value
			}
			request["archived"] = archived
			if cursor != "" {
				request["cursor"] = cursor
			}
			page, err := adapter.requestMap(ctx, "thread/list", request)
			if err != nil {
				return nil, err
			}
			pageItems, ok := page["data"].([]any)
			if !ok {
				return nil, errors.New("Codex session catalog is invalid")
			}
			for _, raw := range pageItems {
				if count == maxSessionsPerStatus {
					break
				}
				thread, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				thread["status"] = status
				items = append(items, thread)
				count++
			}
			nextCursor, _ := page["nextCursor"].(string)
			nextCursor = strings.TrimSpace(nextCursor)
			if nextCursor == "" || count == maxSessionsPerStatus {
				break
			}
			if _, duplicate := seenCursors[nextCursor]; duplicate {
				return nil, errors.New("Codex session catalog cursor repeated")
			}
			seenCursors[nextCursor] = struct{}{}
			cursor = nextCursor
		}
	}
	return map[string]any{"data": items}, nil
}

func (adapter *CodexAdapter) projectSessions(ctx context.Context, data any) (any, error) {
	root, _ := data.(map[string]any)
	items, _ := root["data"].([]any)
	result := make([]any, 0, len(items))
	for _, raw := range items {
		thread, _ := raw.(map[string]any)
		id, _ := thread["id"].(string)
		if id == "" {
			continue
		}
		sourceRef, err := adapter.state.PublishSource(adapter.profileID, "thread", id)
		if err != nil {
			return nil, err
		}
		result = append(result, map[string]any{
			"sourceThreadRef": sourceRef, "preview": sessionText(thread["preview"], 512), "name": sessionText(thread["name"], 256), "modelProvider": sessionText(thread["modelProvider"], 128),
			"createdAt": thread["createdAt"], "updatedAt": thread["updatedAt"], "recencyAt": thread["recencyAt"], "status": thread["status"], "historyLoaded": false,
		})
	}
	return map[string]any{"data": result}, nil
}

func (adapter *CodexAdapter) projectSessionDetail(detail map[string]any, providerThreadID string) (map[string]any, error) {
	thread, _ := detail["thread"].(map[string]any)
	id, _ := thread["id"].(string)
	if strings.TrimSpace(id) == "" {
		id = providerThreadID
	}
	if id != providerThreadID {
		return nil, errors.New("Codex thread detail does not match the requested thread")
	}
	sourceRef, err := adapter.state.PublishSource(adapter.profileID, "thread", id)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"sourceThreadRef": sourceRef,
		"preview":         sessionText(thread["preview"], 512),
		"name":            sessionText(thread["name"], 256),
		"modelProvider":   sessionText(thread["modelProvider"], 128),
		"createdAt":       thread["createdAt"],
		"updatedAt":       thread["updatedAt"],
		"recencyAt":       thread["recencyAt"],
		"historyLoaded":   true,
		"messages":        projectSessionMessages(detail),
	}, nil
}

func sessionText(value any, limit int) string {
	text, _ := value.(string)
	return sanitizeDiagnosticValue(text, limit)
}

func (adapter *CodexAdapter) notification(notification RPCNotification) error {
	if strings.HasPrefix(notification.Method, "thread/realtime/") || notification.Method == "remoteControl/status/changed" {
		return nil
	}
	params := map[string]any{}
	if len(notification.Params) > 0 && json.Unmarshal(notification.Params, &params) != nil {
		return errors.New("Codex notification payload is invalid")
	}
	threadID := identityValue(params, "threadId", "thread")
	if notification.Method == "thread/started" && threadID != "" {
		adapter.setActive(threadID, true)
	}
	if (notification.Method == "thread/archived" || notification.Method == "thread/deleted" || notification.Method == "thread/closed") && threadID != "" {
		adapter.setActive(threadID, false)
	}
	event := map[string]any{"kind": notificationKind(notification.Method), "occurredAt": time.Now().UTC().Format(time.RFC3339Nano)}
	if source, err := adapter.publishOptional("thread", threadID); err != nil {
		return err
	} else if source != "" {
		event["sourceThreadRef"] = source
	}
	if source, err := adapter.publishOptional("turn", identityValue(params, "turnId", "turn")); err != nil {
		return err
	} else if source != "" {
		event["sourceTurnRef"] = source
	}
	if source, err := adapter.publishOptional("item", identityValue(params, "itemId", "item")); err != nil {
		return err
	} else if source != "" {
		event["sourceItemRef"] = source
	}
	if providerRequestID := notificationRequestID(params["requestId"]); providerRequestID != "" {
		if source, publishErr := adapter.publishOptional("request", providerRequestID); publishErr != nil {
			return publishErr
		} else if source != "" {
			event["sourceRequestRef"] = source
		}
	}
	payload := sanitizeEvent(params, "")
	if event["kind"] == "provider.extension" {
		payload = map[string]any{"method": notification.Method, "data": payload}
	}
	event["payload"] = payload
	encoded, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return adapter.onEvent(encoded)
}

func (adapter *CodexAdapter) serverRequest(ctx context.Context, request RPCServerRequest) (any, error) {
	if !mappedServerRequests[request.Method] {
		return nil, fmt.Errorf("Codex server request is unsupported: %s", request.Method)
	}
	params := map[string]any{}
	if len(request.Params) > 0 && json.Unmarshal(request.Params, &params) != nil {
		return nil, errors.New("Codex server request payload is invalid")
	}
	providerRequestID := rpcID(request.ID)
	sourceRequestRef, err := adapter.state.PublishSource(adapter.profileID, "request", providerRequestID)
	if err != nil {
		return nil, err
	}
	projected, answerKeys, err := projectServerRequest(request.Method, params)
	if err != nil {
		return nil, err
	}
	interaction := &pendingInteraction{Method: request.Method, Params: params, AnswerKeys: answerKeys, Response: make(chan any, 1)}
	adapter.mu.Lock()
	if adapter.closed {
		adapter.mu.Unlock()
		return nil, errors.New("Codex adapter is closed")
	}
	adapter.pending[providerRequestID] = interaction
	adapter.mu.Unlock()
	event := map[string]any{"kind": "interaction.requested", "sourceRequestRef": sourceRequestRef, "occurredAt": time.Now().UTC().Format(time.RFC3339Nano), "payload": map[string]any{"method": request.Method, "request": projected}}
	if source, publishErr := adapter.publishOptional("thread", identityValue(params, "threadId", "thread")); publishErr != nil {
		return nil, publishErr
	} else if source != "" {
		event["sourceThreadRef"] = source
	}
	if source, publishErr := adapter.publishOptional("turn", identityValue(params, "turnId", "turn")); publishErr != nil {
		return nil, publishErr
	} else if source != "" {
		event["sourceTurnRef"] = source
	}
	encoded, _ := json.Marshal(event)
	if err = adapter.onEvent(encoded); err != nil {
		adapter.mu.Lock()
		delete(adapter.pending, providerRequestID)
		adapter.mu.Unlock()
		return nil, err
	}
	select {
	case response := <-interaction.Response:
		return response, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (adapter *CodexAdapter) respond(command AgentCommand) error {
	providerRequestID, err := adapter.state.ResolveSource(adapter.profileID, "request", command.SourceRequestRef)
	if err != nil {
		return err
	}
	adapter.mu.Lock()
	pending := adapter.pending[providerRequestID]
	if pending != nil {
		delete(adapter.pending, providerRequestID)
	}
	adapter.mu.Unlock()
	if pending == nil {
		return errors.New("Codex interaction is no longer pending")
	}
	mapped, err := mapInteractionResponse(pending, command.Response)
	if err != nil {
		return err
	}
	pending.Response <- mapped
	return nil
}

func (adapter *CodexAdapter) Close() error {
	adapter.mu.Lock()
	if adapter.closed {
		adapter.mu.Unlock()
		return nil
	}
	adapter.closed = true
	adapter.pending = make(map[string]*pendingInteraction)
	adapter.mu.Unlock()
	_ = adapter.rpc.Close()
	if adapter.command.Process != nil {
		_ = adapter.command.Process.Kill()
	}
	return nil
}

func (adapter *CodexAdapter) Done() <-chan struct{} { return adapter.done }

func (adapter *CodexAdapter) requestMap(ctx context.Context, method string, params any) (map[string]any, error) {
	var result map[string]any
	if err := adapter.rpc.Request(ctx, method, params, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (adapter *CodexAdapter) requestAny(ctx context.Context, method string, params any) (any, error) {
	var result any
	if err := adapter.rpc.Request(ctx, method, params, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (adapter *CodexAdapter) publishOptional(kind, providerID string) (string, error) {
	if providerID == "" {
		return "", nil
	}
	return adapter.state.PublishSource(adapter.profileID, kind, providerID)
}

func (adapter *CodexAdapter) setActive(threadID string, active bool) {
	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	if active {
		adapter.active[threadID] = true
	} else {
		delete(adapter.active, threadID)
	}
}

func (adapter *CodexAdapter) isActive(threadID string) bool {
	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	return adapter.active[threadID]
}

func applyThreadSettings(params map[string]any, settings Settings) {
	if settings.Model != "" {
		params["model"] = settings.Model
	}
	if settings.ApprovalPolicy != "" {
		params["approvalPolicy"] = settings.ApprovalPolicy
	}
	if settings.SandboxPolicy != "" {
		params["sandbox"] = settings.SandboxPolicy
	}
}

func applyTurnSettings(params map[string]any, settings Settings, cwd string) {
	if settings.Model != "" {
		params["model"] = settings.Model
	}
	if settings.ReasoningEffort != "" {
		params["effort"] = settings.ReasoningEffort
	}
	if settings.ApprovalPolicy != "" {
		params["approvalPolicy"] = settings.ApprovalPolicy
	}
	if settings.SandboxPolicy == "read-only" {
		params["sandboxPolicy"] = map[string]any{"type": "readOnly", "networkAccess": false}
	}
	if settings.SandboxPolicy == "workspace-write" {
		params["sandboxPolicy"] = map[string]any{"type": "workspaceWrite", "writableRoots": []string{cwd}, "networkAccess": false, "excludeTmpdirEnvVar": false, "excludeSlashTmp": false}
	}
}

type inputResourceSource struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

func (adapter *CodexAdapter) projectInputResources(resource string, value any) (map[string]any, error) {
	root, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("Codex %s resource is invalid", resource)
	}
	items, ok := root["data"].([]any)
	if !ok {
		return nil, fmt.Errorf("Codex %s catalog is invalid", resource)
	}
	projected := make([]any, 0, len(items))
	if resource == "skills" {
		for _, groupValue := range items {
			group, _ := groupValue.(map[string]any)
			skills, _ := group["skills"].([]any)
			for _, skillValue := range skills {
				skill, _ := skillValue.(map[string]any)
				name, _ := skill["name"].(string)
				path, _ := skill["path"].(string)
				description, _ := skill["description"].(string)
				if enabled, exists := skill["enabled"].(bool); exists && !enabled {
					continue
				}
				item, err := adapter.projectInputResource("skill", name, path, description)
				if err != nil {
					return nil, err
				}
				if item != nil {
					projected = append(projected, item)
				}
			}
		}
	} else {
		for _, appValue := range items {
			app, _ := appValue.(map[string]any)
			id, _ := app["id"].(string)
			name, _ := app["name"].(string)
			description, _ := app["description"].(string)
			if enabled, exists := app["enabled"].(bool); exists && !enabled {
				continue
			}
			item, err := adapter.projectInputResource("app", name, id, description)
			if err != nil {
				return nil, err
			}
			if item != nil {
				projected = append(projected, item)
			}
		}
	}
	if len(projected) > 500 {
		projected = projected[:500]
	}
	return map[string]any{"data": projected}, nil
}

func (adapter *CodexAdapter) projectInputResource(kind, name, value, description string) (map[string]any, error) {
	name = sanitizeDiagnosticValue(name, 256)
	value = strings.TrimSpace(value)
	description = sanitizeDiagnosticValue(description, 1024)
	if name == "" || value == "" || len(value) > 4096 || kind == "skill" && !filepath.IsAbs(value) || kind == "app" && !codexAppIDPattern.MatchString(value) {
		return nil, nil
	}
	encoded, err := json.Marshal(inputResourceSource{Name: name, Value: value})
	if err != nil {
		return nil, err
	}
	sourceKind, inputKind := kind, kind
	if kind == "app" {
		inputKind = "app-mention"
	}
	resourceRef, err := adapter.state.PublishSource(adapter.profileID, sourceKind, string(encoded))
	if err != nil {
		return nil, err
	}
	return map[string]any{"resourceRef": resourceRef, "kind": inputKind, "name": name, "description": description}, nil
}

func (adapter *CodexAdapter) codexInput(inputs []AgentInput, artifacts map[string]LocalArtifact) ([]any, error) {
	result := make([]any, 0, len(inputs))
	for _, input := range inputs {
		switch input.Kind {
		case "text":
			result = append(result, map[string]any{"type": "text", "text": input.Text, "text_elements": []any{}})
		case "artifact":
			artifact := artifacts[input.ArtifactRef]
			kind := "localImage"
			if strings.HasPrefix(artifact.MimeType, "audio/") {
				kind = "localAudio"
			}
			result = append(result, map[string]any{"type": kind, "path": artifact.Path})
		case "skill", "app-mention":
			sourceKind := "skill"
			if input.Kind == "app-mention" {
				sourceKind = "app"
			}
			raw, err := adapter.state.ResolveSource(adapter.profileID, sourceKind, input.ResourceRef)
			if err != nil {
				return nil, err
			}
			var source inputResourceSource
			if json.Unmarshal([]byte(raw), &source) != nil || strings.TrimSpace(source.Name) == "" || strings.TrimSpace(source.Value) == "" {
				return nil, errors.New("Codex input resource mapping is invalid")
			}
			if input.Kind == "skill" {
				if !filepath.IsAbs(source.Value) {
					return nil, errors.New("Codex skill path is invalid")
				}
				result = append(result, map[string]any{"type": "skill", "name": source.Name, "path": source.Value})
			} else {
				result = append(result, map[string]any{"type": "mention", "name": source.Name, "path": "app://" + source.Value})
			}
		}
	}
	return result, nil
}

func codexReviewTarget(target map[string]any) map[string]any {
	switch target["kind"] {
	case "working-tree":
		return map[string]any{"type": "uncommittedChanges"}
	case "base-branch":
		return map[string]any{"type": "baseBranch", "branch": target["branch"]}
	default:
		return map[string]any{"type": "commit", "sha": target["sha"], "title": nil}
	}
}

func nestedID(value map[string]any, key string) (string, error) {
	nested, _ := value[key].(map[string]any)
	id, _ := nested["id"].(string)
	if id == "" {
		return "", fmt.Errorf("Codex %s response is missing id", key)
	}
	return id, nil
}

func identityValue(params map[string]any, direct, nested string) string {
	if value, ok := params[direct].(string); ok {
		return value
	}
	if value, ok := params[nested].(map[string]any); ok {
		result, _ := value["id"].(string)
		return result
	}
	return ""
}

func rpcID(raw json.RawMessage) string {
	var number float64
	if json.Unmarshal(raw, &number) == nil {
		return fmt.Sprintf("n:%v", number)
	}
	var text string
	_ = json.Unmarshal(raw, &text)
	return "s:" + text
}

func notificationRequestID(value any) string {
	switch typed := value.(type) {
	case string:
		if typed != "" {
			return "s:" + typed
		}
	case float64:
		return fmt.Sprintf("n:%v", typed)
	}
	return ""
}

func notificationKind(method string) string {
	if mappedNotifications[method] {
		return method
	}
	return "provider.extension"
}

func sanitizeEvent(value any, key string) any {
	if sensitiveKey(key) || strings.EqualFold(key, "id") || strings.HasSuffix(key, "Id") {
		return nil
	}
	return sanitizeValue(value, key, true)
}

func sanitizeResource(value any, key string) any {
	if sensitiveKey(key) || pathKey(key) {
		return nil
	}
	return sanitizeValue(value, key, false)
}

func sanitizeValue(value any, key string, event bool) any {
	switch typed := value.(type) {
	case string:
		if pathKey(key) || filepath.IsAbs(typed) || strings.HasPrefix(strings.ToLower(typed), "file:") {
			return nil
		}
		return typed
	case []any:
		result := make([]any, 0, len(typed))
		for _, item := range typed {
			projected := sanitizeValue(item, "", event)
			if projected != nil {
				result = append(result, projected)
			}
		}
		return result
	case map[string]any:
		result := make(map[string]any)
		for name, item := range typed {
			if sensitiveKey(name) || pathKey(name) || event && (strings.EqualFold(name, "id") || strings.HasSuffix(name, "Id")) {
				continue
			}
			projected := sanitizeValue(item, name, event)
			if projected != nil {
				result[name] = projected
			}
		}
		return result
	default:
		return value
	}
}

func sensitiveKey(key string) bool {
	lower := strings.ToLower(key)
	return strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "authorization") || strings.Contains(lower, "password") || strings.Contains(lower, "credential")
}

func pathKey(key string) bool {
	lower := strings.ToLower(key)
	return lower == "cwd" || lower == "path" || lower == "codexhome" || lower == "home" || lower == "command" || lower == "args" || lower == "env" || lower == "environment" || lower == "instructionsources" || lower == "writableroots"
}

func projectServerRequest(method string, params map[string]any) (map[string]any, map[string]string, error) {
	projected, _ := sanitizeEvent(params, "").(map[string]any)
	answerKeys := make(map[string]string)
	if method != "item/tool/requestUserInput" {
		return projected, answerKeys, nil
	}
	questions, _ := params["questions"].([]any)
	projectedQuestions := make([]any, 0, len(questions))
	for _, raw := range questions {
		question, _ := raw.(map[string]any)
		providerID, _ := question["id"].(string)
		if providerID == "" {
			return nil, nil, errors.New("Codex user-input question id is invalid")
		}
		digest := sha256.Sum256([]byte(providerID + time.Now().UTC().String()))
		questionRef := "question_" + fmt.Sprintf("%x", digest[:16])
		answerKeys[questionRef] = providerID
		item, _ := sanitizeEvent(question, "").(map[string]any)
		item["questionRef"] = questionRef
		projectedQuestions = append(projectedQuestions, item)
	}
	projected["questions"] = projectedQuestions
	return projected, answerKeys, nil
}

func mapInteractionResponse(pending *pendingInteraction, raw json.RawMessage) (any, error) {
	var response map[string]any
	if json.Unmarshal(raw, &response) != nil {
		return nil, errors.New("interaction response is invalid")
	}
	kind, _ := response["kind"].(string)
	switch pending.Method {
	case "item/commandExecution/requestApproval", "item/fileChange/requestApproval":
		if kind != "approval" {
			return nil, errors.New("approval response is required")
		}
		return map[string]any{"decision": response["decision"]}, nil
	case "item/tool/requestUserInput":
		if kind != "user-input" {
			return nil, errors.New("user-input response is required")
		}
		answers, _ := response["answers"].(map[string]any)
		mapped := make(map[string]any)
		for questionRef, answer := range answers {
			providerID := pending.AnswerKeys[questionRef]
			if providerID == "" {
				return nil, fmt.Errorf("unknown question reference: %s", questionRef)
			}
			mapped[providerID] = map[string]any{"answers": []any{answer}}
		}
		return map[string]any{"answers": mapped}, nil
	case "mcpServer/elicitation/request":
		if kind != "mcp-elicitation" {
			return nil, errors.New("MCP elicitation response is required")
		}
		content := response["content"]
		if response["decision"] != "accept" {
			content = nil
		}
		return map[string]any{"action": response["decision"], "content": content, "_meta": nil}, nil
	case "item/permissions/requestApproval":
		if kind != "permission" {
			return nil, errors.New("permission response is required")
		}
		if response["decision"] == "decline" {
			return map[string]any{"permissions": map[string]any{}, "scope": "turn"}, nil
		}
		requested, _ := pending.Params["permissions"].(map[string]any)
		permissions := make(map[string]any)
		for _, name := range []string{"network", "fileSystem"} {
			if value, ok := requested[name]; ok {
				permissions[name] = value
			}
		}
		scope := response["scope"]
		if scope == nil {
			scope = "turn"
		}
		return map[string]any{"permissions": permissions, "scope": scope}, nil
	case "item/tool/call":
		if kind != "dynamic-tool" {
			return nil, errors.New("dynamic-tool response is required")
		}
		content, _ := response["content"].([]any)
		items := make([]any, 0, len(content))
		for _, rawItem := range content {
			item, _ := rawItem.(map[string]any)
			typeName := map[string]string{"text": "inputText", "image": "inputImage", "audio": "inputAudio"}[fmt.Sprint(item["kind"])]
			field := "text"
			value := item["text"]
			if typeName == "inputImage" {
				field, value = "imageUrl", item["url"]
			} else if typeName == "inputAudio" {
				field, value = "audioUrl", item["url"]
			}
			items = append(items, map[string]any{"type": typeName, field: value})
		}
		return map[string]any{"success": response["success"], "contentItems": items}, nil
	default:
		return nil, errors.New("Codex server request is unsupported")
	}
}

func projectSessionMessages(detail map[string]any) []any {
	thread, _ := detail["thread"].(map[string]any)
	turns, _ := thread["turns"].([]any)
	messages := make([]any, 0)
	for _, rawTurn := range turns {
		turn, _ := rawTurn.(map[string]any)
		items, _ := turn["items"].([]any)
		reasoningParts := make([]string, 0)
		assistantParts := make([]string, 0)
		for _, rawItem := range items {
			item, _ := rawItem.(map[string]any)
			switch item["type"] {
			case "userMessage":
				parts, _ := item["content"].([]any)
				textParts := make([]string, 0)
				for _, rawPart := range parts {
					part, _ := rawPart.(map[string]any)
					if part["type"] == "text" && strings.TrimSpace(fmt.Sprint(part["text"])) != "" {
						textParts = append(textParts, strings.TrimSpace(fmt.Sprint(part["text"])))
					}
				}
				if len(textParts) > 0 {
					messages = append(messages, map[string]any{"role": "user", "content": sanitizeSessionMessage(strings.Join(textParts, "\n"), 16*1024), "createdAt": turn["startedAt"]})
				}
			case "reasoning":
				parts := append(stringSlice(item["summary"]), stringSlice(item["content"])...)
				reasoningParts = append(reasoningParts, parts...)
			case "agentMessage":
				text := strings.TrimSpace(fmt.Sprint(item["text"]))
				if text != "" {
					assistantParts = append(assistantParts, text)
				}
			}
		}
		if len(assistantParts) > 0 {
			message := map[string]any{
				"role": "assistant", "content": sanitizeSessionMessage(strings.Join(assistantParts, "\n\n"), 16*1024),
				"createdAt": firstValue(turn["completedAt"], turn["startedAt"]),
			}
			if len(reasoningParts) > 0 {
				message["reasoningContent"] = sanitizeSessionMessage(strings.Join(reasoningParts, "\n"), 16*1024)
			}
			messages = append(messages, message)
		}
	}
	start, size := len(messages), 0
	for start > 0 {
		encoded, _ := json.Marshal(messages[start-1])
		if size > 0 && size+len(encoded) > 32*1024 {
			break
		}
		size += len(encoded)
		start--
	}
	return messages[start:]
}

func stringSlice(value any) []string {
	items, _ := value.([]any)
	result := make([]string, 0, len(items))
	for _, item := range items {
		text := strings.TrimSpace(fmt.Sprint(item))
		if text != "" {
			result = append(result, text)
		}
	}
	return result
}

func firstValue(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}
