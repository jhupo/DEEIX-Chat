package sub2api

import "context"

type requestHostContextKey struct{}

func WithRequestHost(ctx context.Context, host string) context.Context {
	return context.WithValue(ctx, requestHostContextKey{}, host)
}

func RequestHost(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	value, _ := ctx.Value(requestHostContextKey{}).(string)
	return value
}
