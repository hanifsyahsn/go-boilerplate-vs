package utility

import (
	"context"
	"log/slog"
	"runtime/debug"
	"sync/atomic"
)

// panicHook is told about every recovered panic (main sets it to send an ops
// alert); nil until set.
var panicHook atomic.Pointer[func(ctx context.Context, recovered any)]

// SetPanicHook sets what ReportPanic calls; main sets it once, to alert the
// people running the service (decision 0011).
func SetPanicHook(fn func(ctx context.Context, recovered any)) {
	panicHook.Store(&fn)
}

// ReportPanic hands a recovered panic to the hook, if one is set. Called by
// SafeGoContext's recover and by the router's recovery.
func ReportPanic(ctx context.Context, recovered any) {
	if h := panicHook.Load(); h != nil && *h != nil {
		(*h)(ctx, recovered)
	}
}

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
				ReportPanic(ctx, r)
			}
		}()
		fn(ctx)
	}()
}
