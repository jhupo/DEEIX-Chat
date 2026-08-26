package agentclient

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/shared/agentprotocol"
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

const (
	artifactStagingTimeout        = 35 * time.Second
	gatewayOutgoingBatchSize      = 32
	historyImageUploadConcurrency = 4
)

type Gateway struct {
	ctx              context.Context
	config           Config
	dataDir          string
	identity         *DeviceIdentity
	state            *StateStore
	cloud            *CloudClient
	adapter          *CodexAdapter
	agentVersion     string
	logger           *log.Logger
	wake             chan struct{}
	workspaceUpdates chan []Workspace
	commands         chan queuedCommand
	activeMu         sync.Mutex
	active           map[string]bool
	registerMu       sync.Mutex
	workspaceMu      sync.RWMutex
	workspaces       map[string]Workspace
	socketReady      bool
	updateBridgeSeq  atomic.Uint64
}

type queuedCommand struct {
	ID     string
	Record commandRecord
}

func RunGateway(ctx context.Context, dataDir string, logger *log.Logger, agentVersion string) error {
	delay := time.Second
	for {
		startedAt := time.Now()
		err := runGatewayRuntime(ctx, dataDir, logger, agentVersion)
		if errors.Is(err, ErrUpdateScheduled) || ctx.Err() != nil {
			return err
		}
		if time.Since(startedAt) >= time.Minute {
			delay = time.Second
		}
		message := publicMessage(err)
		logger.Printf("agent runtime restarting: %s", message)
		_ = writeSupervisorStatus(dataDir, "restarting", message)
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

func runGatewayRuntime(ctx context.Context, dataDir string, logger *log.Logger, agentVersion string) error {
	if hasPendingUpdate(dataDir) {
		return ErrUpdateScheduled
	}
	runtimeContext, cancelRuntime := context.WithCancel(ctx)
	defer cancelRuntime()
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
		ctx: runtimeContext, config: config, dataDir: dataDir, identity: identity, state: state, cloud: NewCloudClient(config.CloudURL), logger: logger,
		agentVersion: strings.TrimSpace(agentVersion),
		wake:         make(chan struct{}, 1), workspaceUpdates: make(chan []Workspace, 1),
		commands: make(chan queuedCommand, 128), active: make(map[string]bool), workspaces: make(map[string]Workspace),
	}
	for _, workspace := range config.Workspaces {
		if !workspace.Excluded {
			gateway.workspaces[workspace.WorkspaceID] = workspace
		}
	}
	adapter, err := StartCodexAdapter(runtimeContext, config, state, logger.Writer(), func(event json.RawMessage) error {
		if _, appendErr := state.AppendEvent(event); appendErr != nil {
			return appendErr
		}
		gateway.signalWake()
		return nil
	})
	if err != nil {
		return err
	}
	gateway.adapter = adapter
	defer adapter.Close()
	defer func() { _ = gateway.writeStatus("stopped", "") }()
	go gateway.commandWorker(runtimeContext)
	go gateway.refreshWorkspaceLoop(runtimeContext)
	go gateway.sessionSnapshotLoop(runtimeContext)
	if err = gateway.recoverCommands(); err != nil {
		return err
	}
	delay := time.Second
	for {
		if runtimeContext.Err() != nil {
			return runtimeContext.Err()
		}
		if err = gateway.writeStatus("connecting", ""); err != nil {
			logger.Printf("write status: %v", err)
		}
		connectContext, cancel := context.WithTimeout(runtimeContext, 30*time.Second)
		gateway.socketReady = false
		token, tokenErr := gateway.cloud.ConnectionToken(connectContext, config, identity)
		cancel()
		if tokenErr == nil {
			tokenErr = gateway.runSocket(runtimeContext, token)
		}
		if errors.Is(tokenErr, ErrUpdateScheduled) {
			return ErrUpdateScheduled
		}
		if runtimeContext.Err() != nil {
			return runtimeContext.Err()
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
		case <-runtimeContext.Done():
			return runtimeContext.Err()
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

func (gateway *Gateway) refreshWorkspaceLoop(ctx context.Context) {
	refresh := func() {
		if err := gateway.refreshWorkspaces(ctx); err != nil && ctx.Err() == nil {
			gateway.logger.Printf("refresh Codex workspaces: %s", publicMessage(err))
		}
	}
	refresh()
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			refresh()
		}
	}
}

func (gateway *Gateway) sessionSnapshotLoop(ctx context.Context) {
	revisions := make(map[string]string)
	syncSnapshots := func() {
		scanContext, cancel := context.WithTimeout(ctx, 30*time.Second)
		snapshots, err := gateway.adapter.SessionSnapshots(scanContext)
		cancel()
		if err != nil {
			if ctx.Err() == nil {
				gateway.logger.Printf("refresh Codex sessions: %s", publicMessage(err))
			}
			return
		}
		appended := false
		for _, snapshot := range snapshots {
			if revisions[snapshot.WorkspaceID] == snapshot.Revision {
				continue
			}
			event, marshalErr := json.Marshal(map[string]any{
				"kind":       agentprotocol.SessionSnapshotEventKind,
				"occurredAt": time.Now().UTC().Format(time.RFC3339Nano),
				"payload":    snapshot,
			})
			if marshalErr != nil || len(event) > agentprotocol.MaxProviderEventBytes {
				gateway.logger.Printf("encode Codex session snapshot workspace=%s: event is invalid", snapshot.WorkspaceID)
				continue
			}
			if _, appendErr := gateway.state.AppendEvent(event); appendErr != nil {
				gateway.logger.Printf("persist Codex session snapshot workspace=%s: %s", snapshot.WorkspaceID, publicMessage(appendErr))
				continue
			}
			revisions[snapshot.WorkspaceID] = snapshot.Revision
			appended = true
		}
		if appended {
			gateway.signalWake()
		}
	}
	syncSnapshots()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			syncSnapshots()
		}
	}
}

func (gateway *Gateway) refreshWorkspaces(ctx context.Context) error {
	gateway.registerMu.Lock()
	defer gateway.registerMu.Unlock()
	discoveryContext, cancel := context.WithTimeout(ctx, 2*time.Minute)
	discovered, err := gateway.adapter.DiscoverWorkspaces(discoveryContext)
	cancel()
	if err != nil {
		return err
	}
	gateway.workspaceMu.RLock()
	persisted := persistWorkspaceExclusions(discovered, gateway.config.Workspaces)
	unchanged := equalWorkspaces(gateway.config.Workspaces, persisted)
	gateway.workspaceMu.RUnlock()
	if unchanged {
		return nil
	}
	gateway.workspaceMu.Lock()
	updated := gateway.config
	updated.Workspaces = persisted
	if err = SaveConfig(filepath.Join(gateway.dataDir, "config.json"), updated); err != nil {
		gateway.workspaceMu.Unlock()
		return err
	}
	byID := make(map[string]Workspace, len(discovered))
	for _, workspace := range discovered {
		byID[workspace.WorkspaceID] = workspace
	}
	gateway.config = updated
	gateway.workspaces = byID
	gateway.adapter.replaceWorkspaces(persisted)
	gateway.workspaceMu.Unlock()
	gateway.signalWorkspaceUpdate(discovered)
	return nil
}

func persistWorkspaceExclusions(visible, current []Workspace) []Workspace {
	result := append([]Workspace(nil), visible...)
	for _, workspace := range current {
		if workspace.Excluded {
			result = append(result, workspace)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].WorkspaceID < result[j].WorkspaceID
	})
	return result
}

func equalWorkspaces(left, right []Workspace) bool {
	if len(left) != len(right) {
		return false
	}
	leftByID := make(map[string]Workspace, len(left))
	for _, workspace := range left {
		leftByID[workspace.WorkspaceID] = workspace
	}
	for _, workspace := range right {
		current, ok := leftByID[workspace.WorkspaceID]
		if !ok || current.Name != workspace.Name || current.Root != workspace.Root || current.Registered != workspace.Registered || current.Excluded != workspace.Excluded || current.Hidden != workspace.Hidden || !equalStrings(current.SessionRoots, workspace.SessionRoots) || !equalThreadIDs(current.ThreadIDs, workspace.ThreadIDs) {
			return false
		}
	}
	return true
}

func equalThreadIDs(left, right map[string]struct{}) bool {
	if len(left) != len(right) {
		return false
	}
	for id := range left {
		if _, ok := right[id]; !ok {
			return false
		}
	}
	return true
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func (gateway *Gateway) runSocket(ctx context.Context, token string) error {
	config, err := bridgeWebSocketConfig(gateway.config.CloudURL, token, false)
	if err != nil {
		return err
	}
	dialContext, cancel := context.WithTimeout(ctx, 10*time.Second)
	connection, err := dialBridgeWebSocket(dialContext, config)
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
	registeredWorkspaces := make([]Workspace, 0, len(gateway.workspaces))
	for _, workspace := range gateway.workspaces {
		registeredWorkspaces = append(registeredWorkspaces, workspace)
	}
	gateway.workspaceMu.RUnlock()
	sort.Slice(registeredWorkspaces, func(i, j int) bool { return registeredWorkspaces[i].WorkspaceID < registeredWorkspaces[j].WorkspaceID })
	workspaces := make([]bridgeWorkspace, 0, len(registeredWorkspaces))
	for _, workspace := range registeredWorkspaces {
		workspaces = append(workspaces, bridgeWorkspace{WorkspaceID: workspace.WorkspaceID, Name: workspace.Name, Managed: workspace.Registered, Hidden: workspace.Hidden, Revision: workspaceRevision(workspace)})
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
	if err = validateRuntimeLeaseExpiry(ready.LeaseExpiresAt, time.Now()); err != nil {
		return err
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
		case workspaces := <-gateway.workspaceUpdates:
			registrations := make([]bridgeWorkspace, 0, len(workspaces))
			for _, workspace := range workspaces {
				registrations = append(registrations, bridgeWorkspace{WorkspaceID: workspace.WorkspaceID, Name: workspace.Name, Managed: workspace.Registered, Hidden: workspace.Hidden, Revision: workspaceRevision(workspace)})
			}
			if err = writer.send(bridgeFrame{Version: bridgeVersion, Type: "workspaces.sync", Workspaces: registrations}); err != nil {
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
				if err = validateRuntimeLeaseExpiry(frame.LeaseExpiresAt, time.Now()); err != nil {
					return err
				}
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
				if err = validateArtifactGrants(*frame.Artifacts); err != nil {
					return err
				}
				if len(*frame.Artifacts) > 0 {
					stageCtx, stageCancel := context.WithTimeout(ctx, artifactStagingTimeout)
					_, stageErr := gateway.downloadCommandArtifacts(stageCtx, frame.CommandID, command, *frame.Artifacts)
					stageCancel()
					if stageErr != nil {
						return fmt.Errorf("stage command artifacts: %w", stageErr)
					}
				}
				record, created, receiveErr := gateway.state.Receive(frame.ServerSeq, frame.CommandID, frame.Command, *frame.Artifacts)
				if receiveErr != nil {
					return receiveErr
				}
				if created {
					gateway.enqueue(frame.CommandID, record, concurrentCommand(command.Kind))
				}
				if err = writer.send(bridgeFrame{Version: bridgeVersion, Type: "ack.server", AckServerSeq: frame.ServerSeq}); err != nil {
					return err
				}
			default:
				return fmt.Errorf("unsupported gateway frame: %s", frame.Type)
			}
		}
	}
}

func workspaceRevision(workspace Workspace) string {
	roots := append([]string(nil), workspace.SessionRoots...)
	sort.Strings(roots)
	threadIDs := make([]string, 0, len(workspace.ThreadIDs))
	for id := range workspace.ThreadIDs {
		threadIDs = append(threadIDs, id)
	}
	sort.Strings(threadIDs)
	payload, _ := json.Marshal(struct {
		Hidden    bool     `json:"hidden"`
		Roots     []string `json:"roots"`
		ThreadIDs []string `json:"threadIds"`
	}{Hidden: workspace.Hidden, Roots: roots, ThreadIDs: threadIDs})
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:12])
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
	return config, nil
}

func probeBridgeConnection(ctx context.Context, cloudURL, deviceID, token string) error {
	config, err := bridgeWebSocketConfig(cloudURL, token, true)
	if err != nil {
		return err
	}
	dialContext, cancel := context.WithTimeout(ctx, 10*time.Second)
	connection, err := dialBridgeWebSocket(dialContext, config)
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

func validateRuntimeLeaseExpiry(value string, now time.Time) error {
	expiresAt, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(value))
	if err != nil || expiresAt.Before(now.Add(2*time.Minute)) || expiresAt.After(now.Add(30*time.Minute)) {
		return errors.New("gateway runtime lease expiry is invalid")
	}
	return nil
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
	} else if command.Kind == "thread.read" {
		timeout = 2 * time.Minute
	} else if command.Kind == "turn.start" || command.Kind == "review.start" {
		timeout = 10 * time.Minute
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	var result map[string]any
	updatePrepared := false
	if err == nil {
		artifacts, downloadErr := gateway.downloadCommandArtifacts(ctx, item.ID, command, item.Record.Artifacts)
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
		} else if command.Kind == "workspace.rename" || command.Kind == "workspace.unregister" {
			result, err = gateway.mutateWorkspace(command)
		} else {
			result, err = gateway.adapter.Execute(ctx, command, artifacts)
			if err == nil && command.Kind == "thread.read" {
				err = gateway.uploadHistoryImages(ctx, result)
			}
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

func (gateway *Gateway) downloadCommandArtifacts(ctx context.Context, commandID string, command AgentCommand, grants []ArtifactGrant) (map[string]LocalArtifact, error) {
	gateway.workspaceMu.RLock()
	registeredWorkspaces := make(map[string]Workspace, len(gateway.workspaces))
	for id, workspace := range gateway.workspaces {
		registeredWorkspaces[id] = workspace
	}
	gateway.workspaceMu.RUnlock()
	var artifacts map[string]LocalArtifact
	err := runAsConfiguredUser(func() error {
		var downloadErr error
		artifacts, downloadErr = gateway.cloud.DownloadArtifacts(ctx, commandID, command, grants, registeredWorkspaces)
		return downloadErr
	})
	return artifacts, err
}

func (gateway *Gateway) uploadHistoryImages(ctx context.Context, result map[string]any) error {
	type imageUpload struct {
		path   string
		fileID string
		err    error
	}
	type messageUpload struct {
		message map[string]any
		images  []imageUpload
	}

	session, _ := result["session"].(map[string]any)
	messages, _ := session["messages"].([]any)
	work := make([]messageUpload, 0)
	totalImages := 0
	for _, rawMessage := range messages {
		message, _ := rawMessage.(map[string]any)
		localImages, _ := message["localAttachments"].([]any)
		if len(localImages) == 0 {
			continue
		}
		item := messageUpload{message: message, images: make([]imageUpload, len(localImages))}
		for index, rawImage := range localImages {
			image, _ := rawImage.(map[string]any)
			item.images[index].path, _ = image["path"].(string)
		}
		totalImages += len(item.images)
		work = append(work, item)
	}
	if totalImages == 0 {
		return nil
	}

	tasks := make(chan *imageUpload)
	var uploads sync.WaitGroup
	workerCount := min(historyImageUploadConcurrency, totalImages)
	uploads.Add(workerCount)
	for range workerCount {
		go func() {
			defer uploads.Done()
			for image := range tasks {
				image.err = runAsConfiguredUser(func() error {
					var uploadErr error
					image.fileID, uploadErr = gateway.cloud.UploadHistoryImage(ctx, gateway.config, gateway.identity, image.path)
					return uploadErr
				})
			}
		}()
	}
	for index := range work {
		for imageIndex := range work[index].images {
			tasks <- &work[index].images[imageIndex]
		}
	}
	close(tasks)
	uploads.Wait()

	for _, item := range work {
		message := item.message
		delete(message, "localAttachments")
		attachments := make([]any, 0, len(item.images))
		unavailable := 0
		for _, image := range item.images {
			if image.err != nil {
				if !errors.Is(image.err, errHistoryImageUnavailable) {
					return image.err
				}
				unavailable++
				gateway.logger.Printf("history image unavailable: %s", publicMessage(image.err))
				continue
			}
			attachments = append(attachments, map[string]any{"fileID": image.fileID})
		}
		if len(attachments) > 0 {
			message["attachments"] = attachments
		}
		content := stripSyntheticFileMentions(fmt.Sprint(message["content"]))
		if unavailable > 0 {
			content = strings.TrimSpace(content + "\n\n[One or more attached images are unavailable on this device.]")
		}
		if content == "" {
			content = "[Attached image]"
		}
		message["content"] = content
	}
	return nil
}

func stripSyntheticFileMentions(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	start := strings.Index(content, "# Files mentioned by the user:")
	if start < 0 {
		return strings.TrimSpace(content)
	}
	requestMarker := "## My request:"
	requestStart := strings.Index(content[start:], requestMarker)
	if requestStart < 0 {
		return strings.TrimSpace(content[:start])
	}
	requestStart += start + len(requestMarker)
	return strings.TrimSpace(content[:start] + content[requestStart:])
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
		workspace.Registered = true
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
		if !current.Excluded {
			mergeWorkspace(byID, current)
		}
	}
	mergeWorkspace(byID, workspace)
	registered := make([]Workspace, 0, len(byID))
	for _, current := range byID {
		registered = append(registered, current)
	}
	sort.Slice(registered, func(i, j int) bool {
		return registered[i].Name < registered[j].Name
	})
	gateway.config = persisted
	gateway.workspaces = byID
	gateway.adapter.replaceWorkspaces(persisted.Workspaces)
	gateway.workspaceMu.Unlock()
	gateway.signalWorkspaceUpdate(registered)

	return map[string]any{"kind": "accepted", "workspaceId": workspace.WorkspaceID, "name": workspace.Name}, nil
}

func (gateway *Gateway) mutateWorkspace(command AgentCommand) (map[string]any, error) {
	gateway.registerMu.Lock()
	defer gateway.registerMu.Unlock()
	configPath := filepath.Join(gateway.dataDir, "config.json")
	persisted, err := LoadConfig(configPath)
	if err != nil {
		return nil, err
	}
	gateway.workspaceMu.RLock()
	discovered, discoveredExists := gateway.workspaces[command.WorkspaceID]
	gateway.workspaceMu.RUnlock()
	index := -1
	for currentIndex := range persisted.Workspaces {
		current := persisted.Workspaces[currentIndex]
		if current.WorkspaceID == command.WorkspaceID {
			index = currentIndex
			break
		}
	}
	if index < 0 {
		if !discoveredExists || discovered.Excluded || strings.TrimSpace(discovered.Root) == "" {
			return nil, errors.New("workspace is not available on this device")
		}
		if len(persisted.Workspaces) >= 128 {
			return nil, errors.New("workspace limit reached")
		}
		discovered.Registered = true
		discovered.Excluded = false
		persisted.Workspaces = append(persisted.Workspaces, discovered)
		index = len(persisted.Workspaces) - 1
	}
	workspace := persisted.Workspaces[index]
	if command.Kind == "workspace.unregister" && workspace.Excluded {
		return map[string]any{"kind": "accepted", "workspaceId": workspace.WorkspaceID}, nil
	}
	if !workspace.Registered || workspace.Excluded {
		return nil, errors.New("workspace is not managed by DEEIX")
	}
	if command.Kind == "workspace.rename" {
		name := strings.TrimSpace(command.Name)
		if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > 128 {
			return nil, errors.New("workspace name is invalid")
		}
		workspace.Name = name
	} else {
		workspace.Registered = false
		workspace.Excluded = true
		workspace.Hidden = false
	}
	persisted.Workspaces[index] = workspace
	if err = SaveConfig(configPath, persisted); err != nil {
		return nil, err
	}

	gateway.workspaceMu.Lock()
	visible := make(map[string]Workspace, len(gateway.workspaces))
	for id, current := range gateway.workspaces {
		visible[id] = current
	}
	if command.Kind == "workspace.rename" {
		current, ok := visible[workspace.WorkspaceID]
		if ok {
			current.Name = workspace.Name
			current.Registered = true
			visible[workspace.WorkspaceID] = current
		}
	} else {
		delete(visible, workspace.WorkspaceID)
	}
	registered := make([]Workspace, 0, len(visible))
	for _, current := range visible {
		registered = append(registered, current)
	}
	sort.Slice(registered, func(i, j int) bool { return registered[i].Name < registered[j].Name })
	gateway.config = persisted
	gateway.workspaces = visible
	gateway.adapter.replaceWorkspaces(registered)
	gateway.workspaceMu.Unlock()
	gateway.signalWorkspaceUpdate(registered)

	result := map[string]any{"kind": "accepted", "workspaceId": workspace.WorkspaceID}
	if command.Kind == "workspace.rename" {
		result["name"] = workspace.Name
	}
	return result, nil
}

func concurrentCommand(kind string) bool {
	return kind == "interaction.respond" || kind == "workspace.register" || kind == "workspace.rename" || kind == "workspace.unregister"
}

func (gateway *Gateway) flushOutgoing(writer *socketWriter, after uint64) (uint64, error) {
	pending := outgoingBatch(gateway.state.PendingOutgoing(after))
	if len(pending) == 0 {
		return after, nil
	}
	sentThrough := after
	for _, outgoing := range pending {
		frame := bridgeFrame{Version: bridgeVersion, Type: outgoing.Type, BridgeSeq: outgoing.BridgeSeq, ServerSeq: outgoing.ServerSeq, CommandID: outgoing.CommandID, Outcome: outgoing.Outcome, Event: outgoing.Event}
		if err := writer.send(frame); err != nil {
			return sentThrough, err
		}
		sentThrough = outgoing.BridgeSeq
	}
	return sentThrough, nil
}

func outgoingBatch(pending []outgoingFrame) []outgoingFrame {
	if len(pending) > gatewayOutgoingBatchSize {
		return pending[:gatewayOutgoingBatchSize]
	}
	return pending
}

func (gateway *Gateway) signalWake() {
	select {
	case gateway.wake <- struct{}{}:
	default:
	}
}

func (gateway *Gateway) signalWorkspaceUpdate(workspaces []Workspace) {
	update := append([]Workspace(nil), workspaces...)
	select {
	case gateway.workspaceUpdates <- update:
		return
	default:
	}
	select {
	case <-gateway.workspaceUpdates:
	default:
	}
	select {
	case gateway.workspaceUpdates <- update:
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

func writeSupervisorStatus(dataDir, state, lastError string) error {
	status, err := ReadRuntimeStatus(dataDir)
	if err != nil {
		status = RuntimeStatus{Version: 1}
		if config, loadErr := LoadConfig(filepath.Join(dataDir, "config.json")); loadErr == nil {
			status.DeviceID = config.DeviceID
		}
	}
	status.Version = 1
	status.PID = os.Getpid()
	status.State = state
	status.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	status.LastError = lastError
	data, _ := json.MarshalIndent(status, "", "  ")
	return writeFileAtomic(filepath.Join(dataDir, "runtime-status.json"), append(data, '\n'), 0o600)
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
	if command.Kind == "agent.update" || command.Kind == "resource.refresh" || command.Kind == "workspace.rename" || command.Kind == "workspace.unregister" || command.Kind == "thread.rename" || command.Kind == "thread.metadata.update" || command.Kind == "turn.interrupt" {
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
