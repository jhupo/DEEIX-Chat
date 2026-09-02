package agentclient

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
)

const (
	codexSessionTaskScanChunk = 256 << 10
	codexSessionTaskMaxGap    = 64 << 20
)

type codexSessionTaskState struct {
	initialized bool
	offset      int64
	turnID      string
	status      string
	startedAt   float64
	completedAt float64
}

type codexSessionTaskRecord struct {
	Type    string `json:"type"`
	Payload struct {
		Type        string          `json:"type"`
		TurnID      string          `json:"turn_id"`
		StartedAt   float64         `json:"started_at"`
		CompletedAt float64         `json:"completed_at"`
		Error       json.RawMessage `json:"error"`
	} `json:"payload"`
}

func (adapter *CodexAdapter) sessionTaskStateForProvider(providerThreadID string) (codexSessionTaskState, error) {
	if watched, found := adapter.watchedSessionByProviderID(providerThreadID); found && !watched.archived {
		return adapter.sessionTaskState(watched.path)
	}
	return codexSessionTaskState{}, nil
}

func (adapter *CodexAdapter) sessionTaskState(path string) (codexSessionTaskState, error) {
	path = canonicalSessionPath(path)
	if path == "" {
		return codexSessionTaskState{}, nil
	}
	if !adapter.sessionPathAllowed(path) {
		return codexSessionTaskState{}, errors.New("Codex session task path is invalid")
	}

	adapter.sessionTaskMu.Lock()
	defer adapter.sessionTaskMu.Unlock()

	file, err := os.Open(path)
	if err != nil {
		return codexSessionTaskState{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return codexSessionTaskState{}, err
	}
	state := adapter.sessionTasks[path]
	if !state.initialized || info.Size() < state.offset || info.Size()-state.offset > codexSessionTaskMaxGap {
		state, err = initializeCodexSessionTaskState(file, info.Size())
	} else if info.Size() > state.offset {
		state, err = advanceCodexSessionTaskState(file, info.Size(), state)
	}
	if err != nil {
		return codexSessionTaskState{}, err
	}
	if adapter.sessionTasks == nil {
		adapter.sessionTasks = make(map[string]codexSessionTaskState)
	}
	adapter.sessionTasks[path] = state
	return state, nil
}

func initializeCodexSessionTaskState(file *os.File, size int64) (codexSessionTaskState, error) {
	completeOffset, err := codexSessionCompleteOffset(file, size)
	if err != nil {
		return codexSessionTaskState{}, err
	}
	state := codexSessionTaskState{initialized: true, offset: completeOffset}
	position := completeOffset
	var suffix []byte
	for position > 0 {
		start := max(int64(0), position-codexSessionTaskScanChunk)
		chunk := make([]byte, position-start)
		if _, err = file.ReadAt(chunk, start); err != nil && !errors.Is(err, io.EOF) {
			return codexSessionTaskState{}, err
		}
		data := make([]byte, 0, len(chunk)+len(suffix))
		data = append(data, chunk...)
		data = append(data, suffix...)
		lineEnd := len(data)
		for index := len(chunk) - 1; index >= 0; index-- {
			if chunk[index] != '\n' {
				continue
			}
			if boundary, ok := parseCodexSessionTaskRecord(data[index+1 : lineEnd]); ok {
				applyCodexSessionTaskRecord(&state, boundary)
				return state, nil
			}
			lineEnd = index
		}
		suffix = append(suffix[:0], data[:lineEnd]...)
		if len(suffix) > codexSessionTaskMaxGap {
			return codexSessionTaskState{}, errors.New("Codex session journal line exceeds the scan limit")
		}
		position = start
	}
	if boundary, ok := parseCodexSessionTaskRecord(suffix); ok {
		applyCodexSessionTaskRecord(&state, boundary)
	}
	return state, nil
}

func advanceCodexSessionTaskState(file *os.File, size int64, state codexSessionTaskState) (codexSessionTaskState, error) {
	completeOffset, err := codexSessionCompleteOffset(file, size)
	if err != nil || completeOffset <= state.offset {
		return state, err
	}
	data := make([]byte, completeOffset-state.offset)
	if _, err = file.ReadAt(data, state.offset); err != nil && !errors.Is(err, io.EOF) {
		return codexSessionTaskState{}, err
	}
	for _, line := range bytes.Split(data, []byte{'\n'}) {
		if boundary, ok := parseCodexSessionTaskRecord(line); ok {
			applyCodexSessionTaskRecord(&state, boundary)
		}
	}
	state.offset = completeOffset
	return state, nil
}

func codexSessionCompleteOffset(file *os.File, size int64) (int64, error) {
	position := size
	for position > 0 {
		start := max(int64(0), position-codexSessionTaskScanChunk)
		chunk := make([]byte, position-start)
		if _, err := file.ReadAt(chunk, start); err != nil && !errors.Is(err, io.EOF) {
			return 0, err
		}
		if index := bytes.LastIndexByte(chunk, '\n'); index >= 0 {
			return start + int64(index) + 1, nil
		}
		position = start
	}
	return 0, nil
}

func parseCodexSessionTaskRecord(line []byte) (codexSessionTaskRecord, bool) {
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return codexSessionTaskRecord{}, false
	}
	prefix := line
	if len(prefix) > 512 {
		prefix = prefix[:512]
	}
	if !bytes.Contains(prefix, []byte(`"type":"event_msg"`)) ||
		(!bytes.Contains(prefix, []byte(`"type":"task_started"`)) &&
			!bytes.Contains(prefix, []byte(`"type":"task_complete"`)) &&
			!bytes.Contains(prefix, []byte(`"type":"turn_aborted"`))) {
		return codexSessionTaskRecord{}, false
	}
	var record codexSessionTaskRecord
	if json.Unmarshal(line, &record) != nil || record.Type != "event_msg" || strings.TrimSpace(record.Payload.TurnID) == "" {
		return codexSessionTaskRecord{}, false
	}
	switch record.Payload.Type {
	case "task_started", "task_complete", "turn_aborted":
		return record, true
	default:
		return codexSessionTaskRecord{}, false
	}
}

func applyCodexSessionTaskRecord(state *codexSessionTaskState, record codexSessionTaskRecord) {
	state.turnID = strings.TrimSpace(record.Payload.TurnID)
	state.startedAt = record.Payload.StartedAt
	state.completedAt = record.Payload.CompletedAt
	switch record.Payload.Type {
	case "task_started":
		state.status = "inProgress"
	case "turn_aborted":
		state.status = "interrupted"
	case "task_complete":
		state.status = "completed"
		errorValue := bytes.TrimSpace(record.Payload.Error)
		if len(errorValue) > 0 && !bytes.Equal(errorValue, []byte("null")) {
			state.status = "failed"
		}
	}
}

func applyCodexSessionTaskState(turns []any, state codexSessionTaskState) {
	if state.turnID == "" || state.status == "" {
		return
	}
	for index := len(turns) - 1; index >= 0; index-- {
		turn, ok := turns[index].(map[string]any)
		if !ok || strings.TrimSpace(stringField(turn, "id")) != state.turnID {
			continue
		}
		turn["status"] = state.status
		if state.startedAt > 0 {
			turn["startedAt"] = state.startedAt
		}
		if state.status == "inProgress" {
			delete(turn, "completedAt")
		} else if state.completedAt > 0 {
			turn["completedAt"] = state.completedAt
		}
		return
	}
}

func (adapter *CodexAdapter) sessionTaskStillRunning(providerThreadID, providerTurnID string) bool {
	state, err := adapter.sessionTaskStateForProvider(providerThreadID)
	return err == nil && state.turnID == strings.TrimSpace(providerTurnID) && state.status == "inProgress"
}
