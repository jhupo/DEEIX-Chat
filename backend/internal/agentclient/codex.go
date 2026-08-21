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
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/mod/semver"
)

const codexSchemaHash = "f72b2caa3cbfa4298de9e85c62dda6dfbaf2266ffeb916fed30615ca69ff8c74"
const minimumCodexVersion = "0.147.0"
const maxCodexDesktopStateBytes = 4 << 20

const codexUpgradeInstructions = "Update the official Codex CLI, then rerun the DEEIX Agent installer. Windows (PowerShell): powershell -ExecutionPolicy ByPass -c \"irm https://chatgpt.com/codex/install.ps1 | iex\"; macOS/Linux: curl -fsSL https://chatgpt.com/codex/install.sh | sh"

var codexVersionPattern = regexp.MustCompile(`(?m)^codex-cli\s+(\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?)\s*$`)
var codexAppIDPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,512}$`)
var codexUserThreadSourceKinds = []string{"cli", "vscode", "exec", "appServer", "unknown"}

const maxSessionMessageRunes = 64 * 1024
const maxExecutionTextBytes = 1 << 20
const maxInteractionPreviewBytes = 8 << 10
const maxApprovalItemProjections = 256

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
	"account/read": true,
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

type approvalItemProjection struct {
	TurnID  string
	Command string
	Files   []any
}

type CodexAdapter struct {
	profileID   string
	state       *StateStore
	authMu      sync.RWMutex
	authSummary string
	codexHome   string
	workspaceMu sync.RWMutex
	workspaces  map[string]Workspace
	threadMu    sync.RWMutex
	threadCWD   map[string]string
	rpc         *RPCClient
	command     *exec.Cmd
	version     string
	onEvent     func(json.RawMessage) error
	done        chan struct{}

	mu            sync.Mutex
	pending       map[string]*pendingInteraction
	active        map[string]bool
	approvalItems map[string]approvalItemProjection
	approvalOrder []string
	closed        bool
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
	version := "v" + match[1]
	if !semver.IsValid(version) {
		return "", "", fmt.Errorf("Codex CLI version %q is invalid", match[1])
	}
	if semver.Compare(version, "v"+minimumCodexVersion) < 0 {
		return "", "", fmt.Errorf(
			"Codex CLI is too old: detected %s; DEEIX requires %s or newer. %s",
			match[1], minimumCodexVersion, codexUpgradeInstructions,
		)
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
		approvalItems: make(map[string]approvalItemProjection),
		workspaces:    make(map[string]Workspace, len(config.Workspaces)), threadCWD: make(map[string]string), done: make(chan struct{}),
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
	go func() {
		select {
		case <-adapter.rpc.Done():
			if command.Process != nil {
				_ = command.Process.Kill()
			}
		case <-adapter.done:
		}
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
	compatibilityContext, cancelCompatibility := context.WithTimeout(ctx, 30*time.Second)
	err = adapter.verifyProjectSessionProtocol(compatibilityContext)
	cancelCompatibility()
	if err != nil {
		_ = adapter.Close()
		return nil, err
	}
	return adapter, nil
}

func (adapter *CodexAdapter) verifyProjectSessionProtocol(ctx context.Context) error {
	codexHome := strings.TrimSpace(adapter.codexHome)
	if codexHome == "" || !filepath.IsAbs(codexHome) || strings.ContainsRune(codexHome, 0) {
		return errors.New("Codex app-server returned an invalid Codex home directory")
	}
	_, err := adapter.listSessions(ctx, map[string]any{
		"limit": 1, "sortKey": "recency_at", "sourceKinds": codexUserThreadSourceKinds, "cwd": []string{codexHome},
	})
	if err == nil {
		return nil
	}
	return fmt.Errorf(
		"Codex CLI project session API is incompatible with DEEIX: detected %s; %v. DEEIX requires %s or newer. %s",
		adapter.version, err, minimumCodexVersion, codexUpgradeInstructions,
	)
}

func (adapter *CodexAdapter) Manifest() ProviderManifest {
	manifest := ProviderManifest{
		Provider: "codex", RuntimeVersion: adapter.version, ProtocolVersion: adapter.version + "/stable", SchemaHash: codexSchemaHash,
		Commands:         []string{"agent.update", "workspace.register", "workspace.rename", "workspace.unregister", "thread.create", "thread.lifecycle", "thread.rename", "thread.metadata.update", "thread.compact", "thread.read", "review.start", "turn.start", "turn.steer", "turn.interrupt", "interaction.respond", "resource.refresh"},
		InputKinds:       []string{"text", "artifact", "skill", "app-mention"},
		InteractionKinds: []string{"command_approval", "file_approval", "user_input", "permission", "mcp_elicitation", "dynamic_tool"},
	}
	manifest.Resources.Profile = append([]string(nil), profileResources...)
	manifest.Resources.Workspace = append([]string(nil), workspaceResources...)
	manifest.ThreadSettings.Model = true
	manifest.ThreadSettings.ReasoningEffort = []string{"low", "medium", "high", "xhigh"}
	manifest.ThreadSettings.ApprovalPolicy = []string{"on-request", "never"}
	manifest.ThreadSettings.ApprovalsReviewer = []string{"user", "auto_review"}
	manifest.ThreadSettings.SandboxPolicy = []string{"workspace-write", "danger-full-access"}
	return manifest
}

func (adapter *CodexAdapter) DiscoverWorkspaces(ctx context.Context) ([]Workspace, error) {
	adapter.workspaceMu.RLock()
	configuredWorkspaces := make([]Workspace, 0, len(adapter.workspaces))
	excludedWorkspaceIDs := make(map[string]struct{})
	for _, workspace := range adapter.workspaces {
		if workspace.Excluded {
			excludedWorkspaceIDs[workspace.WorkspaceID] = struct{}{}
		} else if workspace.Registered {
			configuredWorkspaces = append(configuredWorkspaces, workspace)
		}
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
		workspace.Registered = true
		workspace.Name = configured.Name
		mergeWorkspace(byID, workspace)
	}
	desktopWorkspaces := adapter.desktopWorkspaces()
	for _, workspace := range desktopWorkspaces {
		if len(byID) == 128 {
			break
		}
		if _, excluded := excludedWorkspaceIDs[workspace.WorkspaceID]; excluded {
			continue
		}
		mergeWorkspace(byID, workspace)
	}
	if len(desktopWorkspaces) > 0 {
		adapter.mergeUnassignedRecentThreads(ctx, byID, excludedWorkspaceIDs)
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

func (adapter *CodexAdapter) mergeUnassignedRecentThreads(ctx context.Context, workspaces map[string]Workspace, excluded map[string]struct{}) {
	if ctx == nil || adapter.rpc == nil || len(workspaces) >= 128 {
		return
	}
	result, err := adapter.listSessions(ctx, map[string]any{
		"limit": 100, "archived": false, "sortKey": "recency_at", "sourceKinds": codexUserThreadSourceKinds,
	})
	if err != nil {
		return
	}
	root, _ := result.(map[string]any)
	rawItems, _ := root["data"].([]any)
	threadIDs := make(map[string]struct{})
	for _, raw := range rawItems {
		thread, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		threadID, _ := thread["id"].(string)
		threadID = strings.TrimSpace(threadID)
		status, _ := thread["status"].(string)
		if threadID == "" || status != "active" {
			continue
		}
		if cwd, _ := thread["cwd"].(string); strings.TrimSpace(cwd) == "" {
			continue
		}
		assigned := false
		for _, workspace := range workspaces {
			if workspace.Hidden {
				if _, exists := workspace.ThreadIDs[threadID]; exists {
					assigned = true
					break
				}
				continue
			}
			if _, exists := workspace.ThreadIDs[threadID]; exists || sessionCWDWithinRoots(thread, workspace.SessionRoots) {
				assigned = true
				break
			}
		}
		if !assigned {
			threadIDs[threadID] = struct{}{}
		}
	}
	if len(threadIDs) == 0 {
		return
	}
	var anchor string
	for _, workspace := range workspaces {
		if workspace.Hidden && strings.TrimSpace(workspace.Root) != "" {
			anchor = workspace.Root
			break
		}
	}
	if anchor == "" {
		codexHome := strings.TrimSpace(adapter.codexHome)
		if workspace, resolveErr := canonicalWorkspaceAsConfiguredUser(codexHome); resolveErr == nil {
			anchor = workspace.Root
		}
	}
	if anchor == "" {
		return
	}
	sum := sha256.Sum256([]byte("deeix:recent:" + anchor))
	recentID := "workspace-recent-" + hex.EncodeToString(sum[:12])
	if _, blocked := excluded[recentID]; blocked {
		return
	}
	recent, exists := workspaces[recentID]
	if !exists {
		recent = Workspace{WorkspaceID: recentID, Root: anchor, Name: "Recent", Hidden: true, SessionRoots: []string{anchor}, ThreadIDs: make(map[string]struct{})}
	}
	if recent.ThreadIDs == nil {
		recent.ThreadIDs = make(map[string]struct{})
	}
	for threadID := range threadIDs {
		recent.ThreadIDs[threadID] = struct{}{}
	}
	workspaces[recentID] = recent
}

func (adapter *CodexAdapter) desktopWorkspaces() []Workspace {
	codexHome := strings.TrimSpace(adapter.codexHome)
	if codexHome == "" || len(codexHome) > 4096 || !filepath.IsAbs(codexHome) || strings.ContainsRune(codexHome, 0) {
		return nil
	}
	statePath := filepath.Join(codexHome, ".codex-global-state.json")
	var data []byte
	if err := runAsConfiguredUser(func() error {
		file, err := os.Open(statePath)
		if err != nil {
			return err
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxCodexDesktopStateBytes {
			return errors.New("Codex Desktop state file is invalid")
		}
		data, err = io.ReadAll(io.LimitReader(file, maxCodexDesktopStateBytes+1))
		if err != nil || len(data) > maxCodexDesktopStateBytes {
			return errors.New("Codex Desktop state file is invalid")
		}
		return nil
	}); err != nil {
		return nil
	}
	var state struct {
		Projects map[string]struct {
			Name      string   `json:"name"`
			RootPaths []string `json:"rootPaths"`
		} `json:"local-projects"`
		ProjectlessIDs []string `json:"projectless-thread-ids"`
		Assignments    map[string]struct {
			ProjectID string `json:"projectId"`
		} `json:"thread-project-assignments"`
		WorkspaceHints map[string]string `json:"thread-workspace-root-hints"`
	}
	if json.Unmarshal(data, &state) != nil {
		return nil
	}
	workspaces := make([]Workspace, 0, min(len(state.Projects)+1, 128))
	seen := make(map[string]struct{}, len(state.Projects))
	projectIDs := make(map[string]string, len(state.Projects))
	for projectID, project := range state.Projects {
		projectID, project.Name = strings.TrimSpace(projectID), sanitizeDiagnosticValue(project.Name, 128)
		if projectID == "" || project.Name == "" {
			continue
		}
		var selected *Workspace
		for _, root := range project.RootPaths {
			root = strings.TrimSpace(root)
			if root == "" || len(root) > 4096 || !filepath.IsAbs(root) || strings.ContainsRune(root, 0) {
				continue
			}
			workspace, workspaceErr := canonicalWorkspaceAsConfiguredUser(strings.TrimSpace(root))
			if workspaceErr != nil {
				continue
			}
			workspace.Name = project.Name
			workspace.SessionRoots = []string{workspace.Root}
			if selected == nil {
				selected = &workspace
			} else {
				selected.SessionRoots = append(selected.SessionRoots, workspace.Root)
			}
		}
		if selected == nil {
			continue
		}
		if _, duplicate := seen[selected.WorkspaceID]; duplicate {
			continue
		}
		seen[selected.WorkspaceID] = struct{}{}
		projectIDs[projectID] = selected.WorkspaceID
		workspaces = append(workspaces, *selected)
	}
	projectless := make(map[string]struct{}, len(state.ProjectlessIDs))
	for _, id := range state.ProjectlessIDs {
		if id = strings.TrimSpace(id); id != "" {
			projectless[id] = struct{}{}
		}
	}
	var recentRoot string
	for threadID, root := range state.WorkspaceHints {
		if _, ok := projectless[threadID]; !ok {
			continue
		}
		root = strings.TrimSpace(root)
		if root == "" || len(root) > 4096 || !filepath.IsAbs(root) || strings.ContainsRune(root, 0) {
			continue
		}
		if workspace, workspaceErr := canonicalWorkspaceAsConfiguredUser(root); workspaceErr == nil {
			recentRoot = workspace.Root
			break
		}
	}
	// Some Desktop versions persist projectless IDs without root hints. The
	// hidden Recent workspace still needs an internal anchor so it can be
	// synchronized; each thread keeps its real cwd from thread/list and uses
	// that cwd when resumed. The Codex home is never exposed as a project.
	if recentRoot == "" && len(projectless) > 0 {
		if workspace, workspaceErr := canonicalWorkspaceAsConfiguredUser(codexHome); workspaceErr == nil {
			recentRoot = workspace.Root
		}
	}
	if recentRoot != "" && len(projectless) > 0 {
		sum := sha256.Sum256([]byte("deeix:recent:" + recentRoot))
		workspaces = append(workspaces, Workspace{WorkspaceID: "workspace-recent-" + hex.EncodeToString(sum[:12]), Root: recentRoot, Name: "Recent", Hidden: true, SessionRoots: []string{recentRoot}, ThreadIDs: projectless})
	}
	for threadID, assignment := range state.Assignments {
		workspaceID, ok := projectIDs[strings.TrimSpace(assignment.ProjectID)]
		if !ok || strings.TrimSpace(threadID) == "" {
			continue
		}
		for index := range workspaces {
			if workspaces[index].WorkspaceID == workspaceID {
				if workspaces[index].ThreadIDs == nil {
					workspaces[index].ThreadIDs = make(map[string]struct{})
				}
				workspaces[index].ThreadIDs[threadID] = struct{}{}
			}
		}
	}
	return workspaces
}

func canonicalWorkspaceAsConfiguredUser(root string) (workspace Workspace, err error) {
	err = runAsConfiguredUser(func() error {
		workspace, err = CanonicalWorkspace(root)
		return err
	})
	return workspace, err
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
	existing.Registered = existing.Registered || incoming.Registered
	existing.Excluded = existing.Excluded || incoming.Excluded
	existing.Hidden = existing.Hidden || incoming.Hidden
	if !existing.Registered && !incoming.Registered && !incoming.Hidden && strings.TrimSpace(incoming.Name) != "" {
		existing.Name = incoming.Name
	}
	if existing.ThreadIDs == nil && incoming.ThreadIDs != nil {
		existing.ThreadIDs = make(map[string]struct{}, len(incoming.ThreadIDs))
	}
	for threadID := range incoming.ThreadIDs {
		existing.ThreadIDs[threadID] = struct{}{}
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
	var status struct {
		Account *struct {
			Type string `json:"type"`
		} `json:"account"`
	}
	if err := adapter.rpc.Request(ctx, "account/read", map[string]any{"refreshToken": false}, &status); err != nil {
		return "", err
	}
	if status.Account == nil || status.Account.Type != "apiKey" {
		return "", errors.New("Codex must be signed in with an API key")
	}
	token, err := readCodexAPIKey(adapter.codexHome)
	if err != nil {
		return "", err
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

func readCodexAPIKey(codexHome string) (string, error) {
	codexHome = strings.TrimSpace(codexHome)
	if !filepath.IsAbs(codexHome) || strings.ContainsRune(codexHome, 0) {
		return "", errors.New("Codex home is invalid")
	}
	const maxAuthFileBytes = 64 << 10
	var data []byte
	err := runAsConfiguredUser(func() error {
		file, err := os.Open(filepath.Join(codexHome, "auth.json"))
		if err != nil {
			return err
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxAuthFileBytes {
			return errors.New("Codex auth file is invalid")
		}
		data, err = io.ReadAll(io.LimitReader(file, maxAuthFileBytes+1))
		if err != nil {
			return err
		}
		if len(data) > maxAuthFileBytes {
			return errors.New("Codex auth file is too large")
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("read Codex auth file: %w", err)
	}
	var auth struct {
		APIKey string `json:"OPENAI_API_KEY"`
	}
	if json.Unmarshal(data, &auth) != nil {
		return "", errors.New("Codex auth file is invalid")
	}
	token := strings.TrimSpace(auth.APIKey)
	if token == "" || len(token) > 4096 || strings.ContainsAny(token, "\r\n\x00") {
		return "", errors.New("Codex auth file does not contain a valid API key")
	}
	return token, nil
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
	if !ok || workspace.Excluded {
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
	adapter.threadMu.RLock()
	if threadCWD := strings.TrimSpace(adapter.threadCWD[providerThreadID]); threadCWD != "" {
		cwd = threadCWD
	}
	adapter.threadMu.RUnlock()
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
		"apps": "app/list", "mcp": "mcpServerStatus/list", "plugins": "plugin/list", "auth-status": "account/read",
		"sessions": "thread/list", "skills": "skills/list", "hooks": "hooks/list",
	}[name]
	params := map[string]any{}
	switch name {
	case "mcp":
		params["detail"] = "full"
	case "plugins":
		params["forceRefetch"] = true
	case "auth-status":
		params["refreshToken"] = false
	case "sessions":
		if !workspace.Hidden && len(workspace.ThreadIDs) == 0 {
			sessionRoots := workspace.SessionRoots
			if len(sessionRoots) == 0 {
				sessionRoots = []string{cwd}
			}
			params["cwd"] = sessionRoots
		}
		params["limit"], params["archived"] = 100, false
		params["sortKey"] = "recency_at"
		params["sourceKinds"] = codexUserThreadSourceKinds
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
		account, _ := source["account"].(map[string]any)
		authMethod, _ := account["type"].(string)
		if authMethod == "apiKey" {
			authMethod = "apikey"
		}
		data = map[string]any{"authMethod": authMethod, "requiresOpenaiAuth": source["requiresOpenaiAuth"]}
	} else if name == "sessions" {
		data, err = adapter.projectSessions(data, workspace)
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
				if id, _ := thread["id"].(string); strings.TrimSpace(id) != "" {
					if cwd, _ := thread["cwd"].(string); strings.TrimSpace(cwd) != "" {
						adapter.threadMu.Lock()
						adapter.threadCWD[id] = cwd
						adapter.threadMu.Unlock()
					}
				}
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

func (adapter *CodexAdapter) projectSessions(data any, workspace Workspace) (any, error) {
	root, _ := data.(map[string]any)
	items, _ := root["data"].([]any)
	result := make([]any, 0, len(items))
	for _, raw := range items {
		thread, _ := raw.(map[string]any)
		id, _ := thread["id"].(string)
		if id == "" {
			continue
		}
		if workspace.Hidden {
			if _, ok := workspace.ThreadIDs[id]; !ok {
				continue
			}
		} else if len(workspace.ThreadIDs) > 0 {
			if _, assigned := workspace.ThreadIDs[id]; !assigned && !sessionCWDWithinRoots(thread, workspace.SessionRoots) {
				continue
			}
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

func sessionCWDWithinRoots(thread map[string]any, roots []string) bool {
	cwd, _ := thread["cwd"].(string)
	cwd = strings.TrimSpace(cwd)
	if cwd == "" || len(cwd) > 4096 || !filepath.IsAbs(cwd) || strings.ContainsRune(cwd, 0) {
		return false
	}
	for _, root := range roots {
		if pathWithin(root, cwd) {
			return true
		}
	}
	return false
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
	turnID := identityValue(params, "turnId", "turn")
	itemID := identityValue(params, "itemId", "item")
	if notification.Method == "item/started" {
		item, _ := params["item"].(map[string]any)
		adapter.rememberApprovalItem(itemID, turnID, item)
	}
	if notification.Method == "item/completed" && itemID != "" {
		defer adapter.forgetApprovalItem(itemID)
	}
	if notification.Method == "turn/completed" && turnID != "" {
		defer adapter.forgetTurnApprovalItems(turnID)
	}
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
	if source, err := adapter.publishOptional("turn", turnID); err != nil {
		return err
	} else if source != "" {
		event["sourceTurnRef"] = source
	}
	sourceItemRef := ""
	if source, err := adapter.publishOptional("item", itemID); err != nil {
		return err
	} else if source != "" {
		sourceItemRef = source
		event["sourceItemRef"] = source
	}
	if providerRequestID := notificationRequestID(params["requestId"]); providerRequestID != "" {
		if source, publishErr := adapter.publishOptional("request", providerRequestID); publishErr != nil {
			return publishErr
		} else if source != "" {
			event["sourceRequestRef"] = source
		}
	}
	payload := adapter.projectNotification(notification.Method, params, sourceItemRef)
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
	projected, answerKeys, err := adapter.projectServerRequest(request.Method, params)
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
	adapter.approvalItems = make(map[string]approvalItemProjection)
	adapter.approvalOrder = nil
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

func (adapter *CodexAdapter) rememberApprovalItem(itemID, turnID string, item map[string]any) {
	if itemID == "" || len(itemID) > 4096 || len(turnID) > 4096 || item == nil {
		return
	}
	kind := stringField(item, "type")
	if kind == "" {
		kind = stringField(item, "kind")
	}
	projection := approvalItemProjection{TurnID: turnID}
	switch kind {
	case "commandExecution":
		projection.Command = interactionText(item["command"])
	case "fileChange":
		projection.Files = adapter.projectInteractionChanges(item["changes"])
	default:
		return
	}
	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	if adapter.approvalItems == nil {
		adapter.approvalItems = make(map[string]approvalItemProjection)
	}
	if _, exists := adapter.approvalItems[itemID]; exists {
		adapter.approvalItems[itemID] = projection
		return
	}
	if len(adapter.approvalItems) == maxApprovalItemProjections {
		oldest := adapter.approvalOrder[0]
		adapter.approvalOrder = adapter.approvalOrder[1:]
		delete(adapter.approvalItems, oldest)
	}
	adapter.approvalItems[itemID] = projection
	adapter.approvalOrder = append(adapter.approvalOrder, itemID)
}

func (adapter *CodexAdapter) approvalItem(itemID string) approvalItemProjection {
	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	projection := adapter.approvalItems[itemID]
	projection.Files = append([]any(nil), projection.Files...)
	return projection
}

func (adapter *CodexAdapter) forgetApprovalItem(itemID string) {
	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	if _, exists := adapter.approvalItems[itemID]; !exists {
		return
	}
	delete(adapter.approvalItems, itemID)
	for index, cachedItemID := range adapter.approvalOrder {
		if cachedItemID == itemID {
			adapter.approvalOrder = slices.Delete(adapter.approvalOrder, index, index+1)
			return
		}
	}
}

func (adapter *CodexAdapter) forgetTurnApprovalItems(turnID string) {
	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	kept := adapter.approvalOrder[:0]
	for _, itemID := range adapter.approvalOrder {
		if adapter.approvalItems[itemID].TurnID == turnID {
			delete(adapter.approvalItems, itemID)
			continue
		}
		kept = append(kept, itemID)
	}
	adapter.approvalOrder = kept
}

func applyThreadSettings(params map[string]any, settings Settings) {
	if settings.Model != "" {
		params["model"] = settings.Model
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
	if settings.ApprovalsReviewer != "" {
		params["approvalsReviewer"] = settings.ApprovalsReviewer
	}
	if settings.SandboxPolicy == "workspace-write" {
		params["sandboxPolicy"] = map[string]any{"type": "workspaceWrite", "writableRoots": []string{cwd}, "networkAccess": false, "excludeTmpdirEnvVar": false, "excludeSlashTmp": false}
	}
	if settings.SandboxPolicy == "danger-full-access" {
		params["sandboxPolicy"] = map[string]any{"type": "dangerFullAccess"}
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

func (adapter *CodexAdapter) projectNotification(method string, params map[string]any, sourceItemRef string) any {
	switch method {
	case "turn/started", "turn/completed":
		return projectExecutionTurn(params)
	case "item/started", "item/completed":
		item, _ := params["item"].(map[string]any)
		return map[string]any{"itemID": sourceItemRef, "item": adapter.projectExecutionItem(item, sourceItemRef)}
	case "item/agentMessage/delta", "item/plan/delta":
		delta, truncated := boundedText(stringField(params, "delta"), maxExecutionTextBytes)
		return map[string]any{"itemID": sourceItemRef, "delta": delta, "truncated": truncated}
	case "item/reasoning/summaryTextDelta", "item/reasoning/textDelta":
		delta, truncated := boundedText(stringField(params, "delta"), maxExecutionTextBytes)
		result := map[string]any{"itemID": sourceItemRef, "delta": delta, "truncated": truncated}
		for _, key := range []string{"summaryIndex", "contentIndex"} {
			if number, ok := finiteNumber(params[key]); ok {
				result[key] = number
			}
		}
		return result
	case "item/reasoning/summaryPartAdded":
		result := map[string]any{"itemID": sourceItemRef}
		if number, ok := finiteNumber(params["summaryIndex"]); ok {
			result["summaryIndex"] = number
		}
		return result
	case "item/commandExecution/outputDelta":
		output, truncated := boundedText(stringField(params, "delta"), maxExecutionTextBytes)
		return map[string]any{"itemID": sourceItemRef, "outputDelta": output, "truncated": truncated}
	case "item/fileChange/patchUpdated":
		result := map[string]any{"itemID": sourceItemRef, "changes": adapter.projectExecutionChanges(params["changes"])}
		if patch, ok := params["patch"].(string); ok {
			result["patch"], result["truncated"] = boundedText(patch, maxExecutionTextBytes)
		}
		return result
	case "turn/diff/updated":
		diff, truncated := boundedText(stringField(params, "diff"), maxExecutionTextBytes)
		return map[string]any{"diff": diff, "truncated": truncated}
	case "turn/plan/updated":
		return projectTurnPlan(params)
	case "thread/tokenUsage/updated":
		return map[string]any{"tokenUsage": projectTokenUsage(params["tokenUsage"])}
	case "model/rerouted":
		return map[string]any{
			"fromModel": stringField(params, "fromModel"),
			"toModel":   stringField(params, "toModel"),
			"reason":    stringField(params, "reason"),
		}
	case "serverRequest/resolved":
		return map[string]any{}
	default:
		return sanitizeEvent(params, "")
	}
}

func projectExecutionTurn(params map[string]any) map[string]any {
	turn, _ := params["turn"].(map[string]any)
	resultTurn := map[string]any{}
	if status := stringField(turn, "status"); status != "" {
		resultTurn["status"] = status
	}
	if duration, ok := finiteNumber(turn["durationMs"]); ok {
		resultTurn["durationMs"] = duration
	}
	if rawError, ok := turn["error"].(map[string]any); ok {
		projected := map[string]any{}
		for _, key := range []string{"code", "message"} {
			if text := interactionText(rawError[key]); text != "" {
				projected[key] = text
			}
		}
		if len(projected) > 0 {
			resultTurn["error"] = projected
		}
	}
	return map[string]any{"turn": resultTurn}
}

func (adapter *CodexAdapter) projectExecutionItem(item map[string]any, sourceItemRef string) map[string]any {
	result := map[string]any{"itemID": sourceItemRef}
	kind := stringField(item, "type")
	if kind == "" {
		kind = stringField(item, "kind")
	}
	if kind != "" {
		result["kind"] = kind
	}
	if status := stringField(item, "status"); status != "" {
		result["status"] = status
	}
	if command, ok := item["command"].(string); ok {
		result["command"] = command
	}
	if output, ok := item["aggregatedOutput"].(string); ok {
		result["output"], result["truncated"] = boundedText(output, maxExecutionTextBytes)
	} else if output, ok := item["output"].(string); ok {
		result["output"], result["truncated"] = boundedText(output, maxExecutionTextBytes)
	}
	if exitCode, ok := finiteNumber(item["exitCode"]); ok {
		result["exitCode"] = exitCode
	}
	if changes, exists := item["changes"]; exists {
		result["changes"] = adapter.projectExecutionChanges(changes)
	}
	if diff, ok := item["diff"].(string); ok {
		projected, truncated := boundedText(diff, maxExecutionTextBytes)
		result["diff"] = projected
		if truncated {
			result["truncated"] = true
		}
	}
	return result
}

func (adapter *CodexAdapter) projectExecutionChanges(value any) []any {
	items, _ := value.([]any)
	result := make([]any, 0, len(items))
	for _, raw := range items {
		change, _ := raw.(map[string]any)
		path := adapter.projectWorkspacePath(stringField(change, "path"))
		if path == "" {
			continue
		}
		projected := map[string]any{"path": path, "change": changeKind(change["kind"])}
		if previous := adapter.projectWorkspacePath(stringField(change, "previousPath")); previous != "" {
			projected["previousPath"] = previous
		}
		if kind, ok := change["kind"].(map[string]any); ok {
			if previous := adapter.projectWorkspacePath(stringField(kind, "move_path")); previous != "" {
				projected["previousPath"] = previous
			}
		}
		if diff, ok := change["diff"].(string); ok {
			projected["diff"], projected["truncated"] = boundedText(diff, maxExecutionTextBytes)
		}
		result = append(result, projected)
	}
	return result
}

func (adapter *CodexAdapter) projectWorkspacePath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsRune(value, 0) {
		return ""
	}
	if !filepath.IsAbs(value) {
		clean := filepath.Clean(value)
		if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || filepath.IsAbs(clean) {
			return ""
		}
		return filepath.ToSlash(clean)
	}
	adapter.workspaceMu.RLock()
	defer adapter.workspaceMu.RUnlock()
	best := ""
	bestRootLength := -1
	for _, workspace := range adapter.workspaces {
		roots := append([]string{workspace.Root}, workspace.SessionRoots...)
		for _, root := range roots {
			if root == "" || !pathWithin(root, value) || len(root) <= bestRootLength {
				continue
			}
			relative, err := filepath.Rel(root, value)
			if err == nil && relative != "." {
				best = filepath.ToSlash(relative)
				bestRootLength = len(root)
			}
		}
	}
	if best != "" {
		return best
	}
	return filepath.Base(value)
}

func projectTurnPlan(params map[string]any) map[string]any {
	result := map[string]any{"plan": []any{}}
	if explanation, ok := params["explanation"].(string); ok {
		result["explanation"], _ = boundedText(explanation, maxInteractionPreviewBytes)
	}
	items, _ := params["plan"].([]any)
	plan := make([]any, 0, len(items))
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		step, status := stringField(item, "step"), stringField(item, "status")
		if step == "" || !containsString([]string{"pending", "inProgress", "completed"}, status) {
			continue
		}
		step, _ = boundedText(step, maxInteractionPreviewBytes)
		plan = append(plan, map[string]any{"step": step, "status": status})
	}
	result["plan"] = plan
	return result
}

func projectTokenUsage(value any) map[string]any {
	source, _ := value.(map[string]any)
	result := make(map[string]any)
	for _, section := range []string{"total", "last"} {
		breakdown, _ := source[section].(map[string]any)
		projected := make(map[string]any)
		for _, pair := range [][2]string{{"inputTokens", "inputTokens"}, {"cachedInputTokens", "cachedInputTokens"}, {"outputTokens", "outputTokens"}, {"reasoningOutputTokens", "reasoningTokens"}, {"totalTokens", "totalTokens"}} {
			if number, ok := finiteNumber(breakdown[pair[0]]); ok {
				projected[pair[1]] = number
			}
		}
		result[section] = projected
	}
	if number, ok := finiteNumber(source["modelContextWindow"]); ok {
		result["modelContextWindow"] = number
	}
	return result
}

func finiteNumber(value any) (any, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, !isInvalidFloat(typed)
	case float32:
		return typed, !isInvalidFloat(float64(typed))
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, json.Number:
		return value, true
	default:
		return nil, false
	}
}

func isInvalidFloat(value float64) bool {
	return value != value || value > 1.7976931348623157e+308 || value < -1.7976931348623157e+308
}

func stringField(source map[string]any, key string) string {
	value, _ := source[key].(string)
	return value
}

func changeKind(value any) string {
	if text, ok := value.(string); ok && text != "" {
		return text
	}
	if object, ok := value.(map[string]any); ok {
		if text := stringField(object, "type"); text != "" {
			return text
		}
	}
	return "update"
}

func boundedText(value string, limit int) (string, bool) {
	if len(value) <= limit {
		return value, false
	}
	end := limit
	for end > 0 && !utf8.ValidString(value[:end]) {
		end--
	}
	return value[:end], true
}

func sanitizeEvent(value any, key string) any {
	if strings.EqualFold(key, "tokenUsage") {
		return sanitizeTokenUsage(value)
	}
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
			if strings.EqualFold(name, "tokenUsage") {
				if projected := sanitizeTokenUsage(item); projected != nil {
					result[name] = projected
				}
				continue
			}
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

func sanitizeTokenUsage(value any) any {
	allowed := map[string]bool{
		"total": true, "last": true, "inputTokens": true, "cachedInputTokens": true,
		"outputTokens": true, "reasoningOutputTokens": true, "totalTokens": true,
		"modelContextWindow": true,
	}
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any)
		for key, item := range typed {
			if !allowed[key] {
				continue
			}
			if projected := sanitizeTokenUsage(item); projected != nil {
				result[key] = projected
			}
		}
		return result
	case float64, int, int32, int64, uint, uint32, uint64, json.Number:
		return value
	default:
		return nil
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

func (adapter *CodexAdapter) projectServerRequest(method string, params map[string]any) (map[string]any, map[string]string, error) {
	answerKeys := make(map[string]string)
	switch method {
	case "item/commandExecution/requestApproval":
		request := make(map[string]any)
		command := interactionText(params["command"])
		if command == "" {
			command = adapter.approvalItem(identityValue(params, "itemId", "item")).Command
		}
		if command != "" {
			request["command"] = command
		}
		if reason := interactionText(params["reason"]); reason != "" {
			request["reason"] = reason
		}
		return request, answerKeys, nil
	case "item/fileChange/requestApproval":
		request := make(map[string]any)
		if reason := interactionText(params["reason"]); reason != "" {
			request["reason"] = reason
		}
		files := adapter.approvalItem(identityValue(params, "itemId", "item")).Files
		if root, ok := params["grantRoot"].(string); ok && strings.TrimSpace(root) != "" {
			if path := adapter.projectWorkspacePath(root); path != "" {
				files = append(files, map[string]any{"path": path, "change": "write"})
			}
		}
		request["files"] = files
		return request, answerKeys, nil
	case "item/permissions/requestApproval":
		request := map[string]any{
			"permissions":   permissionNames(params["permissions"]),
			"allowedScopes": []any{"turn", "session"},
		}
		if reason := interactionText(params["reason"]); reason != "" {
			request["description"] = reason
		}
		return request, answerKeys, nil
	case "mcpServer/elicitation/request":
		request := make(map[string]any)
		if server := interactionText(firstInteractionValue(params, "serverName", "server")); server != "" {
			request["serverName"] = server
		}
		if message := interactionText(firstInteractionValue(params, "message", "prompt")); message != "" {
			request["message"] = message
		}
		if schema := firstInteractionValue(params, "requestedSchema", "schema"); schema != nil {
			if projected := projectElicitationSchema(schema); projected != nil {
				request["requestedSchema"] = projected
			}
		}
		return request, answerKeys, nil
	case "item/tool/call":
		tool := interactionText(firstInteractionValue(params, "tool", "name"))
		name := tool
		if namespace := interactionText(params["namespace"]); namespace != "" && tool != "" {
			name = namespace + "/" + tool
		}
		request := map[string]any{
			"tool":                 tool,
			"name":                 name,
			"acceptedContentKinds": []any{"text", "image"},
		}
		if preview := interactionArgumentsPreview(firstInteractionValue(params, "arguments", "input")); preview != "" {
			request["argumentsPreview"] = preview
		}
		return request, answerKeys, nil
	case "item/tool/requestUserInput":
		// handled below because provider question IDs must be replaced with opaque refs
	default:
		return nil, nil, fmt.Errorf("Codex server request is unsupported: %s", method)
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
		item := projectUserInputQuestion(question)
		item["questionRef"] = questionRef
		projectedQuestions = append(projectedQuestions, item)
	}
	projected := map[string]any{"questions": projectedQuestions}
	return projected, answerKeys, nil
}

func projectUserInputQuestion(question map[string]any) map[string]any {
	result := map[string]any{"required": true}
	for _, key := range []string{"header", "question"} {
		if value := interactionText(question[key]); value != "" {
			result[key] = value
		}
	}
	if value, ok := question["isOther"].(bool); ok {
		result["allowFreeform"] = value
	}
	if value, ok := question["isSecret"].(bool); ok {
		result["secret"] = value
	}
	options, _ := question["options"].([]any)
	projectedOptions := make([]any, 0, min(len(options), 64))
	for _, raw := range options {
		if len(projectedOptions) == 64 {
			break
		}
		option, _ := raw.(map[string]any)
		label := interactionText(option["label"])
		if label == "" {
			continue
		}
		projected := map[string]any{"label": label}
		if description := interactionText(option["description"]); description != "" {
			projected["description"] = description
		}
		projectedOptions = append(projectedOptions, projected)
	}
	if len(projectedOptions) > 0 {
		result["options"] = projectedOptions
	}
	return result
}

func projectElicitationSchema(value any) map[string]any {
	schema, _ := value.(map[string]any)
	properties, _ := schema["properties"].(map[string]any)
	if len(properties) == 0 || len(properties) > 128 {
		return nil
	}
	projectedProperties := make(map[string]any)
	for name, raw := range properties {
		if len(projectedProperties) == 128 || !validText(name, 256) {
			continue
		}
		field, _ := raw.(map[string]any)
		fieldType := stringField(field, "type")
		if !containsString([]string{"string", "number", "integer", "boolean"}, fieldType) {
			continue
		}
		projected := map[string]any{"type": fieldType}
		for _, key := range []string{"title", "description"} {
			if text := interactionText(field[key]); text != "" {
				projected[key] = text
			}
		}
		values, _ := field["enum"].([]any)
		enums := make([]any, 0, min(len(values), 64))
		for _, rawValue := range values {
			if len(enums) == 64 {
				break
			}
			switch typed := rawValue.(type) {
			case string:
				if text, _ := boundedText(typed, maxInteractionPreviewBytes); text != "" {
					enums = append(enums, text)
				}
			default:
				if number, ok := finiteNumber(rawValue); ok {
					enums = append(enums, number)
				}
			}
		}
		if len(enums) > 0 {
			projected["enum"] = enums
		}
		projectedProperties[name] = projected
	}
	if len(projectedProperties) == 0 {
		return nil
	}
	result := map[string]any{"properties": projectedProperties}
	required, _ := schema["required"].([]any)
	projectedRequired := make([]any, 0, min(len(required), 128))
	seen := make(map[string]bool)
	for _, raw := range required {
		name, _ := raw.(string)
		if _, exists := projectedProperties[name]; !exists || seen[name] {
			continue
		}
		seen[name] = true
		projectedRequired = append(projectedRequired, name)
	}
	if len(projectedRequired) > 0 {
		result["required"] = projectedRequired
	}
	return result
}

func firstInteractionValue(source map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, exists := source[key]; exists && value != nil {
			return value
		}
	}
	return nil
}

func interactionText(value any) string {
	text, _ := value.(string)
	text, _ = boundedText(text, maxInteractionPreviewBytes)
	return text
}

func interactionPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsRune(value, 0) {
		return ""
	}
	if filepath.IsAbs(value) {
		return filepath.Base(value)
	}
	clean := filepath.Clean(value)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || filepath.IsAbs(clean) {
		return filepath.Base(clean)
	}
	return filepath.ToSlash(clean)
}

func (adapter *CodexAdapter) projectInteractionChanges(value any) []any {
	items, _ := value.([]any)
	result := make([]any, 0, min(len(items), 128))
	for _, raw := range items {
		if len(result) == 128 {
			break
		}
		change, _ := raw.(map[string]any)
		path := adapter.projectWorkspacePath(stringField(change, "path"))
		if path == "" || len(path) > maxInteractionPreviewBytes {
			continue
		}
		kind, _ := boundedText(changeKind(firstInteractionValue(change, "kind", "change")), maxInteractionPreviewBytes)
		result = append(result, map[string]any{"path": path, "change": kind})
	}
	return result
}

func permissionNames(value any) []any {
	permissions, _ := value.(map[string]any)
	result := make([]any, 0, 4)
	fileSystem, _ := permissions["fileSystem"].(map[string]any)
	if values, ok := fileSystem["read"].([]any); ok && len(values) > 0 {
		result = append(result, "filesystem.read")
	}
	if values, ok := fileSystem["write"].([]any); ok && len(values) > 0 {
		result = append(result, "filesystem.write")
	}
	if entries, ok := fileSystem["entries"].([]any); ok {
		seen := make(map[string]bool)
		for _, raw := range entries {
			entry, _ := raw.(map[string]any)
			access := interactionText(entry["access"])
			if access != "" && !seen[access] {
				result = append(result, "filesystem."+access)
				seen[access] = true
			}
		}
	}
	network, _ := permissions["network"].(map[string]any)
	if enabled, _ := network["enabled"].(bool); enabled {
		result = append(result, "network")
	}
	return result
}

func interactionArgumentsPreview(value any) string {
	projected := projectInteractionValue(value, "arguments")
	if projected == nil {
		return ""
	}
	encoded, err := json.Marshal(projected)
	if err != nil {
		return ""
	}
	preview, _ := boundedText(string(encoded), maxInteractionPreviewBytes)
	return preview
}

func projectInteractionFields(source map[string]any, allowed []string) map[string]any {
	result := make(map[string]any)
	for _, key := range allowed {
		value, ok := source[key]
		if !ok {
			continue
		}
		if projected := projectInteractionValue(value, key); projected != nil {
			result[key] = projected
		}
	}
	return result
}

func projectInteractionValue(value any, key string) any {
	if sensitiveKey(key) || strings.EqualFold(key, "id") || strings.HasSuffix(key, "Id") {
		return nil
	}
	commandText := strings.Contains(strings.ToLower(key), "command") || strings.Contains(strings.ToLower(key), "execpolicy")
	switch typed := value.(type) {
	case string:
		if pathKey(key) {
			if filepath.IsAbs(typed) {
				return filepath.Base(typed)
			}
			return strings.TrimPrefix(filepath.ToSlash(filepath.Clean(typed)), "../")
		}
		if !commandText && (filepath.IsAbs(typed) || strings.HasPrefix(strings.ToLower(typed), "file:")) {
			return nil
		}
		return typed
	case []any:
		result := make([]any, 0, len(typed))
		for _, item := range typed {
			if projected := projectInteractionValue(item, key); projected != nil {
				result = append(result, projected)
			}
		}
		return result
	case map[string]any:
		result := make(map[string]any)
		for name, item := range typed {
			if projected := projectInteractionValue(item, name); projected != nil {
				result[name] = projected
			}
		}
		return result
	default:
		return value
	}
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
			typeName := map[string]string{"text": "inputText", "image": "inputImage"}[fmt.Sprint(item["kind"])]
			field := "text"
			value := item["text"]
			if typeName == "inputImage" {
				field, value = "imageUrl", item["url"]
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
					messages = append(messages, map[string]any{"role": "user", "content": sanitizeSessionMessage(strings.Join(textParts, "\n"), maxSessionMessageRunes), "createdAt": turn["startedAt"]})
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
				"role": "assistant", "content": sanitizeSessionMessage(strings.Join(assistantParts, "\n\n"), maxSessionMessageRunes),
				"createdAt": firstValue(turn["completedAt"], turn["startedAt"]),
			}
			if len(reasoningParts) > 0 {
				message["reasoningContent"] = sanitizeSessionMessage(strings.Join(reasoningParts, "\n\n"), maxSessionMessageRunes)
			}
			messages = append(messages, message)
		}
	}
	return messages
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
