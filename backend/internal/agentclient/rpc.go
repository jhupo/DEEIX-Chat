package agentclient

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sync"
	"sync/atomic"
)

const (
	maxRPCIncomingLineBytes = 64 << 20
	maxRPCOutgoingLineBytes = 4 << 20
)

var errRPCFrameTooLarge = errors.New("app-server frame exceeds the limit")

var rpcMethodPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9._/-]{0,255}$`)

type RPCNotification struct {
	Method string
	Params json.RawMessage
}

type RPCServerRequest struct {
	ID     json.RawMessage
	Method string
	Params json.RawMessage
}

type RPCClient struct {
	input  io.WriteCloser
	output io.ReadCloser

	writeMu sync.Mutex
	mu      sync.Mutex
	pending map[uint64]chan rpcResponse
	closed  chan struct{}
	err     error
	nextID  atomic.Uint64

	onNotification  func(RPCNotification) error
	onServerRequest func(context.Context, RPCServerRequest) (any, error)
}

type rpcResponse struct {
	result json.RawMessage
	err    error
}

type rpcError struct {
	Code    int
	Message string
}

func (err *rpcError) Error() string {
	return fmt.Sprintf("app-server error %d: %s", err.Code, err.Message)
}

func isRPCErrorCode(err error, code int) bool {
	var rpcErr *rpcError
	return errors.As(err, &rpcErr) && rpcErr.Code == code
}

type rpcEnvelope struct {
	ID          json.RawMessage `json:"id"`
	Method      string          `json:"method"`
	Params      json.RawMessage `json:"params"`
	Result      json.RawMessage `json:"result"`
	EmittedAtMS int64           `json:"emittedAtMs"`
	Error       *struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data,omitempty"`
	} `json:"error"`
}

func NewRPCClient(input io.WriteCloser, output io.ReadCloser) *RPCClient {
	client := &RPCClient{
		input: input, output: output, pending: make(map[uint64]chan rpcResponse), closed: make(chan struct{}),
	}
	client.nextID.Store(0)
	go client.readLoop()
	return client
}

func (client *RPCClient) SetHandlers(notification func(RPCNotification) error, request func(context.Context, RPCServerRequest) (any, error)) {
	client.mu.Lock()
	defer client.mu.Unlock()
	client.onNotification = notification
	client.onServerRequest = request
}

func (client *RPCClient) Request(ctx context.Context, method string, params any, result any) error {
	if !rpcMethodPattern.MatchString(method) {
		return errors.New("app-server RPC method is invalid")
	}
	id := client.nextID.Add(1)
	response := make(chan rpcResponse, 1)
	client.mu.Lock()
	if client.err != nil {
		err := client.err
		client.mu.Unlock()
		return err
	}
	client.pending[id] = response
	client.mu.Unlock()

	payload := map[string]any{"id": id, "method": method}
	if params != nil {
		payload["params"] = params
	}
	if err := client.write(payload); err != nil {
		client.removePending(id)
		return err
	}
	select {
	case <-ctx.Done():
		client.removePending(id)
		return ctx.Err()
	case item := <-response:
		if item.err != nil {
			return item.err
		}
		if result == nil {
			return nil
		}
		if err := json.Unmarshal(item.result, result); err != nil {
			return fmt.Errorf("decode app-server response: %w", err)
		}
		return nil
	}
}

func (client *RPCClient) Notify(method string, params any) error {
	if !rpcMethodPattern.MatchString(method) {
		return errors.New("app-server RPC method is invalid")
	}
	payload := map[string]any{"method": method}
	if params != nil {
		payload["params"] = params
	}
	return client.write(payload)
}

func (client *RPCClient) Close() error {
	client.closeWithError(errors.New("app-server RPC client closed"))
	_ = client.input.Close()
	return client.output.Close()
}

func (client *RPCClient) Done() <-chan struct{} { return client.closed }

func (client *RPCClient) write(value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(data)+1 > maxRPCOutgoingLineBytes {
		return errors.New("outgoing app-server frame exceeds the limit")
	}
	client.writeMu.Lock()
	defer client.writeMu.Unlock()
	client.mu.Lock()
	closedErr := client.err
	client.mu.Unlock()
	if closedErr != nil {
		return closedErr
	}
	data = append(data, '\n')
	_, err = client.input.Write(data)
	return err
}

func (client *RPCClient) readLoop() {
	reader := bufio.NewReaderSize(client.output, 64*1024)
	for {
		line, err := readRPCLine(reader)
		if len(bytes.TrimSpace(line)) > 0 {
			if handleErr := client.handleLine(bytes.TrimSpace(line)); handleErr != nil {
				client.closeWithError(handleErr)
				return
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				err = errors.New("app-server output ended")
			}
			client.closeWithError(err)
			return
		}
	}
}

func readRPCLine(reader *bufio.Reader) ([]byte, error) {
	line := make([]byte, 0, 64*1024)
	for {
		fragment, err := reader.ReadSlice('\n')
		if len(line)+len(fragment) > maxRPCIncomingLineBytes {
			return nil, errRPCFrameTooLarge
		}
		line = append(line, fragment...)
		if err == nil {
			return line, nil
		}
		if !errors.Is(err, bufio.ErrBufferFull) {
			return line, err
		}
	}
}

func (client *RPCClient) handleLine(line []byte) error {
	var envelope rpcEnvelope
	decoder := json.NewDecoder(bytes.NewReader(line))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil || requireEOF(decoder) != nil {
		return errors.New("app-server frame is invalid")
	}
	if envelope.Method != "" {
		if !rpcMethodPattern.MatchString(envelope.Method) {
			return errors.New("app-server RPC method is invalid")
		}
		if len(envelope.ID) > 0 && string(envelope.ID) != "null" {
			request := RPCServerRequest{ID: append(json.RawMessage(nil), envelope.ID...), Method: envelope.Method, Params: append(json.RawMessage(nil), envelope.Params...)}
			go client.handleServerRequest(request)
			return nil
		}
		client.mu.Lock()
		handler := client.onNotification
		client.mu.Unlock()
		if handler != nil {
			return handler(RPCNotification{Method: envelope.Method, Params: envelope.Params})
		}
		return nil
	}
	if len(envelope.ID) == 0 {
		return errors.New("app-server response is missing id")
	}
	var id uint64
	if err := json.Unmarshal(envelope.ID, &id); err != nil || id == 0 {
		return errors.New("app-server response id is invalid")
	}
	client.mu.Lock()
	pending := client.pending[id]
	delete(client.pending, id)
	client.mu.Unlock()
	if pending == nil {
		return nil
	}
	if envelope.Error != nil {
		message := envelope.Error.Message
		if len(message) > 4096 {
			message = message[:4096]
		}
		pending <- rpcResponse{err: &rpcError{Code: envelope.Error.Code, Message: message}}
		return nil
	}
	if len(envelope.Result) == 0 {
		pending <- rpcResponse{err: errors.New("app-server response has neither result nor error")}
		return nil
	}
	pending <- rpcResponse{result: envelope.Result}
	return nil
}

func (client *RPCClient) handleServerRequest(request RPCServerRequest) {
	client.mu.Lock()
	handler := client.onServerRequest
	client.mu.Unlock()
	if handler == nil {
		_ = client.write(map[string]any{"id": request.ID, "error": map[string]any{"code": -32601, "message": "unsupported server request"}})
		return
	}
	result, err := handler(context.Background(), request)
	if err != nil {
		message := err.Error()
		if len(message) > 1024 {
			message = message[:1024]
		}
		_ = client.write(map[string]any{"id": request.ID, "error": map[string]any{"code": -32000, "message": message}})
		return
	}
	_ = client.write(map[string]any{"id": request.ID, "result": result})
}

func (client *RPCClient) removePending(id uint64) {
	client.mu.Lock()
	delete(client.pending, id)
	client.mu.Unlock()
}

func (client *RPCClient) closeWithError(err error) {
	if err == nil {
		err = errors.New("app-server RPC client closed")
	}
	client.mu.Lock()
	if client.err != nil {
		client.mu.Unlock()
		return
	}
	client.err = err
	for id, pending := range client.pending {
		pending <- rpcResponse{err: err}
		delete(client.pending, id)
	}
	close(client.closed)
	client.mu.Unlock()
}
