package middleware

import (
	"context"
	"errors"
	"log/slog"
	"regexp"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/requestid"
)

// validRequestID is what an incoming X-Request-ID must look like to be kept.
// Anything else (spaces, newlines, quotes, too long) could forge or break log
// lines, so it's replaced.
var validRequestID = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)

// RequestID gives every request an id: the incoming X-Request-ID when it's
// well-formed (a proxy's, so both logs share it), else a new UUID. It's set
// on the response header before the handlers run (so every response carries
// it, errors and panics included), in the gin context under requestid.Key and
// in the request context. Mount it first.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(requestid.Header)
		if !validRequestID.MatchString(id) {
			id = uuid.NewString()
		}

		c.Header(requestid.Header, id)
		c.Set(requestid.Key, id)
		c.Request = c.Request.WithContext(requestid.With(c.Request.Context(), id))

		c.Next()
	}
}

// AccessLog writes one "request" record per request once it's been served.
// It logs with c.Request.Context() read after c.Next, so the record gets
// user_id from logging's handler once the auth middleware (which runs later)
// put the caller on it. A request the client abandoned
// (its context cancelled) is warn with cancelled=true whatever the status;
// otherwise error level for a 5xx, info for the rest. Mount it after
// RequestID (so the record carries request_id) and before the recovery (so it
// sees a panic's 500).
func AccessLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		method, path := c.Request.Method, c.Request.URL.Path

		c.Next()

		ctx := c.Request.Context()
		status := c.Writer.Status()
		args := []any{
			"method", method,
			"path", path,
			"status", status,
			"latency_ms", float64(time.Since(start).Microseconds()) / 1000,
			"client_ip", c.ClientIP(),
			"bytes", max(c.Writer.Size(), 0),
		}

		level := slog.LevelInfo
		switch {
		case errors.Is(ctx.Err(), context.Canceled):
			level = slog.LevelWarn
			args = append(args, "cancelled", true)
		case status >= 500:
			level = slog.LevelError
		}
		slog.Log(ctx, level, "request", args...)
	}
}
