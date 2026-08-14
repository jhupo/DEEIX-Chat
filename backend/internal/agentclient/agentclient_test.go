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

func TestCodexAdapterUsesNativeProcessAndAPIKeyProof(t *testing.T) {
	root := t.TempDir()
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
		case "getAuthStatus":
			result = map[string]any{"authMethod": "apikey", "authToken": "sub2-test-key", "requiresOpenaiAuth": false}
		case "model/list":
			result = map[string]any{"data": []any{map[string]any{"id": "gpt-test", "displayName": "GPT Test"}}}
		}
		var id any
		_ = json.Unmarshal(request["id"], &id)
		_ = encoder.Encode(map[string]any{"id": id, "result": result})
	}
}
