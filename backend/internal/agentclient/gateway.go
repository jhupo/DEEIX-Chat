package agentclient

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/websocket"
)

type RuntimeStatus struct {
	Version      int    `json:"version"`
	PID          int    `json:"pid"`
	State        string `json:"state"`
	DeviceID     string `json:"deviceId"`
	CodexVersion string `json:"codexVersion"`
	UpdatedAt    string `json:"updatedAt"`
	LastError    string `json:"lastError,omitempty"`
}

type Gateway struct {
	ctx             context.Context
	config          Config
	dataDir         string
	identity        *DeviceIdentity
	state           *StateStore
	cloud           *CloudClient
	adapter         *CodexAdapter
	agentVersion    string
	logger          *log.Logger
	wake            chan struct{}
	commands        chan queuedCommand
	activeMu        sync.Mutex
	active          map[string]bool
	registerMu      sync.Mutex
	workspaceMu     sync.RWMutex
	workspaces      map[string]Workspace
	socketReady     bool
	updateBridgeSeq atomic.Uint64
}

type queuedCommand struct {
	ID     string
	Record commandRecord
}

func RunGateway(ctx context.Context, dataDir string, logger *log.Logger, agentVersion string) error {
	if hasPendingUpdate(dataDir) {
		return ErrUpdateScheduled
	}
	config, err := LoadConfig(filepath.Join(dataDir, "config.json"))
	if err != nil {
		return err
	}
	identity, err := LoadIdentity(filepath.Join(dataDir, "device-identity.json"))
	if err != nil {
		return err
	}
	state, err := OpenStateStore(filepath.Join(dataDir, "state.json"))
	if err != nil {
		return err
	}
	gateway := &Gateway{
		ctx: ctx, config: config, dataDir: dataDir, identity: identity, state: state, cloud: NewCloudClient(config.CloudURL), logger: logger,
		agentVersion: strings.TrimSpace(agentVersion),
		wake:         make(chan struct{}, 1), commands: make(chan queuedCommand, 128), active: make(map[string]bool), workspaces: make(map[string]Workspace),
	}
	for _, workspace := range config.Workspaces {
		gateway.workspaces[workspace.WorkspaceID] = workspace
	}
	adapter, err := StartCodexAdapter(ctx, config, state, logger.Writer(), func(event json.RawMessage) error {
		if _, appendErr := state.AppendEvent(event); appendErr != nil {
			return appendErr
		}
		gateway.signalWake()
		return nil
	})
	if err != nil {
		return err
	}
	discoveryContext, cancel := context.WithTimeout(ctx, 30*time.Second)
	discoveredWorkspaces, err := adapter.DiscoverWorkspaces(discoveryContext)
	cancel()
	if err != nil {
		_ = adapter.Close()
		return err
	}
	config.Workspaces = discoveredWorkspaces
	gateway.config = config
	gateway.workspaces = make(map[string]Workspace, len(discoveredWorkspaces))
	for _, workspace := range discoveredWorkspaces {
		gateway.workspaces[workspace.WorkspaceID] = workspace
	}
	adapter.replaceWorkspaces(discoveredWorkspaces)
	gateway.adapter = adapter
	defer adapter.Close()
	defer func() { _ = gateway.writeStatus("stopped", "") }()
	go gateway.commandWorker(ctx)
	if err = gateway.recoverCommands(); err != nil {
		return err
	}
	delay := time.Second
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err = gateway.writeStatus("connecting", ""); err != nil {
			logger.Printf("write status: %v", err)
		}
		connectContext, cancel := context.WithTimeout(ctx, 30*time.Second)
		gateway.socketReady = false
		token, tokenErr := gateway.cloud.ConnectionToken(connectContext, config, identity)
		cancel()
		if tokenErr == nil {
			tokenErr = gateway.runSocket(ctx, token)
		}
		if errors.Is(tokenErr, ErrUpdateScheduled) {
			return ErrUpdateScheduled
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		select {
		case <-adapter.Done():
			return errors.New("Codex app-server exited")
		default:
		}
		message := publicMessage(tokenErr)
		logger.Printf("gateway connection failed: %s", message)
		_ = gateway.writeStatus("reconnecting", message)
		if gateway.socketReady {
			delay = time.Second
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(jitterReconnectDelay(delay)):
		}
		if delay < 30*time.Second {
			delay *= 2
			if delay > 30*time.Second {
				delay = 30 * time.Second
			}
		}
	}
}

func (gateway *Gateway) recoverCommands() error {
	for id, record := range gateway.state.RecoverableCommands() {
		command, err := parseAgentCommand(record.Command)
		if err != nil {
			return err
		}
		if record.State == "started" && !safeToReplay(command) {
			outcome := errorOutcome("outcome_unknown", "the local runtime stopped while this command was executing")
			if _, err = gateway.state.AppendTerminal(id, outcome); err != nil {
				return err
			}
			continue
		}
		gateway.enqueue(id, record, concurrentCommand(command.Kind))
	}
	return nil
}

func (gateway *Gateway) runSocket(ctx context.Context, token string) error {
	config, err := bridgeWebSocketConfig(gateway.config.CloudURL, token, false)
	if err != nil {
		return err
	}
	dialContext, cancel := context.WithTimeout(ctx, 10*time.Second)
	connection, err := config.DialContext(dialContext)
	cancel()
	if err != nil {
		return err
	}
	defer connection.Close()
	connection.MaxPayloadBytes = bridgeMaxPayload
	_ = connection.SetDeadline(time.Now().Add(10 * time.Second))
	writer := &socketWriter{connection: connection}
	ackServer, ackBridge := gateway.state.Cursors()
	if err = writer.send(bridgeFrame{Version: bridgeVersion, Type: "hello", ProfileID: gateway.config.ProfileID, AckServerSeq: ackServer, AckBridgeSeq: ackBridge}); err != nil {
		return err
	}
	var challenge bridgeFrame
	if err = receiveBridgeFrame(connection, &challenge); err != nil {
		return errors.New("gateway runtime challenge is invalid")
	}
	if authErr := bridgeAuthError(challenge); authErr != nil {
		return authErr
	}
	if challenge.Version != bridgeVersion || challenge.Type != "auth.challenge" || challenge.ProfileID != gateway.config.ProfileID || !validPublicID(challenge.ChallengeID, "agp") || !strings.HasPrefix(challenge.Challenge, "deeix-runtime-auth-proof-v1\n") {
		return errors.New("gateway runtime challenge is invalid")
	}
	proofDeadline, err := runtimeProofDeadline(challenge.ExpiresAt, time.Now())
	if err != nil {
		return err
	}
	_ = connection.SetDeadline(proofDeadline)
	proofContext, cancel := context.WithTimeout(ctx, 30*time.Second)
	proof, err := gateway.adapter.ProveRuntimeAuth(proofContext, challenge.Challenge)
	cancel()
	if err != nil {
		_ = writer.send(bridgeFrame{
			Version: bridgeVersion, Type: "auth.error", ProfileID: gateway.config.ProfileID,
			ChallengeID: challenge.ChallengeID, ErrorCode: "runtime_proof_unavailable", ErrorMessage: publicMessage(err),
		})
		return err
	}
	gateway.workspaceMu.RLock()
	registeredWorkspaces := append([]Workspace(nil), gateway.config.Workspaces...)
	gateway.workspaceMu.RUnlock()
	workspaces := make([]bridgeWorkspace, 0, len(registeredWorkspaces))
	for _, workspace := range registeredWorkspaces {
		workspaces = append(workspaces, bridgeWorkspace{WorkspaceID: workspace.WorkspaceID, Name: workspace.Name})
	}
	manifest := gateway.adapter.Manifest()
	if validAgentVersion(gateway.agentVersion) {
		manifest.AgentVersion = gateway.agentVersion
	}
	if err = writer.send(bridgeFrame{Version: bridgeVersion, Type: "auth.proof", ProfileID: gateway.config.ProfileID, ChallengeID: challenge.ChallengeID, Proof: proof, Workspaces: workspaces, Manifest: &manifest}); err != nil {
		return err
	}
	var ready bridgeFrame
	if err = receiveBridgeFrame(connection, &ready); err != nil {
		return errors.New("gateway runtime authorization failed")
	}
	if authErr := bridgeAuthError(ready); authErr != nil {
		if diagnostic := gateway.adapter.RuntimeAuthDiagnostic(); diagnostic != "" {
			return fmt.Errorf("%w; local Codex auth: %s", authErr, diagnostic)
		}
		return authErr
	}
	if ready.Version != bridgeVersion || ready.Type != "auth.ready" || ready.ProfileID != gateway.config.ProfileID {
		return errors.New("gateway runtime authorization failed")
	}
	var welcome bridgeFrame
	if err = receiveBridgeFrame(connection, &welcome); err != nil || welcome.Version != bridgeVersion || welcome.Type != "welcome" || welcome.DeviceID != gateway.config.DeviceID || welcome.HeartbeatSeconds < 5 || welcome.HeartbeatSeconds > 300 {
		return errors.New("gateway welcome frame is invalid")
	}
	_ = connection.SetDeadline(time.Time{})
	if err = gateway.state.AcknowledgeServer(welcome.AckServerSeq); err != nil {
		return err
	}
	if welcome.AckBridgeSeq > ackBridge {
		if err = gateway.state.AcknowledgeBridge(welcome.AckBridgeSeq); err != nil {
			return err
		}
	}
	if updateSeq := gateway.updateBridgeSeq.Load(); updateSeq > 0 && welcome.AckBridgeSeq >= updateSeq {
		return ErrUpdateScheduled
	}
	_ = gateway.writeStatus("connected", "")
	gateway.logger.Printf("connected device %s", gateway.config.DeviceID)
	frames := make(chan socketRead, 1)
	go readSocket(connection, frames)
	heartbeat := time.NewTicker(time.Duration(welcome.HeartbeatSeconds) * time.Second / 2)
	defer heartbeat.Stop()
	readTimeout := time.Duration(welcome.HeartbeatSeconds*2) * time.Second
	_ = connection.SetReadDeadline(time.Now().Add(readTimeout))
	sentThrough := welcome.AckBridgeSeq
	if sentThrough < ackBridge {
		sentThrough = ackBridge
	}
	if sentThrough, err = gateway.flushOutgoing(writer, sentThrough); err != nil {
		return err
	}
	gateway.socketReady = true
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-gateway.adapter.Done():
			return errors.New("Codex app-server exited")
		case <-heartbeat.C:
			if err = writer.send(bridgeFrame{Version: bridgeVersion, Type: "ping"}); err != nil {
				return err
			}
			_ = gateway.writeStatus("connected", "")
		case <-gateway.wake:
			_, acknowledged := gateway.state.Cursors()
			if sentThrough != acknowledged {
				continue
			}
			if sentThrough, err = gateway.flushOutgoing(writer, sentThrough); err != nil {
				return err
			}
		case read := <-frames:
			if read.err != nil {
				return read.err
			}
			frame := read.frame
			_ = connection.SetReadDeadline(time.Now().Add(readTimeout))
			if frame.Version != bridgeVersion {
				return errors.New("gateway frame version is invalid")
			}
			switch frame.Type {
			case "pong":
			case "ack.bridge":
				if frame.AckBridgeSeq == 0 {
					return errors.New("gateway bridge acknowledgment is invalid")
				}
				if err = gateway.state.AcknowledgeBridge(frame.AckBridgeSeq); err != nil {
					return err
				}
				if updateSeq := gateway.updateBridgeSeq.Load(); updateSeq > 0 && frame.AckBridgeSeq >= updateSeq {
					return ErrUpdateScheduled
				}
				if sentThrough == frame.AckBridgeSeq {
					if sentThrough, err = gateway.flushOutgoing(writer, sentThrough); err != nil {
						return err
					}
				}
			case "command":
				if frame.ServerSeq == 0 || !validRef(frame.CommandID, 256) || frame.Artifacts == nil || len(*frame.Artifacts) > 16 {
					return errors.New("gateway command frame is invalid")
				}
				command, parseErr := parseAgentCommand(frame.Command)
				if parseErr != nil || command.DeviceID != gateway.config.DeviceID {
					return errors.New("gateway command payload is invalid")
				}
				record, created, receiveErr := gateway.state.Receive(frame.ServerSeq, frame.CommandID, frame.Command, *frame.Artifacts)
				if receiveErr != nil {
					return receiveErr
				}
				if err = writer.send(bridgeFrame{Version: bridgeVersion, Type: "ack.server", AckServerSeq: frame.ServerSeq}); err != nil {
					return err
				}
				if created {
					gateway.enqueue(frame.CommandID, record, concurrentCommand(command.Kind))
				}
			default:
				return fmt.Errorf("unsupported gateway frame: %s", frame.Type)
			}
		}
	}
}

func bridgeWebSocketConfig(cloudURL, token string, probe bool) (*websocket.Config, error) {
	endpoint, err := url.Parse(cloudURL)
	if err != nil {
		return nil, err
	}
	origin := endpoint.Scheme + "://" + endpoint.Host
	if endpoint.Scheme == "https" {
		endpoint.Scheme = "wss"
	} else {
		endpoint.Scheme = "ws"
	}
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/api/v1/agent/bridge/connect"
	endpoint.RawPath = ""
	endpoint.RawQuery = ""
	if probe {
		query := endpoint.Query()
		query.Set("probe", "1")
		endpoint.RawQuery = query.Encode()
	}
	config, err := websocket.NewConfig(endpoint.String(), origin)
	if err != nil {
		return nil, err
	}
	config.Protocol = []string{bridgeProtocol, "deeix.auth." + token}
	config.Dialer = &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	return config, nil
}

func probeBridgeConnection(ctx context.Context, cloudURL, deviceID, token string) error {
	config, err := bridgeWebSocketConfig(cloudURL, token, true)
	if err != nil {
		return err
	}
	dialContext, cancel := context.WithTimeout(ctx, 10*time.Second)
	connection, err := config.DialContext(dialContext)
	cancel()
	if err != nil {
		return err
	}
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(10 * time.Second))
	var ready bridgeFrame
	if err = receiveBridgeFrame(connection, &ready); err != nil {
		return err
	}
	if ready.Version != bridgeVersion || ready.Type != "probe.ready" || ready.DeviceID != deviceID {
		return errors.New("gateway websocket probe response is invalid")
	}
	return nil
}

func jitterReconnectDelay(base time.Duration) time.Duration {
	if base <= 0 {
		return 0
	}
	var value [1]byte
	if _, err := rand.Read(value[:]); err != nil {
		return base
	}
	percent := 80 + int(value[0])*41/256
	return base * time.Duration(percent) / 100
}

func runtimeProofDeadline(value string, now time.Time) (time.Time, error) {
	expiresAt, err := time.Parse(time.RFC3339Nano, value)
	if err != nil || !expiresAt.After(now) || expiresAt.After(now.Add(2*time.Minute)) {
		return time.Time{}, errors.New("gateway runtime challenge expiry is invalid")
	}
	return expiresAt, nil
}

func (gateway *Gateway) commandWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case item := <-gateway.commands:
			gateway.executeCommand(ctx, item)
		}
	}
}

func (gateway *Gateway) enqueue(id string, record commandRecord, concurrent bool) {
	gateway.activeMu.Lock()
	if gateway.active[id] {
		gateway.activeMu.Unlock()
		return
	}
	gateway.active[id] = true
	gateway.activeMu.Unlock()
	item := queuedCommand{ID: id, Record: record}
	if concurrent {
		go gateway.executeCommand(gateway.ctx, item)
		return
	}
	gateway.commands <- item
}

func (gateway *Gateway) executeCommand(parent context.Context, item queuedCommand) {
	defer func() {
		gateway.activeMu.Lock()
		delete(gateway.active, item.ID)
		gateway.activeMu.Unlock()
	}()
	command, err := parseAgentCommand(item.Record.Command)
	if err == nil {
		err = gateway.state.MarkStarted(item.ID)
	}
	timeout := time.Minute
	if command.Kind == "resource.refresh" {
		timeout = 30 * time.Second
	} else if command.Kind == "turn.start" || command.Kind == "review.start" {
		timeout = 10 * time.Minute
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	var result map[string]any
	updatePrepared := false
	if err == nil {
		gateway.workspaceMu.RLock()
		registeredWorkspaces := make(map[string]Workspace, len(gateway.workspaces))
		for id, workspace := range gateway.workspaces {
			registeredWorkspaces[id] = workspace
		}
		gateway.workspaceMu.RUnlock()
		var artifacts map[string]LocalArtifact
		downloadErr := runAsConfiguredUser(func() error {
			var artifactErr error
			artifacts, artifactErr = gateway.cloud.DownloadArtifacts(ctx, item.ID, command, item.Record.Artifacts, registeredWorkspaces)
			return artifactErr
		})
		if downloadErr != nil {
			err = downloadErr
		} else if command.Kind == "agent.update" {
			err = preparePendingUpdate(gateway.dataDir, command.TargetVersion)
			if err == nil {
				updatePrepared = true
				result = map[string]any{"kind": "update-scheduled", "targetVersion": command.TargetVersion}
			}
		} else if command.Kind == "workspace.register" {
			result, err = gateway.registerWorkspace(command)
		} else {
			result, err = gateway.adapter.Execute(ctx, command, artifacts)
		}
	}
	var outcome json.RawMessage
	if err != nil {
		outcome = errorOutcome(errorCode(err), publicMessage(err))
	} else {
		outcome, _ = json.Marshal(map[string]any{"kind": "result", "result": result})
	}
	outgoing, appendErr := gateway.state.AppendTerminal(item.ID, outcome)
	if appendErr != nil {
		if updatePrepared {
			clearPendingUpdate(gateway.dataDir)
		}
		gateway.logger.Printf("persist command %s terminal outcome: %v", item.ID, appendErr)
		return
	}
	if updatePrepared {
		gateway.updateBridgeSeq.Store(outgoing.BridgeSeq)
	}
	gateway.signalWake()
}

func (gateway *Gateway) registerWorkspace(command AgentCommand) (map[string]any, error) {
	gateway.registerMu.Lock()
	defer gateway.registerMu.Unlock()
	path := filepath.Clean(strings.TrimSpace(command.Path))
	volumeRoot := filepath.VolumeName(path) + string(filepath.Separator)
	if path == "." || !filepath.IsAbs(path) || path == volumeRoot || len(path) > 4096 ||
		strings.ContainsRune(path, 0) || strings.HasPrefix(path, `\\`) {
		return nil, errors.New("workspace path must be absolute")
	}
	configPath := filepath.Join(gateway.dataDir, "config.json")
	persisted, err := LoadConfig(configPath)
	if err != nil {
		return nil, err
	}
	created := false
	committed := false
	defer func() {
		if created && !committed {
			_ = runAsConfiguredUser(func() error { return os.Remove(path) })
		}
	}()
	var workspace Workspace
	err = runAsConfiguredUser(func() error {
		if command.Create {
			if len(persisted.Workspaces) >= 128 {
				return errors.New("workspace limit reached")
			}
			parent := filepath.Dir(path)
			resolvedParent, resolveErr := filepath.EvalSymlinks(parent)
			if resolveErr != nil {
				return errors.New("workspace parent directory does not exist")
			}
			info, statErr := os.Stat(resolvedParent)
			if statErr != nil || !info.IsDir() {
				return errors.New("workspace parent directory does not exist")
			}
			path = filepath.Join(resolvedParent, filepath.Base(path))
			if mkdirErr := os.Mkdir(path, 0o755); mkdirErr != nil && !errors.Is(mkdirErr, os.ErrExist) {
				return fmt.Errorf("create workspace directory: %w", mkdirErr)
			} else {
				created = mkdirErr == nil
			}
		}
		selected, selectErr := CanonicalWorkspace(path)
		if selectErr != nil {
			return selectErr
		}
		dataRoot, dataErr := filepath.Abs(gateway.dataDir)
		if dataErr == nil {
			if resolved, resolveErr := filepath.EvalSymlinks(dataRoot); resolveErr == nil {
				dataRoot = resolved
			}
		}
		if dataErr != nil || pathWithin(dataRoot, selected.Root) || pathWithin(selected.Root, dataRoot) {
			return errors.New("workspace path is reserved for Agent data")
		}
		probe, probeErr := os.CreateTemp(selected.Root, ".deeix-write-probe-*")
		if probeErr != nil {
			return errors.New("workspace directory is not writable by the configured user")
		}
		probePath := probe.Name()
		if closeErr := probe.Close(); closeErr != nil {
			_ = os.Remove(probePath)
			return closeErr
		}
		if removeErr := os.Remove(probePath); removeErr != nil {
			return removeErr
		}
		workspace, selectErr = codexProjectWorkspace(selected.Root)
		if selectErr != nil {
			workspace = selected
			workspace.SessionRoots = []string{workspace.Root}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	known := false
	for _, current := range persisted.Workspaces {
		if current.WorkspaceID == workspace.WorkspaceID {
			known = true
			break
		}
	}
	if !known && len(persisted.Workspaces) >= 128 {
		return nil, errors.New("workspace limit reached")
	}
	upsertWorkspace(&persisted.Workspaces, workspace)
	if err = SaveConfig(configPath, persisted); err != nil {
		return nil, err
	}
	committed = true

	gateway.workspaceMu.Lock()
	byID := make(map[string]Workspace, len(gateway.config.Workspaces)+1)
	for _, current := range gateway.config.Workspaces {
		mergeWorkspace(byID, current)
	}
	mergeWorkspace(byID, workspace)
	registered := make([]Workspace, 0, len(byID))
	for _, current := range byID {
		registered = append(registered, current)
	}
	sort.Slice(registered, func(i, j int) bool {
		return registered[i].Name < registered[j].Name
	})
	gateway.config.Workspaces = registered
	gateway.workspaces = byID
	gateway.adapter.replaceWorkspaces(registered)
	gateway.workspaceMu.Unlock()

	return map[string]any{"kind": "accepted", "workspaceId": workspace.WorkspaceID, "name": workspace.Name}, nil
}

func concurrentCommand(kind string) bool {
	return kind == "interaction.respond" || kind == "workspace.register"
}

func (gateway *Gateway) flushOutgoing(writer *socketWriter, after uint64) (uint64, error) {
	pending := gateway.state.PendingOutgoing(after)
	if len(pending) == 0 {
		return after, nil
	}
	outgoing := pending[0]
	frame := bridgeFrame{Version: bridgeVersion, Type: outgoing.Type, BridgeSeq: outgoing.BridgeSeq, ServerSeq: outgoing.ServerSeq, CommandID: outgoing.CommandID, Outcome: outgoing.Outcome, Event: outgoing.Event}
	if err := writer.send(frame); err != nil {
		return after, err
	}
	return outgoing.BridgeSeq, nil
}

func (gateway *Gateway) signalWake() {
	select {
	case gateway.wake <- struct{}{}:
	default:
	}
}

func bridgeAuthError(frame bridgeFrame) error {
	if frame.Version != bridgeVersion || frame.Type != "auth.error" {
		return nil
	}
	message := strings.TrimSpace(frame.ErrorMessage)
	if message == "" {
		message = "gateway runtime authorization failed"
	}
	if code := strings.TrimSpace(frame.ErrorCode); code != "" {
		message += " (" + code + ")"
	}
	return errors.New(message)
}

func (gateway *Gateway) writeStatus(state, lastError string) error {
	status := RuntimeStatus{Version: 1, PID: os.Getpid(), State: state, DeviceID: gateway.config.DeviceID, CodexVersion: gateway.adapter.version, UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano), LastError: lastError}
	data, _ := json.MarshalIndent(status, "", "  ")
	return writeFileAtomic(filepath.Join(gateway.dataDir, "runtime-status.json"), append(data, '\n'), 0o600)
}

type socketWriter struct {
	mu         sync.Mutex
	connection *websocket.Conn
}

func (writer *socketWriter) send(frame bridgeFrame) error {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	return websocket.JSON.Send(writer.connection, frame)
}

type socketRead struct {
	frame bridgeFrame
	err   error
}

func readSocket(connection *websocket.Conn, output chan<- socketRead) {
	for {
		var frame bridgeFrame
		err := receiveBridgeFrame(connection, &frame)
		output <- socketRead{frame: frame, err: err}
		if err != nil {
			return
		}
	}
}

func receiveBridgeFrame(connection *websocket.Conn, frame *bridgeFrame) error {
	var data []byte
	if err := websocket.Message.Receive(connection, &data); err != nil {
		return err
	}
	if len(data) == 0 || len(data) > bridgeMaxPayload {
		return errors.New("gateway frame size is invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(frame); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("gateway frame contains trailing data")
	}
	return nil
}

func safeToReplay(command AgentCommand) bool {
	if command.Kind == "agent.update" || command.Kind == "resource.refresh" || command.Kind == "thread.rename" || command.Kind == "thread.metadata.update" || command.Kind == "turn.interrupt" {
		return true
	}
	return command.Kind == "thread.lifecycle" && command.Action != "fork"
}

func errorOutcome(code, message string) json.RawMessage {
	value, _ := json.Marshal(map[string]any{"kind": "error", "error": map[string]any{"code": code, "message": message}})
	return value
}

func errorCode(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "artifact") {
		return "artifact_error"
	}
	if strings.Contains(message, "source reference") {
		return "source_not_found"
	}
	return "provider_error"
}

func publicMessage(err error) string {
	if err == nil {
		return "gateway connection ended"
	}
	message := strings.Map(func(character rune) rune {
		if character < 32 {
			return ' '
		}
		return character
	}, err.Error())
	if len(message) > 1024 {
		message = message[:1024]
	}
	return message
}
