package agentclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
	ctx        context.Context
	config     Config
	dataDir    string
	identity   *DeviceIdentity
	state      *StateStore
	cloud      *CloudClient
	adapter    *CodexAdapter
	logger     *log.Logger
	wake       chan struct{}
	commands   chan queuedCommand
	activeMu   sync.Mutex
	active     map[string]bool
	workspaces map[string]Workspace
}

type queuedCommand struct {
	ID     string
	Record commandRecord
}

func RunGateway(ctx context.Context, dataDir string, logger *log.Logger) error {
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
		wake: make(chan struct{}, 1), commands: make(chan queuedCommand, 128), active: make(map[string]bool), workspaces: make(map[string]Workspace),
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
		token, tokenErr := gateway.cloud.ConnectionToken(connectContext, config, identity)
		cancel()
		if tokenErr == nil {
			tokenErr = gateway.runSocket(ctx, token)
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
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
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
		gateway.enqueue(id, record, command.Kind == "interaction.respond")
	}
	return nil
}

func (gateway *Gateway) runSocket(ctx context.Context, token string) error {
	endpoint, err := url.Parse(gateway.config.CloudURL)
	if err != nil {
		return err
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
	config, err := websocket.NewConfig(endpoint.String(), origin)
	if err != nil {
		return err
	}
	config.Protocol = []string{bridgeProtocol, "deeix.auth." + token}
	config.Dialer = &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	connection, err := websocket.DialConfig(config)
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
	if err = receiveBridgeFrame(connection, &challenge); err != nil || challenge.Version != bridgeVersion || challenge.Type != "auth.challenge" || challenge.ProfileID != gateway.config.ProfileID || !validPublicID(challenge.ChallengeID, "agp") || !strings.HasPrefix(challenge.Challenge, "deeix-runtime-auth-proof-v1\n") {
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
		return err
	}
	workspaces := make([]bridgeWorkspace, 0, len(gateway.config.Workspaces))
	for _, workspace := range gateway.config.Workspaces {
		workspaces = append(workspaces, bridgeWorkspace{WorkspaceID: workspace.WorkspaceID, Name: workspace.Name})
	}
	manifest := gateway.adapter.Manifest()
	if err = writer.send(bridgeFrame{Version: bridgeVersion, Type: "auth.proof", ProfileID: gateway.config.ProfileID, ChallengeID: challenge.ChallengeID, Proof: proof, Workspaces: workspaces, Manifest: &manifest}); err != nil {
		return err
	}
	var ready bridgeFrame
	if err = receiveBridgeFrame(connection, &ready); err != nil || ready.Version != bridgeVersion || ready.Type != "auth.ready" || ready.ProfileID != gateway.config.ProfileID {
		return errors.New("gateway runtime authorization failed")
	}
	var welcome bridgeFrame
	if err = receiveBridgeFrame(connection, &welcome); err != nil || welcome.Version != bridgeVersion || welcome.Type != "welcome" || welcome.DeviceID != gateway.config.DeviceID || welcome.HeartbeatSeconds < 5 || welcome.HeartbeatSeconds > 300 {
		return errors.New("gateway welcome frame is invalid")
	}
	_ = connection.SetDeadline(time.Time{})
	if welcome.AckBridgeSeq > ackBridge {
		if err = gateway.state.AcknowledgeBridge(welcome.AckBridgeSeq); err != nil {
			return err
		}
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
		case <-gateway.wake:
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
					gateway.enqueue(frame.CommandID, record, command.Kind == "interaction.respond")
				}
			default:
				return fmt.Errorf("unsupported gateway frame: %s", frame.Type)
			}
		}
	}
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
	if err == nil {
		artifacts, downloadErr := gateway.cloud.DownloadArtifacts(ctx, item.ID, command, item.Record.Artifacts, gateway.workspaces)
		if downloadErr != nil {
			err = downloadErr
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
	if _, appendErr := gateway.state.AppendTerminal(item.ID, outcome); appendErr != nil {
		gateway.logger.Printf("persist command %s terminal outcome: %v", item.ID, appendErr)
		return
	}
	gateway.signalWake()
}

func (gateway *Gateway) flushOutgoing(writer *socketWriter, after uint64) (uint64, error) {
	for _, outgoing := range gateway.state.PendingOutgoing(after) {
		frame := bridgeFrame{Version: bridgeVersion, Type: outgoing.Type, BridgeSeq: outgoing.BridgeSeq, ServerSeq: outgoing.ServerSeq, CommandID: outgoing.CommandID, Outcome: outgoing.Outcome, Event: outgoing.Event}
		if err := writer.send(frame); err != nil {
			return after, err
		}
		after = outgoing.BridgeSeq
	}
	return after, nil
}

func (gateway *Gateway) signalWake() {
	select {
	case gateway.wake <- struct{}{}:
	default:
	}
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
	if command.Kind == "resource.refresh" || command.Kind == "thread.rename" || command.Kind == "thread.metadata.update" || command.Kind == "turn.interrupt" {
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
