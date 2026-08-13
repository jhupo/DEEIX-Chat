package agentgateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	appagent "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/agentgateway"
	"golang.org/x/net/websocket"
)

const (
	bridgeProtocol     = "deeix.bridge.v1"
	authProtocolPrefix = "deeix.auth."
	bridgeVersion      = 1
	bridgeMaxPayload   = (2 << 20) + (64 << 10)
	bridgeHelloTimeout = 10 * time.Second
	bridgeHeartbeat    = 30 * time.Second
)

type bridgeFrame struct {
	Version          int                    `json:"version"`
	Type             string                 `json:"type"`
	ProfileID        string                 `json:"profileId,omitempty"`
	ChallengeID      string                 `json:"challengeId,omitempty"`
	Challenge        string                 `json:"challenge,omitempty"`
	ExpiresAt        string                 `json:"expiresAt,omitempty"`
	Proof            string                 `json:"proof,omitempty"`
	LeaseExpiresAt   string                 `json:"leaseExpiresAt,omitempty"`
	Workspaces       []bridgeWorkspace      `json:"workspaces,omitempty"`
	AckServerSeq     uint64                 `json:"ackServerSeq,omitempty"`
	AckBridgeSeq     uint64                 `json:"ackBridgeSeq,omitempty"`
	DeviceID         string                 `json:"deviceId,omitempty"`
	HeartbeatSeconds int                    `json:"heartbeatSeconds,omitempty"`
	ServerSeq        uint64                 `json:"serverSeq,omitempty"`
	BridgeSeq        uint64                 `json:"bridgeSeq,omitempty"`
	CommandID        string                 `json:"commandId,omitempty"`
	Command          json.RawMessage        `json:"command,omitempty"`
	Outcome          json.RawMessage        `json:"outcome,omitempty"`
	Event            json.RawMessage        `json:"event,omitempty"`
	Artifacts        *[]bridgeArtifactGrant `json:"artifacts,omitempty"`
}

type bridgeWorkspace struct {
	WorkspaceID string `json:"workspaceId"`
	Name        string `json:"name"`
}

type bridgeArtifactGrant struct {
	ArtifactRef string `json:"artifactRef"`
	FileName    string `json:"fileName"`
	MimeType    string `json:"mimeType"`
	SizeBytes   int64  `json:"sizeBytes"`
	SHA256      string `json:"sha256"`
	ExpiresAt   string `json:"expiresAt"`
	Grant       string `json:"grant"`
}

type bridgeHub struct {
	mu          sync.Mutex
	connections map[string]*websocket.Conn
}

func newBridgeHub() *bridgeHub {
	return &bridgeHub{connections: make(map[string]*websocket.Conn)}
}

func (h *bridgeHub) replace(deviceID string, connection *websocket.Conn) func() {
	h.mu.Lock()
	previous := h.connections[deviceID]
	h.connections[deviceID] = connection
	h.mu.Unlock()
	if previous != nil && previous != connection {
		_ = previous.Close()
	}
	return func() {
		h.mu.Lock()
		if h.connections[deviceID] == connection {
			delete(h.connections, deviceID)
		}
		h.mu.Unlock()
	}
}

func (h *Handler) connect(w http.ResponseWriter, request *http.Request) {
	var identity *appagent.ConnectionIdentity
	server := websocket.Server{
		Handshake: func(config *websocket.Config, req *http.Request) error {
			token, err := connectionToken(config.Protocol)
			if err != nil {
				return err
			}
			identity, err = h.service.AuthenticateConnection(req.Context(), token)
			if err != nil {
				return err
			}
			config.Protocol = []string{bridgeProtocol}
			return nil
		},
		Handler: func(connection *websocket.Conn) {
			if identity == nil {
				_ = connection.Close()
				return
			}
			connection.MaxPayloadBytes = bridgeMaxPayload
			h.serveBridge(connection, identity)
		},
	}
	server.ServeHTTP(w, request)
}

func (h *Handler) serveBridge(connection *websocket.Conn, identity *appagent.ConnectionIdentity) {
	_ = connection.SetDeadline(time.Now().Add(bridgeHelloTimeout))
	var hello bridgeFrame
	if err := receiveBridgeFrame(connection, &hello); err != nil ||
		hello.Version != bridgeVersion || hello.Type != "hello" ||
		hello.AckBridgeSeq > identity.LastAckedBridgeSeq || !validHelloFrame(hello) {
		_ = connection.Close()
		return
	}
	ctx, cancel := socketRuntimeAuthContext()
	challenge, err := h.service.BeginRuntimeProof(ctx, identity, hello.ProfileID)
	cancel()
	if err != nil {
		_ = connection.Close()
		return
	}
	if err = websocket.JSON.Send(connection, bridgeFrame{
		Version: bridgeVersion, Type: "auth.challenge", ProfileID: challenge.Profile.PublicID,
		ChallengeID: challenge.Challenge.PublicID, Challenge: challenge.Canonical,
		ExpiresAt: challenge.Challenge.ExpiresAt.UTC().Format(time.RFC3339Nano),
	}); err != nil {
		return
	}
	_ = connection.SetDeadline(challenge.Challenge.ExpiresAt)
	var proof bridgeFrame
	if err = receiveBridgeFrame(connection, &proof); err != nil ||
		proof.Version != bridgeVersion || proof.Type != "auth.proof" ||
		proof.ProfileID != challenge.Profile.PublicID || proof.ChallengeID != challenge.Challenge.PublicID ||
		!validAuthProofFrame(proof) {
		_ = connection.Close()
		return
	}
	ctx, cancel = socketRuntimeAuthContext()
	leaseExpiresAt, err := h.service.CompleteRuntimeProof(ctx, identity, challenge, proof.Proof)
	cancel()
	if err != nil {
		_ = connection.Close()
		return
	}
	registrations := make([]appagent.WorkspaceRegistration, 0, len(proof.Workspaces))
	for _, workspace := range proof.Workspaces {
		registrations = append(registrations, appagent.WorkspaceRegistration{WorkspaceID: workspace.WorkspaceID, Name: workspace.Name})
	}
	ctx, cancel = socketRuntimeAuthContext()
	err = h.service.SyncWorkspaces(ctx, identity, challenge, registrations)
	cancel()
	if err != nil {
		_ = connection.Close()
		return
	}
	if err = websocket.JSON.Send(connection, bridgeFrame{
		Version: bridgeVersion, Type: "auth.ready", ProfileID: challenge.Profile.PublicID,
		LeaseExpiresAt: leaseExpiresAt.UTC().Format(time.RFC3339Nano),
	}); err != nil {
		return
	}
	_ = connection.SetDeadline(time.Now().Add(2 * bridgeHeartbeat))
	cleanup := h.hub.replace(identity.DeviceID, connection)
	defer cleanup()

	ctx, cancel = socketOperationContext()
	err = h.service.AckServerCommands(ctx, identity, hello.AckServerSeq)
	cancel()
	if err != nil {
		_ = connection.Close()
		return
	}
	if hello.AckServerSeq > identity.LastAckedServerSeq {
		identity.LastAckedServerSeq = hello.AckServerSeq
	}
	if err := websocket.JSON.Send(connection, bridgeFrame{
		Version: bridgeVersion, Type: "welcome", DeviceID: identity.DeviceID,
		AckServerSeq: identity.LastAckedServerSeq, AckBridgeSeq: identity.LastAckedBridgeSeq,
		HeartbeatSeconds: int(bridgeHeartbeat / time.Second),
	}); err != nil {
		return
	}
	sentThrough, err := h.sendCommands(connection, identity, identity.LastAckedServerSeq)
	if err != nil {
		return
	}
	for {
		now := time.Now()
		if !now.Before(leaseExpiresAt) {
			return
		}
		deadline := now.Add(2 * bridgeHeartbeat)
		if leaseExpiresAt.Before(deadline) {
			deadline = leaseExpiresAt
		}
		_ = connection.SetDeadline(deadline)
		var frame bridgeFrame
		if err := receiveBridgeFrame(connection, &frame); err != nil {
			if errors.Is(err, io.EOF) {
				return
			}
			return
		}
		if frame.Version != bridgeVersion {
			return
		}
		switch frame.Type {
		case "ping":
			if !validPingFrame(frame) {
				return
			}
			if err := websocket.JSON.Send(connection, bridgeFrame{Version: bridgeVersion, Type: "pong"}); err != nil {
				return
			}
			if sentThrough == identity.LastAckedServerSeq {
				sentThrough, err = h.sendCommands(connection, identity, sentThrough)
				if err != nil {
					return
				}
			}
		case "ack.server":
			if !validServerAckFrame(frame) {
				return
			}
			ctx, cancel = socketOperationContext()
			err = h.service.AckServerCommands(ctx, identity, frame.AckServerSeq)
			cancel()
			if frame.AckServerSeq == 0 || err != nil {
				return
			}
			if frame.AckServerSeq > identity.LastAckedServerSeq {
				identity.LastAckedServerSeq = frame.AckServerSeq
			}
			if identity.LastAckedServerSeq == sentThrough {
				sentThrough, err = h.sendCommands(connection, identity, sentThrough)
				if err != nil {
					return
				}
			}
		case "terminal":
			if !validTerminalFrame(frame) {
				return
			}
			ctx, cancel = socketOperationContext()
			acknowledged, err := h.service.ApplyTerminalFrame(
				ctx, identity, frame.BridgeSeq, frame.ServerSeq, frame.CommandID, frame.Outcome,
			)
			cancel()
			if err != nil {
				return
			}
			if err = websocket.JSON.Send(connection, bridgeFrame{
				Version: bridgeVersion, Type: "ack.bridge", AckBridgeSeq: acknowledged,
			}); err != nil {
				return
			}
		case "event":
			if !validEventFrame(frame) {
				return
			}
			ctx, cancel = socketOperationContext()
			acknowledged, err := h.service.ApplyEventFrame(ctx, identity, challenge.Profile.ID, frame.BridgeSeq, frame.Event)
			cancel()
			if err != nil {
				return
			}
			if err = websocket.JSON.Send(connection, bridgeFrame{
				Version: bridgeVersion, Type: "ack.bridge", AckBridgeSeq: acknowledged,
			}); err != nil {
				return
			}
		default:
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
		return fmt.Errorf("bridge frame size is invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(frame); err != nil {
		return err
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return fmt.Errorf("bridge frame contains trailing data")
	}
	return nil
}

func validHelloFrame(frame bridgeFrame) bool {
	return validClientEmpty(frame) && validProfileID(frame.ProfileID) && frame.Proof == ""
}

func validEventFrame(frame bridgeFrame) bool {
	return frame.AckServerSeq == 0 && frame.AckBridgeSeq == 0 && frame.ServerSeq == 0 &&
		frame.BridgeSeq > 0 && frame.CommandID == "" && len(frame.Event) > 0 &&
		len(frame.Command) == 0 && len(frame.Outcome) == 0 && validClientMetadataEmpty(frame)
}

func validPingFrame(frame bridgeFrame) bool {
	return frame.AckServerSeq == 0 && frame.AckBridgeSeq == 0 && validClientEmpty(frame) && frame.ProfileID == "" && frame.Proof == ""
}

func validServerAckFrame(frame bridgeFrame) bool {
	return frame.AckServerSeq > 0 && frame.AckBridgeSeq == 0 && validClientEmpty(frame) && frame.ProfileID == "" && frame.Proof == ""
}

func validTerminalFrame(frame bridgeFrame) bool {
	return frame.AckServerSeq == 0 && frame.AckBridgeSeq == 0 && frame.ServerSeq > 0 &&
		frame.BridgeSeq > 0 && frame.CommandID != "" && len(frame.Outcome) > 0 &&
		len(frame.Command) == 0 && len(frame.Event) == 0 && validClientMetadataEmpty(frame)

}

func validAuthProofFrame(frame bridgeFrame) bool {
	if !validProfileID(frame.ProfileID) || !strings.HasPrefix(frame.ChallengeID, "agp_") ||
		len(frame.ChallengeID) != 36 || len(frame.Proof) < 43 || len(frame.Proof) > 64 ||
		len(frame.Workspaces) == 0 || len(frame.Workspaces) > 128 {
		return false
	}
	seen := make(map[string]struct{}, len(frame.Workspaces))
	for _, workspace := range frame.Workspaces {
		if !validProfileID(workspace.WorkspaceID) || strings.TrimSpace(workspace.Name) == "" || len(workspace.Name) > 128 {
			return false
		}
		if _, exists := seen[workspace.WorkspaceID]; exists {
			return false
		}
		seen[workspace.WorkspaceID] = struct{}{}
	}
	return frame.AckServerSeq == 0 && frame.AckBridgeSeq == 0 && frame.ServerSeq == 0 &&
		frame.BridgeSeq == 0 && frame.CommandID == "" && len(frame.Command) == 0 &&
		len(frame.Outcome) == 0 && len(frame.Event) == 0 && frame.DeviceID == "" &&
		frame.HeartbeatSeconds == 0 && frame.Challenge == "" && frame.ExpiresAt == "" &&
		frame.LeaseExpiresAt == "" && frame.Artifacts == nil
}

func validClientEmpty(frame bridgeFrame) bool {
	return frame.ServerSeq == 0 && frame.BridgeSeq == 0 && frame.CommandID == "" &&
		len(frame.Command) == 0 && len(frame.Outcome) == 0 && len(frame.Event) == 0 &&
		frame.DeviceID == "" && frame.HeartbeatSeconds == 0 && frame.ChallengeID == "" &&
		frame.Challenge == "" && frame.ExpiresAt == "" && frame.LeaseExpiresAt == "" &&
		len(frame.Workspaces) == 0 && frame.Artifacts == nil
}

func validClientMetadataEmpty(frame bridgeFrame) bool {
	return frame.DeviceID == "" && frame.HeartbeatSeconds == 0 && frame.ChallengeID == "" &&
		frame.Challenge == "" && frame.ExpiresAt == "" && frame.LeaseExpiresAt == "" &&
		frame.ProfileID == "" && frame.Proof == "" && len(frame.Workspaces) == 0 && frame.Artifacts == nil

}

func validProfileID(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for index, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || (index > 0 && strings.ContainsRune("._:-", character)) {
			continue
		}
		return false
	}
	return true
}

func (h *Handler) sendCommands(connection *websocket.Conn, identity *appagent.ConnectionIdentity, after uint64) (uint64, error) {
	ctx, cancel := socketOperationContext()
	defer cancel()
	items, err := h.service.CommandsForDelivery(ctx, identity, after)
	if err != nil {
		return after, err
	}
	sentThrough := after
	for _, item := range items {
		ctx, cancel = socketOperationContext()
		err = h.service.MarkCommandDelivered(ctx, identity, item.InternalID)
		cancel()
		if err != nil {
			return sentThrough, err
		}
		artifacts := make([]bridgeArtifactGrant, len(item.Artifacts))
		for index, artifact := range item.Artifacts {
			artifacts[index] = bridgeArtifactGrant{
				ArtifactRef: artifact.ArtifactRef, FileName: artifact.FileName,
				MimeType: artifact.MimeType, SizeBytes: artifact.SizeBytes,
				SHA256: artifact.SHA256, ExpiresAt: artifact.ExpiresAt, Grant: artifact.Grant,
			}
		}
		if err = websocket.JSON.Send(connection, bridgeFrame{
			Version: bridgeVersion, Type: "command", ServerSeq: item.ServerSeq,
			CommandID: item.CommandID, Command: item.Command, Artifacts: &artifacts,
		}); err != nil {
			return sentThrough, err
		}
		sentThrough = item.ServerSeq
	}
	return sentThrough, nil
}

func socketOperationContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 5*time.Second)
}

func socketRuntimeAuthContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 30*time.Second)
}

func connectionToken(protocols []string) (string, error) {
	bridgeSeen := false
	token := ""
	for _, protocol := range protocols {
		protocol = strings.TrimSpace(protocol)
		switch {
		case protocol == bridgeProtocol:
			if bridgeSeen {
				return "", fmt.Errorf("duplicate bridge protocol")
			}
			bridgeSeen = true
		case strings.HasPrefix(protocol, authProtocolPrefix):
			if token != "" {
				return "", fmt.Errorf("duplicate bridge credential")
			}
			token = strings.TrimPrefix(protocol, authProtocolPrefix)
		default:
			return "", fmt.Errorf("unsupported bridge protocol")
		}
	}
	if !bridgeSeen || token == "" || len(token) > 128 {
		return "", fmt.Errorf("missing bridge credential")
	}
	return token, nil
}
