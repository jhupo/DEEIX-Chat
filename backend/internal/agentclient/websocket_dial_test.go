package agentclient

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/net/websocket"
)

func TestDialBridgeWebSocketThroughHTTPProxy(t *testing.T) {
	target := httptest.NewServer(websocket.Handler(func(connection *websocket.Conn) {
		defer connection.Close()
		var message string
		if err := websocket.Message.Receive(connection, &message); err == nil {
			_ = websocket.Message.Send(connection, "echo:"+message)
		}
	}))
	defer target.Close()

	var connectCount atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodConnect {
			http.Error(response, "CONNECT required", http.StatusMethodNotAllowed)
			return
		}
		upstream, err := net.DialTimeout("tcp", request.Host, 5*time.Second)
		if err != nil {
			http.Error(response, "upstream unavailable", http.StatusBadGateway)
			return
		}
		hijacker, ok := response.(http.Hijacker)
		if !ok {
			upstream.Close()
			http.Error(response, "hijacking unavailable", http.StatusInternalServerError)
			return
		}
		client, buffered, err := hijacker.Hijack()
		if err != nil {
			upstream.Close()
			return
		}
		connectCount.Add(1)
		_, _ = fmt.Fprint(buffered, "HTTP/1.1 200 Connection Established\r\n\r\n")
		_ = buffered.Flush()
		clientWithBufferedInput := &bufferedNetConn{Conn: client, reader: buffered.Reader}
		go func() {
			defer upstream.Close()
			defer client.Close()
			_, _ = io.Copy(upstream, clientWithBufferedInput)
		}()
		_, _ = io.Copy(client, upstream)
		_ = client.Close()
		_ = upstream.Close()
	}))
	defer proxy.Close()

	endpoint := "ws" + strings.TrimPrefix(target.URL, "http")
	config, err := websocket.NewConfig(endpoint, target.URL)
	if err != nil {
		t.Fatalf("create websocket config: %v", err)
	}
	proxyURL, err := url.Parse(proxy.URL)
	if err != nil {
		t.Fatalf("parse proxy URL: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	connection, err := dialBridgeWebSocketWithProxy(ctx, config, proxyURL)
	if err != nil {
		t.Fatalf("dial websocket through proxy: %v", err)
	}
	defer connection.Close()

	if err = websocket.Message.Send(connection, "ready"); err != nil {
		t.Fatalf("send websocket message: %v", err)
	}
	var reply string
	if err = websocket.Message.Receive(connection, &reply); err != nil {
		t.Fatalf("receive websocket message: %v", err)
	}
	if reply != "echo:ready" {
		t.Fatalf("unexpected websocket reply %q", reply)
	}
	if connectCount.Load() != 1 {
		t.Fatalf("expected one proxy CONNECT, got %d", connectCount.Load())
	}
}
