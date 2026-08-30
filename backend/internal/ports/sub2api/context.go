package sub2api

import (
	"context"
	"strings"
	"sync"
)

type requestHostContextKey struct{}

type ConnectorEndpoint struct {
	PublicID       string
	Protocol       string
	AccountBaseURL string
	ModelBaseURL   string
}

type requestScope struct {
	host      string
	once      sync.Once
	connector ConnectorEndpoint
	err       error
}

func WithRequestHost(ctx context.Context, host string) context.Context {
	return context.WithValue(ctx, requestHostContextKey{}, &requestScope{host: strings.TrimSpace(host)})
}

func RequestHost(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	scope, _ := ctx.Value(requestHostContextKey{}).(*requestScope)
	if scope == nil {
		return ""
	}
	return scope.host
}

func ResolveRequestConnector(
	ctx context.Context,
	resolve func(context.Context, string) (ConnectorEndpoint, error),
) (ConnectorEndpoint, error) {
	if ctx == nil || resolve == nil {
		return ConnectorEndpoint{}, ErrConnectorUnavailable
	}
	scope, _ := ctx.Value(requestHostContextKey{}).(*requestScope)
	if scope == nil || scope.host == "" {
		return ConnectorEndpoint{}, ErrConnectorUnavailable
	}
	scope.once.Do(func() {
		scope.connector, scope.err = resolve(ctx, scope.host)
	})
	return scope.connector, scope.err
}
