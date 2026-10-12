// Package requestid carries the per-request id (X-Request-ID) through a
// context so every log line a request writes can be tied back to it.
// middleware.RequestID sets it; logging's handler reads it.
package requestid

import (
	"context"

	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/requestuser"
)

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

// Detach returns a fresh context.Background() carrying only ctx's request id
// and caller (requestuser): no deadline, no cancellation, no other values.
// Use it for a goroutine that outlives the request, so its logs keep both
// ids. Never hand a goroutine the *gin.Context, nor context.WithoutCancel of
// it — gin reuses it for the next request.
func Detach(ctx context.Context) context.Context {
	detached := context.Background()
	if id := FromContext(ctx); id != "" {
		detached = With(detached, id)
	}
	if userID, ok := requestuser.FromContext(ctx); ok {
		detached = requestuser.With(detached, userID)
	}
	return detached
}
