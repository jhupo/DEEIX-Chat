package agentclient

import (
	"bufio"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Println("codex-cli 0.147.0")
		os.Exit(0)
	}
	if len(os.Args) == 2 && os.Args[1] == "app-server" {
		runFakeAppServer()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestRuntimeProofDeadline(t *testing.T) {
	now := time.Date(2026, time.August, 14, 12, 0, 0, 0, time.UTC)
	want := now.Add(time.Minute)
	got, err := runtimeProofDeadline(want.Format(time.RFC3339Nano), now)
	if err != nil || !got.Equal(want) {
		t.Fatalf("runtimeProofDeadline() = %v, %v, want %v", got, err, want)
	}
	for _, value := range []string{"invalid", now.Format(time.RFC3339Nano), now.Add(3 * time.Minute).Format(time.RFC3339Nano)} {
		if _, err = runtimeProofDeadline(value, now); err == nil {
			t.Fatalf("runtimeProofDeadline(%q) accepted", value)
		}
	}
}

func TestJitterReconnectDelayWithinBounds(t *testing.T) {
	base := 10 * time.Second
	for range 100 {
		delay := jitterReconnectDelay(base)
		if delay < 8*time.Second || delay > 12*time.Second {
			t.Fatalf("jitterReconnectDelay(%s) = %s", base, delay)
		}
	}
}

func TestBridgeAuthErrorIncludesStableCode(t *testing.T) {
	err := bridgeAuthError(bridgeFrame{
		Version: bridgeVersion, Type: "auth.error", ErrorCode: "runtime_key_rejected", ErrorMessage: "key rejected",
	})
	if err == nil || err.Error() != "key rejected (runtime_key_rejected)" {
		t.Fatalf("bridgeAuthError() = %v", err)
	}
}

func TestParseWorkspaceRegisterCommand(t *testing.T) {
	command, err := parseAgentCommand(json.RawMessage(`{
		"kind":"workspace.register",
		"deviceId":"agd_0123456789abcdef0123456789abcdef",
		"profileId":"codex-default",
		"path":"C:\\source\\project",
		"create":true
	}`))
	if err != nil || command.Path != `C:\source\project` || !command.Create {
		t.Fatalf("parseAgentCommand() = %#v, %v", command, err)
	}
}

func TestConfigRoundTripAndWorkspaceUpsert(t *testing.T) {
	root := t.TempDir()
	workspace := Workspace{WorkspaceID: "workspace-0123456789abcdef01234567", Root: root, Name: "workspace"}
	config := Config{
		Version: 1, CloudURL: "https://example.com", UserPublicID: "0123456789abcdef0123456789abcdef",
		DeviceID: "agd_0123456789abcdef0123456789abcdef", ProfileID: "codex-default", CodexExecutable: filepath.Join(root, "codex"),
		Workspaces: []Workspace{workspace},
	}
	path := filepath.Join(root, "config.json")
	if err := SaveConfig(path, config); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.DeviceID != config.DeviceID || loaded.Workspaces[0].Root != workspace.Root {
		t.Fatalf("unexpected config round trip: %#v", loaded)
	}
	upsertWorkspace(&loaded.Workspaces, workspace)
	if len(loaded.Workspaces) != 1 {
		t.Fatalf("workspace was duplicated: %#v", loaded.Workspaces)
	}
}

func TestRegisterWorkspacePersistsAndRejectsAgentData(t *testing.T) {
	dataDir := repositoryTestDir(t)
	initialRoot := repositoryTestDir(t)
	initial := Workspace{WorkspaceID: "workspace-0123456789abcdef01234567", Root: initialRoot, Name: "initial"}
	config := Config{
		Version: 1, CloudURL: "https://example.com", UserPublicID: "0123456789abcdef0123456789abcdef",
		DeviceID: "agd_0123456789abcdef0123456789abcdef", ProfileID: "codex-default", CodexExecutable: os.Args[0],
		Workspaces: []Workspace{initial},
	}
	if err := SaveConfig(filepath.Join(dataDir, "config.json"), config); err != nil {
		t.Fatal(err)
	}
	gateway := &Gateway{
		config: config, dataDir: dataDir, workspaces: map[string]Workspace{initial.WorkspaceID: initial},
		adapter: &CodexAdapter{workspaces: map[string]Workspace{initial.WorkspaceID: initial}},
	}
	createdPath := filepath.Join(repositoryTestDir(t), "new-project")
	result, err := gateway.registerWorkspace(AgentCommand{Path: createdPath, Create: true})
	if err != nil || result["workspaceId"] == "" {
		t.Fatalf("registerWorkspace() = %#v, %v", result, err)
	}
	loaded, err := LoadConfig(filepath.Join(dataDir, "config.json"))
	if err != nil || len(loaded.Workspaces) != 2 {
		t.Fatalf("registered config = %#v, %v", loaded.Workspaces, err)
	}
	reserved := filepath.Join(dataDir, "reserved-project")
	if _, err = gateway.registerWorkspace(AgentCommand{Path: reserved, Create: true}); err == nil {
		t.Fatal("Agent data directory was accepted as a workspace")
	}
	if _, err = os.Stat(reserved); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("rejected workspace directory was not rolled back: %v", err)
	}
	if _, err = gateway.registerWorkspace(AgentCommand{Path: filepath.Dir(dataDir)}); err == nil {
		t.Fatal("Agent data directory ancestor was accepted as a workspace")
	}
}

func repositoryTestDir(t *testing.T) string {
	t.Helper()
	directory, err := os.MkdirTemp(".", ".agent-test-*")
	if err != nil {
		t.Fatal(err)
	}
	absolute, err := filepath.Abs(directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(absolute) })
	return absolute
}

func TestCodexProjectWorkspaceCollapsesLinkedWorktree(t *testing.T) {
	repositoryRoot := filepath.Join(t.TempDir(), "source-repository")
	gitDirectory := filepath.Join(repositoryRoot, ".git")
	worktreeGitDirectory := filepath.Join(gitDirectory, "worktrees", "generated-task")
	worktreeRoot := filepath.Join(t.TempDir(), "generated-task")
	for _, directory := range []string{worktreeGitDirectory, worktreeRoot} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(worktreeGitDirectory, "commondir"), []byte("../..\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worktreeRoot, ".git"), []byte("gitdir: "+worktreeGitDirectory+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	workspace, err := codexProjectWorkspace(worktreeRoot)
	if err != nil {
		t.Fatal(err)
	}
	want, err := CanonicalWorkspace(repositoryRoot)
	if err != nil {
		t.Fatal(err)
	}
	canonicalWorktree, err := filepath.EvalSymlinks(worktreeRoot)
	if err != nil {
		t.Fatal(err)
	}
	if workspace.WorkspaceID != want.WorkspaceID || workspace.Root != want.Root || workspace.Name != want.Name ||
		len(workspace.SessionRoots) != 1 || workspace.SessionRoots[0] != canonicalWorktree {
		t.Fatalf("linked worktree was not collapsed: %#v", workspace)
	}
}

func TestCodexProjectWorkspaceRejectsDirectoryWithoutGitBoundary(t *testing.T) {
	if _, err := codexProjectWorkspace(t.TempDir()); err == nil {
		t.Fatal("non-Git thread directory was promoted to a project")
	}
}

func TestMergeWorkspacePreservesConfiguredSessionRoots(t *testing.T) {
	workspaces := map[string]Workspace{}
	mergeWorkspace(workspaces, Workspace{WorkspaceID: "workspace-one", Root: "repo", SessionRoots: []string{"worktree-a"}})
	mergeWorkspace(workspaces, Workspace{WorkspaceID: "workspace-one", Root: "repo", SessionRoots: []string{"worktree-b", "worktree-a"}})
	got := workspaces["workspace-one"].SessionRoots
	if len(got) != 2 || got[0] != "worktree-a" || got[1] != "worktree-b" {
		t.Fatalf("workspace session roots were not merged: %#v", got)
	}
}

func TestCloudBoundaryRejectsMetadataAndRedirects(t *testing.T) {
	if _, err := NormalizeCloudURL("http://169.254.169.254/latest/meta-data"); err == nil {
		t.Fatal("metadata endpoint was accepted as a server URL")
	}
	if _, err := NormalizeCloudURL("http://example.com"); err == nil {
		t.Fatal("unencrypted remote server URL was accepted")
	}
	redirected := false
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		redirected = true
	}))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		http.Redirect(response, &http.Request{}, target.URL, http.StatusFound)
	}))
	defer source.Close()
	response, err := newAgentHTTPClient().Get(source.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusFound || redirected {
		t.Fatal("agent HTTP client followed a redirect")
	}
}

func TestInstallEnrollsOnceAndReusesDeviceIdentity(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/agent/bridge/enrollment-challenges":
			requests.Add(1)
			_ = json.NewEncoder(response).Encode(map[string]any{"data": map[string]any{
				"challengeId": "age_0123456789abcdef0123456789abcdef",
				"canonical":   "deeix-device-enrollment-v1\nuser\ndevice\nchallenge\nexpiry\nnonce",
				"expiresAt":   time.Now().Add(time.Minute).UTC().Format(time.RFC3339),
			}})
		case "/api/v1/agent/bridge/enrollments":
			requests.Add(1)
			_ = json.NewEncoder(response).Encode(map[string]any{"data": map[string]any{
				"deviceId": "agd_0123456789abcdef0123456789abcdef",
				"status":   "active",
			}})
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp(".", ".agent-install-workspace-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	codexHome, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(codexHome, "auth.json"), []byte("{\"OPENAI_API_KEY\":\"sub2-test-key\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DEEIX_TEST_THREAD_CWD", codexHome)
	dataRoot, err := os.MkdirTemp(".", ".agent-install-data-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dataRoot) })
	dataDir := filepath.Join(dataRoot, "agent")
	options := InstallOptions{
		Server: server.URL, UserPublicID: "0123456789abcdef0123456789abcdef", Workspace: root,
		Name: "fixture", CodexExecutable: executable, DataDir: dataDir,
	}
	first, err := Install(context.Background(), options, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	identityBefore, err := os.ReadFile(filepath.Join(dataDir, "device-identity.json"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := Install(context.Background(), options, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	identityAfter, err := os.ReadFile(filepath.Join(dataDir, "device-identity.json"))
	if err != nil {
		t.Fatal(err)
	}
	if first.Updated || !second.Updated || first.DeviceID != second.DeviceID || string(identityBefore) != string(identityAfter) {
		t.Fatalf("install was not idempotent: first=%#v second=%#v", first, second)
	}
	if requests.Load() != 2 {
		t.Fatalf("repeat install enrolled a second device: %d requests", requests.Load())
	}
	identityPath := filepath.Join(dataDir, "device-identity.json")
	if err = os.Remove(identityPath); err != nil {
		t.Fatal(err)
	}
	if _, err = Install(context.Background(), options, io.Discard); err == nil {
		t.Fatal("existing device configuration accepted a replacement identity")
	}
	if _, err = os.Stat(identityPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("existing device configuration regenerated its missing identity")
	}
}

func TestStatePersistsCommandOutcomeAndSourceMapping(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store, err := OpenStateStore(path)
	if err != nil {
		t.Fatal(err)
	}
	command := json.RawMessage(`{"kind":"resource.refresh","deviceId":"agd_0123456789abcdef0123456789abcdef","profileId":"codex-default","resource":{"scope":"profile","name":"models"}}`)
	record, created, err := store.Receive(1, "agcmd_0123456789abcdef0123456789abcdef", command, nil)
	if err != nil || !created || record.ServerSeq != 1 {
		t.Fatalf("receive failed: created=%v record=%#v err=%v", created, record, err)
	}
	if err = store.MarkStarted("agcmd_0123456789abcdef0123456789abcdef"); err != nil {
		t.Fatal(err)
	}
	outcome := json.RawMessage(`{"kind":"result","result":{"kind":"accepted"}}`)
	frame, err := store.AppendTerminal("agcmd_0123456789abcdef0123456789abcdef", outcome)
	if err != nil || frame.BridgeSeq != 1 {
		t.Fatalf("append terminal failed: %#v %v", frame, err)
	}
	reference, err := store.PublishSource("codex-default", "thread", "provider-thread-1")
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenStateStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = reopened.AcknowledgeServer(3); err != nil {
		t.Fatal(err)
	}
	if server, _ := reopened.Cursors(); server != 3 {
		t.Fatalf("server cursor was not reconciled: %d", server)
	}
	nextCommand := json.RawMessage(`{"kind":"resource.refresh","deviceId":"agd_0123456789abcdef0123456789abcdef","profileId":"codex-default","resource":{"scope":"profile","name":"apps"}}`)
	if _, created, receiveErr := reopened.Receive(4, "agcmd_1123456789abcdef0123456789abcdef", nextCommand, nil); receiveErr != nil || !created {
		t.Fatalf("receive after server cursor reconciliation failed: created=%v err=%v", created, receiveErr)
	}
	resolved, err := reopened.ResolveSource("codex-default", "thread", reference)
	if err != nil || resolved != "provider-thread-1" {
		t.Fatalf("source mapping was not durable: %q %v", resolved, err)
	}
	if err = reopened.AcknowledgeBridge(1); err != nil {
		t.Fatal(err)
	}
	if pending := reopened.PendingOutgoing(0); len(pending) != 0 {
		t.Fatalf("acknowledged frames were retained: %#v", pending)
	}
}

func TestParseAgentCommandRejectsUnknownField(t *testing.T) {
	_, err := parseAgentCommand(json.RawMessage(`{"kind":"resource.refresh","deviceId":"agd_0123456789abcdef0123456789abcdef","profileId":"codex-default","resource":{"scope":"profile","name":"models"},"extra":true}`))
	if err == nil {
		t.Fatal("unknown trust-boundary field was accepted")
	}
}

func TestParseAgentCommandAcceptsThreadRead(t *testing.T) {
	command, err := parseAgentCommand(json.RawMessage(`{"kind":"thread.read","deviceId":"agd_0123456789abcdef0123456789abcdef","profileId":"codex-default","workspaceId":"workspace-1","threadId":"agth_0123456789abcdef0123456789abcdef","sourceThreadRef":"source-thread-1"}`))
	if err != nil || command.Kind != "thread.read" || command.SourceThreadRef != "source-thread-1" {
		t.Fatalf("thread read command was rejected: %#v %v", command, err)
	}
}

func TestRPCServerRequestDoesNotBlockResponses(t *testing.T) {
	serverReads, clientWrites := io.Pipe()
	clientReads, serverWrites := io.Pipe()
	client := NewRPCClient(clientWrites, clientReads)
	defer client.Close()
	requestSeen := make(chan struct{})
	releaseRequest := make(chan struct{})
	client.SetHandlers(nil, func(context.Context, RPCServerRequest) (any, error) {
		close(requestSeen)
		<-releaseRequest
		return map[string]any{"decision": "accept"}, nil
	})
	go func() {
		buffer := make([]byte, 4096)
		n, _ := serverReads.Read(buffer)
		var request map[string]any
		_ = json.Unmarshal(buffer[:n], &request)
		_, _ = serverWrites.Write([]byte(`{"id":"approval-1","method":"item/commandExecution/requestApproval","params":{}}` + "\n"))
		response, _ := json.Marshal(map[string]any{"id": request["id"], "result": map[string]any{"ok": true}})
		_, _ = serverWrites.Write(append(response, '\n'))
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var result struct {
		OK bool `json:"ok"`
	}
	if err := client.Request(ctx, "test/read", map[string]any{}, &result); err != nil {
		t.Fatal(err)
	}
	if !result.OK {
		t.Fatal("RPC response was not decoded")
	}
	select {
	case <-requestSeen:
	case <-ctx.Done():
		t.Fatal("server request handler did not start")
	}
	close(releaseRequest)
}

func TestLockedAppServerContractMatchesNativeRegistry(t *testing.T) {
	path := filepath.Join("..", "..", "..", "docs", "agent-runtime", "codex-app-server-v0.147.0.lock.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var lock struct {
		GeneratedArtifacts struct {
			FullJSONBundle struct {
				SHA256 string `json:"sha256"`
			} `json:"full_json_bundle"`
		} `json:"generated_artifacts"`
		Unions map[string]struct {
			Members []struct {
				Name        string `json:"name"`
				Disposition string `json:"disposition"`
			} `json:"members"`
		} `json:"unions"`
	}
	if err = json.Unmarshal(data, &lock); err != nil {
		t.Fatal(err)
	}
	if lock.GeneratedArtifacts.FullJSONBundle.SHA256 != codexSchemaHash {
		t.Fatalf("schema hash drift: %s", lock.GeneratedArtifacts.FullJSONBundle.SHA256)
	}
	assertMappedSet(t, lock.Unions["ClientRequest"].Members, dispatchedClientRequests)
	assertMappedSet(t, lock.Unions["ServerRequest"].Members, mappedServerRequests)
	assertMappedSet(t, lock.Unions["ServerNotification"].Members, mappedNotifications)
}

func TestProjectSessionMessagesMergesAssistantItemsWithinTurn(t *testing.T) {
	var detail map[string]any
	if err := json.Unmarshal([]byte(`{
		"thread":{"turns":[{
			"startedAt":1786615200,
			"completedAt":1786615260,
			"items":[
				{"type":"userMessage","content":[{"type":"text","text":"inspect"}]},
				{"type":"reasoning","summary":["checked configuration"],"content":[]},
				{"type":"agentMessage","text":"first paragraph"},
				{"type":"agentMessage","text":"second paragraph"}
			]
		}]}
	}`), &detail); err != nil {
		t.Fatal(err)
	}
	messages := projectSessionMessages(detail)
	if len(messages) != 2 {
		t.Fatalf("projected messages = %#v", messages)
	}
	assistant, ok := messages[1].(map[string]any)
	if !ok || assistant["content"] != "first paragraph\n\nsecond paragraph" || assistant["reasoningContent"] != "checked configuration" {
		t.Fatalf("projected assistant message = %#v", messages[1])
	}
}

func TestCodexAdapterUsesNativeProcessAndAPIKeyProof(t *testing.T) {
	root, err := os.MkdirTemp(".", ".agent-codex-workspace-")
	if err != nil {
		t.Fatal(err)
	}
	root, err = filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	if err = os.Mkdir(filepath.Join(root, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "auth.json"), []byte("{\"OPENAI_API_KEY\":\"sub2-test-key\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DEEIX_TEST_THREAD_CWD", root)
	state, err := OpenStateStore(filepath.Join(root, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	config := Config{
		Version: 1, CloudURL: "https://example.com", UserPublicID: "0123456789abcdef0123456789abcdef",
		DeviceID: "agd_0123456789abcdef0123456789abcdef", ProfileID: "codex-default", CodexExecutable: executable,
		Workspaces: []Workspace{{WorkspaceID: "workspace-0123456789abcdef01234567", Root: root, Name: "workspace"}},
	}
	adapter, err := StartCodexAdapter(context.Background(), config, state, io.Discard, func(json.RawMessage) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer adapter.Close()
	challenge := "deeix-runtime-auth-proof-v1\nuser\ndevice\nprofile\nfingerprint\nnonce\n1"
	proof, err := adapter.ProveRuntimeAuth(context.Background(), challenge)
	if err != nil {
		t.Fatal(err)
	}
	mac := hmac.New(sha256.New, []byte("sub2-test-key"))
	_, _ = mac.Write([]byte(challenge))
	if proof != base64.RawURLEncoding.EncodeToString(mac.Sum(nil)) {
		t.Fatalf("unexpected runtime proof: %s", proof)
	}
	diagnostic := adapter.RuntimeAuthDiagnostic()
	if strings.Contains(diagnostic, "sub2-test-key") || !strings.Contains(diagnostic, "key=sub2...-key") ||
		!strings.Contains(diagnostic, "fingerprint=sha256:") || !strings.Contains(diagnostic, "codexHome=") {
		t.Fatalf("unsafe or incomplete runtime auth diagnostic: %q", diagnostic)
	}
	result, err := adapter.Execute(context.Background(), AgentCommand{
		Kind: "resource.refresh", DeviceID: config.DeviceID, ProfileID: config.ProfileID,
		Resource: &struct {
			Scope string `json:"scope"`
			Name  string `json:"name"`
		}{Scope: "profile", Name: "models"},
	}, nil)
	if err != nil || result["kind"] != "resource" {
		t.Fatalf("resource request failed: %#v %v", result, err)
	}
	apps, err := adapter.Execute(context.Background(), AgentCommand{
		Kind: "resource.refresh", DeviceID: config.DeviceID, ProfileID: config.ProfileID,
		Resource: &struct {
			Scope string `json:"scope"`
			Name  string `json:"name"`
		}{Scope: "profile", Name: "apps"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	appCatalog, _ := apps["data"].(map[string]any)
	appItems, _ := appCatalog["data"].([]any)
	if len(appItems) != 1 {
		t.Fatalf("unexpected app snapshot: %#v", apps)
	}
	app, _ := appItems[0].(map[string]any)
	appRef, _ := app["resourceRef"].(string)
	if !strings.HasPrefix(appRef, "app_") || app["id"] != nil || strings.Contains(fmt.Sprint(apps), "calendar-private-id") {
		t.Fatalf("unsafe app snapshot: %#v", apps)
	}
	workspaces, err := adapter.DiscoverWorkspaces(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want, err := CanonicalWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(workspaces) != 1 || workspaces[0].WorkspaceID != want.WorkspaceID || workspaces[0].Root != want.Root ||
		workspaces[0].Name != want.Name || len(workspaces[0].SessionRoots) != 1 || workspaces[0].SessionRoots[0] != want.Root {
		t.Fatalf("unexpected discovered workspaces: %#v", workspaces)
	}
	adapter.replaceWorkspaces(workspaces)
	skills, err := adapter.Execute(context.Background(), AgentCommand{
		Kind: "resource.refresh", DeviceID: config.DeviceID, ProfileID: config.ProfileID, WorkspaceID: want.WorkspaceID,
		Resource: &struct {
			Scope string `json:"scope"`
			Name  string `json:"name"`
		}{Scope: "workspace", Name: "skills"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	skillCatalog, _ := skills["data"].(map[string]any)
	skillItems, _ := skillCatalog["data"].([]any)
	if len(skillItems) != 1 {
		t.Fatalf("unexpected skill snapshot: %#v", skills)
	}
	skill, _ := skillItems[0].(map[string]any)
	skillRef, _ := skill["resourceRef"].(string)
	if !strings.HasPrefix(skillRef, "skill_") || skill["path"] != nil || strings.Contains(fmt.Sprint(skills), root) {
		t.Fatalf("unsafe skill snapshot: %#v", skills)
	}
	threadRef, err := state.PublishSource(config.ProfileID, "thread", "thread-input-test")
	if err != nil {
		t.Fatal(err)
	}
	turn, err := adapter.Execute(context.Background(), AgentCommand{
		Kind: "turn.start", DeviceID: config.DeviceID, ProfileID: config.ProfileID, WorkspaceID: want.WorkspaceID,
		ThreadID: "agth_0123456789abcdef0123456789abcdef", SourceThreadRef: threadRef,
		Input: []AgentInput{
			{Kind: "text", Text: "use both resources"},
			{Kind: "skill", ResourceRef: skillRef},
			{Kind: "app-mention", ResourceRef: appRef},
		},
		Settings: &Settings{},
	}, nil)
	if err != nil || turn["kind"] != "turn-started" {
		t.Fatalf("resource input request failed: %#v %v", turn, err)
	}
	sessions, err := adapter.Execute(context.Background(), AgentCommand{
		Kind: "resource.refresh", DeviceID: config.DeviceID, ProfileID: config.ProfileID, WorkspaceID: want.WorkspaceID,
		Resource: &struct {
			Scope string `json:"scope"`
			Name  string `json:"name"`
		}{Scope: "workspace", Name: "sessions"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	catalog, _ := sessions["data"].(map[string]any)
	items, _ := catalog["data"].([]any)
	if len(items) != 8 {
		t.Fatalf("unexpected session summary count: %#v", sessions)
	}
	first, _ := items[0].(map[string]any)
	if first["historyLoaded"] != false || first["messages"] != nil {
		t.Fatalf("session catalog eagerly included history: %#v", first)
	}
	sourceRef, _ := first["sourceThreadRef"].(string)
	detail, err := adapter.Execute(context.Background(), AgentCommand{
		Kind: "thread.read", DeviceID: config.DeviceID, ProfileID: config.ProfileID, WorkspaceID: want.WorkspaceID,
		ThreadID: "agth_0123456789abcdef0123456789abcdef", SourceThreadRef: sourceRef,
	}, nil)
	if err != nil || detail["kind"] != "thread-read" {
		t.Fatalf("thread detail request failed: %#v %v", detail, err)
	}
	session, _ := detail["session"].(map[string]any)
	messages, _ := session["messages"].([]any)
	if session["historyLoaded"] != true || len(messages) != 2 {
		t.Fatalf("thread detail did not project messages: %#v", session)
	}
}

func TestReadCodexAPIKeyRejectsInvalidCredential(t *testing.T) {
	root, err := os.MkdirTemp(".", ".agent-codex-auth-")
	if err != nil {
		t.Fatal(err)
	}
	root, err = filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	if err = os.WriteFile(filepath.Join(root, "auth.json"), []byte("{\"OPENAI_API_KEY\":\"line\\nkey\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err = readCodexAPIKey(root); err == nil {
		t.Fatal("accepted a multiline Codex API key")
	}
}

func assertMappedSet(t *testing.T, members []struct {
	Name        string `json:"name"`
	Disposition string `json:"disposition"`
}, actual map[string]bool) {
	t.Helper()
	expected := make(map[string]bool)
	for _, member := range members {
		if member.Disposition == "mapped" {
			expected[member.Name] = true
		}
	}
	if len(expected) != len(actual) {
		t.Fatalf("mapped method count drift: lock=%d native=%d", len(expected), len(actual))
	}
	for method := range expected {
		if !actual[method] {
			t.Fatalf("locked mapped method is missing: %s", method)
		}
	}
}

func runFakeAppServer() {
	reader := bufio.NewScanner(os.Stdin)
	reader.Buffer(make([]byte, 64*1024), maxRPCLineBytes)
	encoder := json.NewEncoder(os.Stdout)
	for reader.Scan() {
		var request map[string]json.RawMessage
		if json.Unmarshal(reader.Bytes(), &request) != nil || len(request["id"]) == 0 {
			continue
		}
		var method string
		_ = json.Unmarshal(request["method"], &method)
		var result any = map[string]any{}
		switch method {
		case "initialize":
			result = map[string]any{"codexHome": os.Getenv("DEEIX_TEST_THREAD_CWD")}
		case "account/read":
			result = map[string]any{"account": map[string]any{"type": "apiKey"}, "requiresOpenaiAuth": true}
		case "model/list":
			result = map[string]any{"data": []any{map[string]any{"id": "gpt-test", "displayName": "GPT Test"}}}
		case "app/list":
			result = map[string]any{"data": []any{map[string]any{"id": "calendar-private-id", "name": "Calendar", "description": "Read calendar events", "enabled": true}}, "nextCursor": nil}
		case "skills/list":
			root := os.Getenv("DEEIX_TEST_THREAD_CWD")
			result = map[string]any{"data": []any{map[string]any{
				"cwd":    root,
				"skills": []any{map[string]any{"name": "review", "description": "Review changes", "path": filepath.Join(root, ".codex", "skills", "review", "SKILL.md"), "enabled": true}},
			}}}
		case "turn/start":
			var params struct {
				Input []map[string]any `json:"input"`
			}
			_ = json.Unmarshal(request["params"], &params)
			root := os.Getenv("DEEIX_TEST_THREAD_CWD")
			if len(params.Input) == 3 && params.Input[1]["type"] == "skill" && params.Input[1]["name"] == "review" &&
				params.Input[1]["path"] == filepath.Join(root, ".codex", "skills", "review", "SKILL.md") &&
				params.Input[2]["type"] == "mention" && params.Input[2]["name"] == "Calendar" && params.Input[2]["path"] == "app://calendar-private-id" {
				result = map[string]any{"turn": map[string]any{"id": "turn-input-test"}}
			}
		case "thread/list":
			root := os.Getenv("DEEIX_TEST_THREAD_CWD")
			var params map[string]any
			_ = json.Unmarshal(request["params"], &params)
			if params["cursor"] == "next" {
				result = map[string]any{"data": []any{map[string]any{"id": "thread-3", "cwd": root, "name": "Third thread"}}, "nextCursor": nil}
			} else {
				result = map[string]any{"data": []any{
					map[string]any{"id": "thread-1", "cwd": root, "name": "First thread", "preview": "first"},
					map[string]any{"id": "thread-2", "cwd": root, "name": "Second thread", "preview": "second"},
					map[string]any{"id": "thread-missing-cwd"},
				}, "nextCursor": "next"}
			}
		case "thread/read":
			result = map[string]any{"thread": map[string]any{
				"id": "thread-1", "name": "First thread", "preview": "first",
				"turns": []any{map[string]any{"startedAt": 1, "completedAt": 2, "items": []any{
					map[string]any{"type": "userMessage", "content": []any{map[string]any{"type": "text", "text": "hello"}}},
					map[string]any{"type": "agentMessage", "text": "world"},
				}}},
			}}
		}
		var id any
		_ = json.Unmarshal(request["id"], &id)
		_ = encoder.Encode(map[string]any{"id": id, "result": result})
	}
}
