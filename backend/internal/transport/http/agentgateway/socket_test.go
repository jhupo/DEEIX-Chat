package agentgateway

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	appagent "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/agentgateway"
	domainagent "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/agentgateway"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
	"golang.org/x/net/websocket"
)

func TestConnectionTokenProtocols(t *testing.T) {
	token, err := connectionToken([]string{bridgeProtocol, authProtocolPrefix + "deeix_connection_value"})
	if err != nil || token != "deeix_connection_value" {
		t.Fatalf("connectionToken() = %q, %v", token, err)
	}
	invalid := [][]string{
		{},
		{bridgeProtocol},
		{authProtocolPrefix + "value"},
		{bridgeProtocol, authProtocolPrefix + "one", authProtocolPrefix + "two"},
		{bridgeProtocol, "other", authProtocolPrefix + "value"},
	}
	for _, protocols := range invalid {
		if _, err := connectionToken(protocols); err == nil {
			t.Fatalf("accepted protocols: %#v", protocols)
		}
	}
}

func TestClientFramesRejectServerArtifactGrants(t *testing.T) {
	artifacts := []bridgeArtifactGrant{}
	frame := bridgeFrame{Version: 1, Type: "ping", Artifacts: &artifacts}
	if validPingFrame(frame) {
		t.Fatal("client frame must not carry server artifact grants")
	}
}

func TestBridgeSocketHandshakeAndSingleUseCredential(t *testing.T) {
	const token = "deeix_connection_test_value"
	const runtimeKey = "sk-test-runtime-key"
	const userPublicID = "f6f910e920934def9a5cda479fc25251"
	hash := sha256.Sum256([]byte(token))
	repo := &socketRepo{
		tokenHash: hex.EncodeToString(hash[:]),
		device: domainagent.Device{
			ID: 3, PublicID: "agd_f6f910e920934def9a5cda479fc25251", UserID: 7,
			Status: domainagent.DeviceStatusActive, CredentialVersion: 1,
			PublicKeyFingerprint: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		},
		commands: []domainagent.Command{{
			ID: 11, PublicID: "agcmd_0123456789abcdef0123456789abcdef", UserID: 7, DeviceID: 3,
			ServerSeq: 1, Kind: "resource.refresh", State: "queued",
			PayloadJSON: `{"kind":"resource.refresh","deviceId":"agd_f6f910e920934def9a5cda479fc25251","profileId":"profile_1","resource":{"scope":"profile","name":"models"}}`,
		}},
	}
	service, err := appagent.NewService(repo, "01234567890123456789012345678901")
	if err != nil {
		t.Fatal(err)
	}
	runtimeAuth := &socketRuntimeAuth{userPublicID: userPublicID, key: runtimeKey}
	service.SetRuntimeAuth(runtimeAuth, runtimeAuth)
	handler := NewHandler(service)
	server := httptest.NewServer(http.HandlerFunc(handler.connect))
	defer server.Close()

	config, err := websocket.NewConfig("ws"+strings.TrimPrefix(server.URL, "http"), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	config.Protocol = []string{bridgeProtocol, authProtocolPrefix + token}
	connection, err := websocket.DialConfig(config)
	if err != nil {
		t.Fatalf("dial bridge: %v", err)
	}
	defer connection.Close()

	if err = websocket.JSON.Send(connection, bridgeFrame{Version: bridgeVersion, Type: "hello", ProfileID: "profile_1"}); err != nil {
		t.Fatal(err)
	}
	var challenge bridgeFrame
	if err = websocket.JSON.Receive(connection, &challenge); err != nil {
		t.Fatal(err)
	}
	if challenge.Type != "auth.challenge" || challenge.ProfileID != "profile_1" ||
		!strings.Contains(challenge.Challenge, "\n"+userPublicID+"\n") {
		t.Fatalf("unexpected runtime challenge: %#v", challenge)
	}
	mac := hmac.New(sha256.New, []byte(runtimeKey))
	_, _ = mac.Write([]byte(challenge.Challenge))
	if err = websocket.JSON.Send(connection, bridgeFrame{
		Version: bridgeVersion, Type: "auth.proof", ProfileID: "profile_1",
		ChallengeID: challenge.ChallengeID, Proof: base64.RawURLEncoding.EncodeToString(mac.Sum(nil)),
		Workspaces: []bridgeWorkspace{{WorkspaceID: "workspace_1", Name: "workspace_1"}},
	}); err != nil {
		t.Fatal(err)
	}
	var ready bridgeFrame
	if err = websocket.JSON.Receive(connection, &ready); err != nil || ready.Type != "auth.ready" || ready.ProfileID != "profile_1" {
		t.Fatalf("unexpected runtime ready: %#v, %v", ready, err)
	}
	var welcome bridgeFrame
	if err = websocket.JSON.Receive(connection, &welcome); err != nil {
		t.Fatal(err)
	}
	if welcome.Type != "welcome" || welcome.DeviceID != repo.device.PublicID || welcome.HeartbeatSeconds != 30 {
		t.Fatalf("unexpected welcome: %#v", welcome)
	}
	var command bridgeFrame
	if err = websocket.JSON.Receive(connection, &command); err != nil {
		t.Fatal(err)
	}
	if command.Type != "command" || command.ServerSeq != 1 || command.CommandID != repo.commands[0].PublicID {
		t.Fatalf("unexpected command: %#v", command)
	}
	if err = websocket.JSON.Send(connection, bridgeFrame{Version: bridgeVersion, Type: "ack.server", AckServerSeq: 1}); err != nil {
		t.Fatal(err)
	}
	outcome := []byte(`{"kind":"result","result":{"kind":"accepted"}}`)
	if err = websocket.JSON.Send(connection, bridgeFrame{
		Version: bridgeVersion, Type: "terminal", BridgeSeq: 1, ServerSeq: 1,
		CommandID: command.CommandID, Outcome: outcome,
	}); err != nil {
		t.Fatal(err)
	}
	var terminalAck bridgeFrame
	if err = websocket.JSON.Receive(connection, &terminalAck); err != nil || terminalAck.Type != "ack.bridge" || terminalAck.AckBridgeSeq != 1 {
		t.Fatalf("unexpected bridge ack: %#v, %v", terminalAck, err)
	}
	event := []byte(`{"kind":"item/agentMessage/delta","sourceThreadRef":"thread_1","occurredAt":"2026-08-13T00:00:00Z","payload":{"delta":"hello"}}`)
	if err = websocket.JSON.Send(connection, bridgeFrame{
		Version: bridgeVersion, Type: "event", BridgeSeq: 2, Event: event,
	}); err != nil {
		t.Fatal(err)
	}
	var eventAck bridgeFrame
	if err = websocket.JSON.Receive(connection, &eventAck); err != nil || eventAck.Type != "ack.bridge" || eventAck.AckBridgeSeq != 2 {
		t.Fatalf("unexpected event ack: %#v, %v", eventAck, err)
	}
	if err = websocket.JSON.Send(connection, bridgeFrame{Version: bridgeVersion, Type: "ping"}); err != nil {
		t.Fatal(err)
	}
	var pong bridgeFrame
	if err = websocket.JSON.Receive(connection, &pong); err != nil || pong.Type != "pong" {
		t.Fatalf("unexpected pong: %#v, %v", pong, err)
	}

	secondConfig, err := websocket.NewConfig("ws"+strings.TrimPrefix(server.URL, "http"), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	secondConfig.Protocol = []string{bridgeProtocol, authProtocolPrefix + token}
	if second, dialErr := websocket.DialConfig(secondConfig); dialErr == nil {
		_ = second.Close()
		t.Fatal("single-use connection credential was accepted twice")
	}
}

type socketRepo struct {
	mu        sync.Mutex
	tokenHash string
	device    domainagent.Device
	consumed  bool
	commands  []domainagent.Command
	serverAck uint64
	bridgeAck uint64
}

func TestBridgeHubWakesEveryDeviceForTheUser(t *testing.T) {
	hub := newBridgeHub()
	hub.connections["device-a"] = &bridgeConnection{userID: 7, wake: make(chan struct{}, 1)}
	hub.connections["device-b"] = &bridgeConnection{userID: 7, wake: make(chan struct{}, 1)}
	hub.connections["device-c"] = &bridgeConnection{userID: 8, wake: make(chan struct{}, 1)}
	browser, cleanup := hub.subscribeUser(7)
	defer cleanup()

	hub.notifyUser(7)
	for _, deviceID := range []string{"device-a", "device-b"} {
		select {
		case <-hub.connections[deviceID].wake:
		default:
			t.Fatalf("%s did not receive the user wake", deviceID)
		}
	}
	select {
	case <-hub.connections["device-c"].wake:
		t.Fatal("wake crossed the user boundary")
	default:
	}
	select {
	case <-browser:
	default:
		t.Fatal("browser subscriber did not receive the user wake")
	}
}

func (r *socketRepo) ConsumeConnection(_ context.Context, tokenHash string, now time.Time) (*domainagent.Device, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.consumed || tokenHash != r.tokenHash {
		return nil, repository.ErrConflict
	}
	r.consumed = true
	copy := r.device
	copy.LastSeenAt = &now
	return &copy, nil
}

func (*socketRepo) CreateEnrollmentChallenge(context.Context, *domainagent.DeviceEnrollmentChallenge) error {
	return nil
}
func (*socketRepo) GetEnrollmentChallenge(context.Context, string) (*domainagent.DeviceEnrollmentChallenge, error) {
	return nil, repository.ErrNotFound
}
func (*socketRepo) ConsumeEnrollmentChallengeAndEnroll(context.Context, uint, *domainagent.Device, time.Time) (*domainagent.Device, error) {
	return nil, repository.ErrNotFound
}
func (*socketRepo) ListDevices(context.Context, uint) ([]domainagent.Device, error) {
	return nil, nil
}
func (*socketRepo) GetDevice(context.Context, uint, string) (*domainagent.Device, error) {
	return nil, repository.ErrNotFound
}
func (r *socketRepo) GetDeviceByPublicID(_ context.Context, publicID string) (*domainagent.Device, error) {
	if publicID != r.device.PublicID {
		return nil, repository.ErrNotFound
	}
	copy := r.device
	return &copy, nil
}
func (*socketRepo) RenameDevice(context.Context, uint, string, string) (*domainagent.Device, error) {
	return nil, repository.ErrNotFound
}
func (*socketRepo) RevokeDevice(context.Context, uint, string, time.Time) error {
	return repository.ErrNotFound
}
func (*socketRepo) CreateDeviceCredential(context.Context, uint, *domainagent.Credential) error {
	return repository.ErrNotFound
}
func (*socketRepo) GetCredential(context.Context, string, string) (*domainagent.Credential, error) {
	return nil, repository.ErrNotFound
}
func (*socketRepo) ConsumeChallengeAndCreateConnection(context.Context, uint, uint, *domainagent.Credential, time.Time) (*domainagent.Credential, error) {
	return nil, repository.ErrNotFound
}
func (r *socketRepo) ListCommandsForDelivery(_ context.Context, _ uint, after uint64, _ int) ([]domainagent.Command, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]domainagent.Command, 0, len(r.commands))
	for _, command := range r.commands {
		if command.ServerSeq > after {
			result = append(result, command)
		}
	}
	return result, nil
}
func (r *socketRepo) MarkCommandDelivered(_ context.Context, _ uint, commandID uint, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.commands) == 0 || r.commands[0].ID != commandID {
		return repository.ErrNotFound
	}
	r.commands[0].DeliveredAt = &now
	return nil
}
func (r *socketRepo) AckServerCommands(_ context.Context, _ uint, through uint64, _ time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if through > uint64(len(r.commands)) {
		return repository.ErrConflict
	}
	if through > r.serverAck {
		r.serverAck = through
	}
	return nil
}
func (r *socketRepo) ApplyTerminalFrame(_ context.Context, _ uint, bridgeSeq, serverSeq uint64, commandID, _, payloadJSON string, _ time.Time) (uint64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if bridgeSeq != r.bridgeAck+1 || serverSeq != 1 || commandID != r.commands[0].PublicID || payloadJSON == "" {
		return r.bridgeAck, repository.ErrConflict
	}
	r.bridgeAck = bridgeSeq
	return r.bridgeAck, nil
}
func (r *socketRepo) ApplyEventFrame(_ context.Context, _, _ uint, bridgeSeq uint64, _ string, event *domainagent.Event, _ time.Time) (*domainagent.AppliedEventFrame, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if bridgeSeq != r.bridgeAck+1 || event == nil || event.PayloadJSON == "" {
		return nil, repository.ErrConflict
	}
	r.bridgeAck = bridgeSeq
	return &domainagent.AppliedEventFrame{Acknowledged: r.bridgeAck, Event: *event}, nil
}
func (*socketRepo) ListPendingConversationEvents(context.Context, uint, int) ([]domainagent.AppliedEventFrame, error) {
	return nil, nil
}
func (*socketRepo) MarkConversationEventProjected(context.Context, uint, time.Time) error { return nil }

func (r *socketRepo) BeginRuntimeProof(_ context.Context, deviceID uint, profilePublicID string, profile *domainagent.RuntimeProfile, challenge *domainagent.RuntimeProofChallenge, _ time.Time) (*domainagent.RuntimeProfile, *domainagent.RuntimeProofChallenge, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if deviceID != r.device.ID || profilePublicID == "" {
		return nil, nil, repository.ErrNotFound
	}
	profile.ID = 21
	challenge.ID = 22
	challenge.ProfileID = profile.ID
	profileCopy, challengeCopy := *profile, *challenge
	return &profileCopy, &challengeCopy, nil
}

func (r *socketRepo) CompleteRuntimeProof(_ context.Context, deviceID, profileID, challengeID uint, remoteKeyID int64, credentialHash string, _, _ time.Time) error {
	if deviceID != r.device.ID || profileID != 21 || challengeID != 22 || remoteKeyID != 31 || len(credentialHash) != 64 {
		return repository.ErrConflict
	}
	return nil
}

func (r *socketRepo) SyncWorkspaces(_ context.Context, userID, deviceID, profileID uint, items []domainagent.Workspace, _ time.Time) error {
	if userID != r.device.UserID || deviceID != r.device.ID || profileID != 21 || len(items) != 1 || items[0].PublicID != "workspace_1" {
		return repository.ErrConflict
	}
	return nil
}

func (*socketRepo) ListRuntimeProfiles(context.Context, uint, string) ([]domainagent.RuntimeProfile, error) {
	return nil, nil
}
func (*socketRepo) ListWorkspaces(context.Context, uint, string) ([]domainagent.Workspace, error) {
	return nil, nil
}
func (*socketRepo) CreateArtifact(context.Context, uint, string, string, *domainagent.Artifact) (*domainagent.Artifact, error) {
	return nil, repository.ErrNotFound
}
func (*socketRepo) ListArtifactsForCommand(context.Context, uint, uint, []string) ([]domainagent.Artifact, error) {
	return nil, nil
}
func (*socketRepo) GetArtifactForCommand(context.Context, string, string) (*domainagent.Artifact, *domainagent.Command, error) {
	return nil, nil, repository.ErrNotFound
}
func (*socketRepo) QueueResourceRefresh(context.Context, string, string, uint, string, string, string, string, *domainagent.Command, time.Time) (*domainagent.Command, error) {
	return nil, repository.ErrNotFound
}
func (*socketRepo) GetResourceSnapshot(context.Context, uint, string, string, string, string) (*domainagent.ResourceSnapshot, error) {
	return nil, repository.ErrNotFound
}
func (*socketRepo) QueueTurnInterrupt(context.Context, string, string, uint, string, *domainagent.Command, time.Time) (*domainagent.Command, error) {
	return nil, repository.ErrNotFound
}
func (*socketRepo) StartThread(context.Context, string, string, *domainagent.Thread, *domainagent.Turn, *domainagent.Command, time.Time) (*domainagent.Thread, *domainagent.Turn, error) {
	return nil, nil, repository.ErrNotFound
}
func (*socketRepo) GetThreadByConversation(context.Context, uint, uint) (*domainagent.Thread, error) {
	return nil, repository.ErrNotFound
}
func (*socketRepo) StartTurn(context.Context, string, string, *domainagent.Turn, *domainagent.Command, time.Time) (*domainagent.Turn, error) {
	return nil, repository.ErrNotFound
}
func (*socketRepo) GetTurnByRunID(context.Context, uint, string) (*domainagent.Turn, error) {
	return nil, repository.ErrNotFound
}
func (*socketRepo) ResolveExecutionTarget(context.Context, uint, string, string, string, time.Time) (string, error) {
	return "", repository.ErrNotFound
}
func (*socketRepo) ListInteractions(context.Context, uint, string, string, int) ([]domainagent.Interaction, error) {
	return nil, nil
}
func (*socketRepo) RespondInteraction(context.Context, string, string, uint, string, json.RawMessage, *domainagent.Command, time.Time) (*domainagent.Interaction, error) {
	return nil, repository.ErrNotFound
}

type socketRuntimeAuth struct {
	userPublicID string
	key          string
}

func (a *socketRuntimeAuth) RuntimeUser(context.Context, uint) (string, int64, error) {
	return a.userPublicID, 17, nil
}

func (a *socketRuntimeAuth) RuntimeUserByPublicID(context.Context, string) (uint, string, int64, error) {
	return 7, a.userPublicID, 17, nil
}

func (a *socketRuntimeAuth) MatchRuntimeProof(_ context.Context, _ uint, remoteUserID int64, challenge, proof []byte) (int64, string, error) {
	mac := hmac.New(sha256.New, []byte(a.key))
	_, _ = mac.Write(challenge)
	if remoteUserID != 17 || !hmac.Equal(mac.Sum(nil), proof) {
		return 0, "", repository.ErrNotFound
	}
	return 31, strings.Repeat("a", 64), nil
}
