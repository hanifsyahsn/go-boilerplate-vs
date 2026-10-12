// Package requestid carries the per-request id (X-Request-ID) through a
// context so every log line a request writes can be tied back to it.
// middleware.RequestID sets it; logging's handler reads it.
package requestid

import "context"

// Header is the request / response header that carries the id.
const Header = "X-Request-ID"

// Key is the gin context key (c.Set / c.Get) and the log attribute name.
const Key = "request_id"

type ctxKey struct{}

// With returns a copy of ctx carrying id.
func With(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// FromContext returns the id in ctx, or "" when there is none. It checks the
// typed key first (a request context, a Detach-ed one), then ctx.Value(Key),
// so a *gin.Context handed to a service works too.
func FromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if id, ok := ctx.Value(ctxKey{}).(string); ok && id != "" {
		return id
	}
	if id, ok := ctx.Value(Key).(string); ok {
		return id
	}
	return ""
}

// Detach returns a fresh context.Background() carrying only ctx's id: no
// deadline, no cancellation, no values. Use it for a goroutine that outlives
// the request. Never use context.WithoutCancel on a *gin.Context instead —
// gin reuses it for the next request.
func Detach(ctx context.Context) context.Context {
	id := FromContext(ctx)
	if id == "" {
		return context.Background()
	}
	return With(context.Background(), id)
}
