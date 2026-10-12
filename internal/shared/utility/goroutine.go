package utility

import (
	"context"
	"log/slog"
	"runtime/debug"
)

// SafeGo runs fn in a goroutine; a panic is recovered and logged instead of
// crashing the process. Prefer SafeGoContext when there's a ctx.
func SafeGo(fn func()) {
	SafeGoContext(context.Background(), func(context.Context) { fn() })
}

// SafeGoContext runs fn(ctx) in a goroutine; a panic is recovered and logged
// with ctx (so it carries the request id). From a request, pass
// requestid.Detach(ctx): the request's own context is cancelled — and a
// *gin.Context reused — once the response is written.
func SafeGoContext(ctx context.Context, fn func(ctx context.Context)) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.ErrorContext(ctx, "panic recovered", "panic", r, "stack", string(debug.Stack()))
			}
		}()
		fn(ctx)
	}()
}
