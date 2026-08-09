package update

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"path"
	"time"
)

const maxProtocolBody = 64 << 10

type Client struct {
	socket string
	http   *http.Client
}

type HTTPError struct{ Status int }

func (e *HTTPError) Error() string { return fmt.Sprintf("updater status %d", e.Status) }

func NewClient(socket string) *Client {
	return &Client{socket: socket, http: &http.Client{Timeout: 8 * time.Second, Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socket)
	}}}}
}

func (c *Client) Status(ctx context.Context) (Status, error) {
	var out Status
	return out, c.do(ctx, http.MethodGet, "/v1/status", nil, &out)
}
func (c *Client) Check(ctx context.Context) (Status, error) {
	var out Status
	return out, c.do(ctx, http.MethodPost, "/v1/check", nil, &out)
}
func (c *Client) Install(ctx context.Context, in InstallRequest) (Job, error) {
	var out Job
	return out, c.do(ctx, http.MethodPost, "/v1/install", in, &out)
}
func (c *Client) Job(ctx context.Context, id string) (Job, error) {
	var out Job
	return out, c.do(ctx, http.MethodGet, path.Join("/v1/jobs", id), nil, &out)
}
func (c *Client) do(ctx context.Context, method, p string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://unix"+p, body)
	if err != nil {
		return err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxProtocolBody+1))
	if err != nil {
		return err
	}
	if len(b) > maxProtocolBody {
		return fmt.Errorf("updater response too large")
	}
	if resp.StatusCode/100 != 2 {
		return &HTTPError{Status: resp.StatusCode}
	}
	return json.Unmarshal(b, out)
}
