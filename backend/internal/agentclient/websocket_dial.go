package agentclient

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/http/httpproxy"
	xproxy "golang.org/x/net/proxy"
	"golang.org/x/net/websocket"
)

func dialBridgeWebSocket(ctx context.Context, config *websocket.Config) (*websocket.Conn, error) {
	requestURL := *config.Location
	switch requestURL.Scheme {
	case "ws":
		requestURL.Scheme = "http"
	case "wss":
		requestURL.Scheme = "https"
	default:
		return nil, fmt.Errorf("unsupported gateway websocket scheme %q", requestURL.Scheme)
	}
	proxyURL, err := httpproxy.FromEnvironment().ProxyFunc()(&requestURL)
	if err != nil {
		return nil, fmt.Errorf("resolve gateway proxy: %w", err)
	}
	return dialBridgeWebSocketWithProxy(ctx, config, proxyURL)
}

func dialBridgeWebSocketWithProxy(ctx context.Context, config *websocket.Config, proxyURL *url.URL) (*websocket.Conn, error) {
	targetAddress, err := websocketAddress(config.Location)
	if err != nil {
		return nil, err
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}

	var connection net.Conn
	if proxyURL == nil {
		connection, err = dialer.DialContext(ctx, "tcp", targetAddress)
	} else {
		connection, err = dialProxyTunnel(ctx, dialer, proxyURL, targetAddress)
	}
	if err != nil {
		return nil, err
	}
	success := false
	defer func() {
		if !success {
			_ = connection.Close()
		}
	}()

	if config.Location.Scheme == "wss" {
		tlsConnection := tls.Client(connection, &tls.Config{MinVersion: tls.VersionTLS12, ServerName: config.Location.Hostname()})
		if err = tlsConnection.HandshakeContext(ctx); err != nil {
			return nil, fmt.Errorf("gateway TLS handshake: %w", err)
		}
		connection = tlsConnection
	}

	client, err := newWebSocketClientContext(ctx, config, connection)
	if err != nil {
		return nil, err
	}
	_ = client.SetDeadline(time.Time{})
	success = true
	return client, nil
}

func websocketAddress(location *url.URL) (string, error) {
	if location == nil || location.Hostname() == "" {
		return "", fmt.Errorf("gateway websocket location is invalid")
	}
	port := location.Port()
	if port == "" {
		switch location.Scheme {
		case "ws":
			port = "80"
		case "wss":
			port = "443"
		default:
			return "", fmt.Errorf("unsupported gateway websocket scheme %q", location.Scheme)
		}
	}
	return net.JoinHostPort(location.Hostname(), port), nil
}

func dialProxyTunnel(ctx context.Context, dialer *net.Dialer, proxyURL *url.URL, targetAddress string) (net.Conn, error) {
	switch strings.ToLower(proxyURL.Scheme) {
	case "http", "https":
		return dialHTTPProxyTunnel(ctx, dialer, proxyURL, targetAddress)
	case "socks5", "socks5h":
		proxyDialer, err := xproxy.FromURL(proxyURL, dialer)
		if err != nil {
			return nil, fmt.Errorf("configure gateway SOCKS proxy: %w", err)
		}
		contextDialer, ok := proxyDialer.(xproxy.ContextDialer)
		if !ok {
			return nil, fmt.Errorf("gateway SOCKS proxy does not support cancellation")
		}
		connection, err := contextDialer.DialContext(ctx, "tcp", targetAddress)
		if err != nil {
			return nil, fmt.Errorf("connect gateway SOCKS proxy: %w", err)
		}
		return connection, nil
	default:
		return nil, fmt.Errorf("unsupported gateway proxy scheme %q", proxyURL.Scheme)
	}
}

func dialHTTPProxyTunnel(ctx context.Context, dialer *net.Dialer, proxyURL *url.URL, targetAddress string) (net.Conn, error) {
	proxyAddress := proxyURL.Host
	if proxyURL.Port() == "" {
		port := "80"
		if strings.EqualFold(proxyURL.Scheme, "https") {
			port = "443"
		}
		proxyAddress = net.JoinHostPort(proxyURL.Hostname(), port)
	}
	connection, err := dialer.DialContext(ctx, "tcp", proxyAddress)
	if err != nil {
		return nil, fmt.Errorf("connect gateway HTTP proxy: %w", err)
	}
	success := false
	defer func() {
		if !success {
			_ = connection.Close()
		}
	}()

	if strings.EqualFold(proxyURL.Scheme, "https") {
		tlsConnection := tls.Client(connection, &tls.Config{MinVersion: tls.VersionTLS12, ServerName: proxyURL.Hostname()})
		if err = tlsConnection.HandshakeContext(ctx); err != nil {
			return nil, fmt.Errorf("gateway proxy TLS handshake: %w", err)
		}
		connection = tlsConnection
	}

	request := &http.Request{
		Method: http.MethodConnect,
		URL:    &url.URL{Opaque: targetAddress},
		Host:   targetAddress,
		Header: make(http.Header),
	}
	if proxyURL.User != nil {
		password, _ := proxyURL.User.Password()
		credential := base64.StdEncoding.EncodeToString([]byte(proxyURL.User.Username() + ":" + password))
		request.Header.Set("Proxy-Authorization", "Basic "+credential)
	}
	if err = request.Write(connection); err != nil {
		return nil, fmt.Errorf("write gateway proxy CONNECT: %w", err)
	}
	reader := bufio.NewReader(connection)
	response, err := http.ReadResponse(reader, request)
	if err != nil {
		return nil, fmt.Errorf("read gateway proxy CONNECT: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		_ = response.Body.Close()
		return nil, fmt.Errorf("gateway proxy CONNECT returned %s", response.Status)
	}

	success = true
	return &bufferedNetConn{Conn: connection, reader: reader}, nil
}

type bufferedNetConn struct {
	net.Conn
	reader *bufio.Reader
}

func (connection *bufferedNetConn) Read(buffer []byte) (int, error) {
	return connection.reader.Read(buffer)
}

func newWebSocketClientContext(ctx context.Context, config *websocket.Config, connection net.Conn) (*websocket.Conn, error) {
	type result struct {
		client *websocket.Conn
		err    error
	}
	resultChannel := make(chan result, 1)
	go func() {
		client, err := websocket.NewClient(config, connection)
		resultChannel <- result{client: client, err: err}
	}()
	select {
	case <-ctx.Done():
		_ = connection.SetDeadline(time.Now())
		<-resultChannel
		return nil, fmt.Errorf("gateway websocket handshake: %w", ctx.Err())
	case outcome := <-resultChannel:
		if outcome.err != nil {
			return nil, fmt.Errorf("gateway websocket handshake: %w", outcome.err)
		}
		return outcome.client, nil
	}
}
