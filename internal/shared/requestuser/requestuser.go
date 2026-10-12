// Package requestuser carries the authenticated caller's id through a context
// so every log line a request writes says who made it (user_id), the same way
// requestid adds request_id. middleware.AccessTokenMiddleware sets it;
// logging's handler reads it.
package requestuser

import (
	"context"

	internalsharedconstant "github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/constant"
)

type ctxKey struct{}

// With returns a copy of ctx carrying the caller's id.
func With(ctx context.Context, id int64) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// FromContext returns the caller's id, false outside an authenticated request
// (or for a nil ctx). It checks the typed key first (a request context, a
// detached one), then the gin key the auth middleware sets, so a
// *gin.Context handed to a service works too.
func FromContext(ctx context.Context) (int64, bool) {
	if ctx == nil {
		return 0, false
	}
	if id, ok := ctx.Value(ctxKey{}).(int64); ok {
		return id, true
	}
	if id, ok := ctx.Value(internalsharedconstant.UserIdKey).(int64); ok {
		return id, true
	}
	return 0, false
}
