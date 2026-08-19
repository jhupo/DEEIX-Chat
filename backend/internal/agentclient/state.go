package agentclient

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"sort"
	"strings"
	"sync"
)

type durableState struct {
	Version       int                      `json:"version"`
	AckServerSeq  uint64                   `json:"ackServerSeq"`
	AckBridgeSeq  uint64                   `json:"ackBridgeSeq"`
	NextBridgeSeq uint64                   `json:"nextBridgeSeq"`
	Commands      map[string]commandRecord `json:"commands"`
	Outgoing      []outgoingFrame          `json:"outgoing"`
	Sources       []sourceMapping          `json:"sources"`
}

type commandRecord struct {
	ServerSeq uint64          `json:"serverSeq"`
	Hash      string          `json:"hash"`
	Command   json.RawMessage `json:"command"`
	Artifacts []ArtifactGrant `json:"artifacts,omitempty"`
	State     string          `json:"state"`
	Outcome   json.RawMessage `json:"outcome,omitempty"`
	BridgeSeq uint64          `json:"bridgeSeq,omitempty"`
}

type outgoingFrame struct {
	Type      string          `json:"type"`
	BridgeSeq uint64          `json:"bridgeSeq"`
	ServerSeq uint64          `json:"serverSeq,omitempty"`
	CommandID string          `json:"commandId,omitempty"`
	Outcome   json.RawMessage `json:"outcome,omitempty"`
	Event     json.RawMessage `json:"event,omitempty"`
}

type sourceMapping struct {
	ProfileID  string `json:"profileId"`
	Kind       string `json:"kind"`
	SourceRef  string `json:"sourceRef"`
	ProviderID string `json:"providerId"`
}

type StateStore struct {
	mu    sync.Mutex
	path  string
	state durableState
}

func OpenStateStore(path string) (*StateStore, error) {
	store := &StateStore{path: path, state: durableState{Version: 1, Commands: make(map[string]commandRecord)}}
	data, err := readFileAtomic(path)
	if errors.Is(err, os.ErrNotExist) {
		if err = store.persistLocked(); err != nil {
			return nil, err
		}
		return store, nil
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(data, &store.state); err != nil {
		return nil, fmt.Errorf("read agent state: %w", err)
	}
	if err = store.validateLocked(); err != nil {
		return nil, err
	}
	return store, nil
}

func (store *StateStore) Cursors() (uint64, uint64) {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.state.AckServerSeq, store.state.AckBridgeSeq
}

func (store *StateStore) AcknowledgeServer(through uint64) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if through <= store.state.AckServerSeq {
		return nil
	}
	previous := cloneDurableState(store.state)
	store.state.AckServerSeq = through
	if err := store.persistLocked(); err != nil {
		store.state = previous
		return err
	}
	return nil
}

func (store *StateStore) Receive(serverSeq uint64, commandID string, command json.RawMessage, artifacts []ArtifactGrant) (commandRecord, bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if serverSeq == 0 || !validRef(commandID, 256) || !json.Valid(command) {
		return commandRecord{}, false, errors.New("gateway command receipt is invalid")
	}
	hashText, err := jsonHash(command)
	if err != nil {
		return commandRecord{}, false, errors.New("gateway command JSON is invalid")
	}
	if current, ok := store.state.Commands[commandID]; ok {
		if current.ServerSeq != serverSeq || current.Hash != hashText {
			return commandRecord{}, false, errors.New("gateway command identity was reused")
		}
		return cloneCommandRecord(current), false, nil
	}
	if serverSeq != store.state.AckServerSeq+1 {
		return commandRecord{}, false, fmt.Errorf("gateway command sequence gap: got %d after %d", serverSeq, store.state.AckServerSeq)
	}
	for _, artifact := range artifacts {
		if err := validateArtifactGrant(artifact); err != nil {
			return commandRecord{}, false, err
		}
	}
	previous := cloneDurableState(store.state)
	record := commandRecord{ServerSeq: serverSeq, Hash: hashText, Command: append(json.RawMessage(nil), command...), Artifacts: append([]ArtifactGrant(nil), artifacts...), State: "received"}
	store.state.Commands[commandID] = record
	store.state.AckServerSeq = serverSeq
	if err := store.persistLocked(); err != nil {
		store.state = previous
		return commandRecord{}, false, err
	}
	return cloneCommandRecord(record), true, nil
}

func (store *StateStore) MarkStarted(commandID string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	record, ok := store.state.Commands[commandID]
	if !ok {
		return errors.New("gateway command is not journaled")
	}
	if record.State == "terminal" || record.State == "started" {
		return nil
	}
	previous := cloneDurableState(store.state)
	record.State = "started"
	store.state.Commands[commandID] = record
	if err := store.persistLocked(); err != nil {
		store.state = previous
		return err
	}
	return nil
}

func (store *StateStore) AppendTerminal(commandID string, outcome json.RawMessage) (outgoingFrame, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	normalized, err := normalizeBridgeJSON(outcome)
	if err != nil {
		return outgoingFrame{}, errors.New("gateway terminal outcome is invalid")
	}
	outcome = normalized
	record, ok := store.state.Commands[commandID]
	if !ok || !validTerminalOutcome(outcome) {
		return outgoingFrame{}, errors.New("gateway terminal outcome is invalid")
	}
	if record.State == "terminal" {
		if !jsonEqual(record.Outcome, outcome) {
			return outgoingFrame{}, errors.New("gateway terminal outcome cannot be replaced")
		}
		return outgoingFrame{Type: "terminal", BridgeSeq: record.BridgeSeq, ServerSeq: record.ServerSeq, CommandID: commandID, Outcome: append(json.RawMessage(nil), outcome...)}, nil
	}
	previous := cloneDurableState(store.state)
	store.state.NextBridgeSeq++
	frame := outgoingFrame{Type: "terminal", BridgeSeq: store.state.NextBridgeSeq, ServerSeq: record.ServerSeq, CommandID: commandID, Outcome: append(json.RawMessage(nil), outcome...)}
	record.State = "terminal"
	record.Outcome = append(json.RawMessage(nil), outcome...)
	record.BridgeSeq = frame.BridgeSeq
	store.state.Commands[commandID] = record
	store.state.Outgoing = append(store.state.Outgoing, frame)
	if err := store.persistLocked(); err != nil {
		store.state = previous
		return outgoingFrame{}, err
	}
	return frame, nil
}

func (store *StateStore) AppendEvent(event json.RawMessage) (outgoingFrame, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	normalized, err := normalizeBridgeJSON(event)
	if err != nil {
		return outgoingFrame{}, errors.New("provider event is invalid")
	}
	event = normalized
	if !validProviderEvent(event) {
		return outgoingFrame{}, errors.New("provider event is invalid")
	}
	previous := cloneDurableState(store.state)
	store.state.NextBridgeSeq++
	frame := outgoingFrame{Type: "event", BridgeSeq: store.state.NextBridgeSeq, Event: append(json.RawMessage(nil), event...)}
	store.state.Outgoing = append(store.state.Outgoing, frame)
	if err := store.persistLocked(); err != nil {
		store.state = previous
		return outgoingFrame{}, err
	}
	return frame, nil
}

func normalizeBridgeJSON(raw json.RawMessage) (json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, errors.New("JSON contains trailing data")
	}
	return json.Marshal(normalizeBridgeJSONValue(value))
}

func normalizeBridgeJSONValue(value any) any {
	switch typed := value.(type) {
	case string:
		return strings.Map(func(character rune) rune {
			if character < 32 && character != '\n' && character != '\r' && character != '\t' {
				return '\uFFFD'
			}
			return character
		}, strings.ToValidUTF8(typed, "\uFFFD"))
	case []any:
		for index := range typed {
			typed[index] = normalizeBridgeJSONValue(typed[index])
		}
		return typed
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			normalizedKey, _ := normalizeBridgeJSONValue(key).(string)
			result[normalizedKey] = normalizeBridgeJSONValue(item)
		}
		return result
	default:
		return value
	}
}

func (store *StateStore) PendingOutgoing(after uint64) []outgoingFrame {
	store.mu.Lock()
	defer store.mu.Unlock()
	result := make([]outgoingFrame, 0, len(store.state.Outgoing))
	for _, frame := range store.state.Outgoing {
		if frame.BridgeSeq > after {
			result = append(result, cloneOutgoingFrame(frame))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].BridgeSeq < result[j].BridgeSeq })
	return result
}

func (store *StateStore) AcknowledgeBridge(through uint64) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if through <= store.state.AckBridgeSeq {
		return nil
	}
	if through > store.state.NextBridgeSeq {
		return errors.New("bridge acknowledgment exceeds the durable cursor")
	}
	previous := cloneDurableState(store.state)
	want := store.state.AckBridgeSeq + 1
	for _, frame := range store.state.Outgoing {
		if frame.BridgeSeq < want || frame.BridgeSeq > through {
			continue
		}
		if frame.BridgeSeq != want {
			return errors.New("durable outgoing frame sequence contains a gap")
		}
		want++
	}
	if want != through+1 {
		return errors.New("bridge acknowledgment references a missing frame")
	}
	store.state.AckBridgeSeq = through
	pending := store.state.Outgoing[:0]
	for _, frame := range store.state.Outgoing {
		if frame.BridgeSeq > through {
			pending = append(pending, frame)
		}
	}
	store.state.Outgoing = pending
	for commandID, record := range store.state.Commands {
		if record.State == "terminal" && record.BridgeSeq <= through {
			delete(store.state.Commands, commandID)
		}
	}
	if err := store.persistLocked(); err != nil {
		store.state = previous
		return err
	}
	return nil
}

func (store *StateStore) RecoverableCommands() map[string]commandRecord {
	store.mu.Lock()
	defer store.mu.Unlock()
	result := make(map[string]commandRecord)
	for id, record := range store.state.Commands {
		if record.State != "terminal" {
			result[id] = cloneCommandRecord(record)
		}
	}
	return result
}

func (store *StateStore) ResolveSource(profileID, kind, sourceRef string) (string, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	for _, mapping := range store.state.Sources {
		if mapping.ProfileID == profileID && mapping.Kind == kind && mapping.SourceRef == sourceRef {
			return mapping.ProviderID, nil
		}
	}
	return "", fmt.Errorf("%s source reference is not registered: %s", kind, sourceRef)
}

func (store *StateStore) PublishSource(profileID, kind, providerID string) (string, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if !validRef(profileID, 64) || !validSourceKind(kind) || providerID == "" || len(providerID) > 4096 {
		return "", errors.New("source mapping is invalid")
	}
	for _, mapping := range store.state.Sources {
		if mapping.ProfileID == profileID && mapping.Kind == kind && mapping.ProviderID == providerID {
			return mapping.SourceRef, nil
		}
	}
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	sourceRef := kind + "_" + hex.EncodeToString(random)
	previous := cloneDurableState(store.state)
	store.state.Sources = append(store.state.Sources, sourceMapping{ProfileID: profileID, Kind: kind, SourceRef: sourceRef, ProviderID: providerID})
	if err := store.persistLocked(); err != nil {
		store.state = previous
		return "", err
	}
	return sourceRef, nil
}

func (store *StateStore) validateLocked() error {
	if store.state.Version != 1 || store.state.AckBridgeSeq > store.state.NextBridgeSeq || store.state.Commands == nil {
		return errors.New("agent state is invalid")
	}
	last := store.state.AckBridgeSeq
	for _, frame := range store.state.Outgoing {
		if frame.BridgeSeq != last+1 || frame.BridgeSeq > store.state.NextBridgeSeq {
			return errors.New("agent outgoing state contains a sequence gap")
		}
		if frame.Type == "terminal" && (!validRef(frame.CommandID, 256) || frame.ServerSeq == 0 || !validTerminalOutcome(frame.Outcome)) {
			return errors.New("agent terminal state is invalid")
		}
		if frame.Type == "event" && !validProviderEvent(frame.Event) {
			return errors.New("agent event state is invalid")
		}
		if frame.Type != "terminal" && frame.Type != "event" {
			return errors.New("agent outgoing frame type is invalid")
		}
		last = frame.BridgeSeq
	}
	if last != store.state.NextBridgeSeq {
		return errors.New("agent outgoing cursor is invalid")
	}
	for id, record := range store.state.Commands {
		if !validRef(id, 256) || record.ServerSeq == 0 || !json.Valid(record.Command) || (record.State != "received" && record.State != "started" && record.State != "terminal") {
			return errors.New("agent command state is invalid")
		}
		hash, err := jsonHash(record.Command)
		if err != nil || record.Hash != hash {
			return errors.New("agent command state checksum is invalid")
		}
		for _, artifact := range record.Artifacts {
			if err := validateArtifactGrant(artifact); err != nil {
				return errors.New("agent command artifact state is invalid")
			}
		}
	}
	for _, mapping := range store.state.Sources {
		if !validRef(mapping.ProfileID, 64) || !validSourceKind(mapping.Kind) || !validRef(mapping.SourceRef, 256) || mapping.ProviderID == "" || len(mapping.ProviderID) > 4096 {
			return errors.New("agent source state is invalid")
		}
	}
	return nil
}

func (store *StateStore) persistLocked() error {
	data, err := json.MarshalIndent(store.state, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(store.path, append(data, '\n'), 0o600)
}

func cloneCommandRecord(value commandRecord) commandRecord {
	value.Command = append(json.RawMessage(nil), value.Command...)
	value.Outcome = append(json.RawMessage(nil), value.Outcome...)
	value.Artifacts = append([]ArtifactGrant(nil), value.Artifacts...)
	return value
}

func cloneOutgoingFrame(value outgoingFrame) outgoingFrame {
	value.Outcome = append(json.RawMessage(nil), value.Outcome...)
	value.Event = append(json.RawMessage(nil), value.Event...)
	return value
}

func jsonEqual(left, right []byte) bool {
	var a, b any
	return json.Unmarshal(left, &a) == nil && json.Unmarshal(right, &b) == nil && reflect.DeepEqual(a, b)
}

func validSourceKind(value string) bool {
	return value == "thread" || value == "turn" || value == "item" || value == "request" || value == "skill" || value == "app"
}

func jsonHash(raw json.RawMessage) (string, error) {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", err
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(canonical)
	return hex.EncodeToString(hash[:]), nil
}

func cloneDurableState(value durableState) durableState {
	result := value
	result.Commands = make(map[string]commandRecord, len(value.Commands))
	for id, record := range value.Commands {
		result.Commands[id] = cloneCommandRecord(record)
	}
	result.Outgoing = make([]outgoingFrame, len(value.Outgoing))
	for index, frame := range value.Outgoing {
		result.Outgoing[index] = cloneOutgoingFrame(frame)
	}
	result.Sources = append([]sourceMapping(nil), value.Sources...)
	return result
}
