// Package logging configures log/slog: text (development) or JSON
// (production) output, a minimum level, and a request_id attribute on every
// record logged with a request's context (requestid.FromContext).
//
// Log with the *Context variants (slog.ErrorContext(ctx, ...)) whenever a
// ctx is in scope — that's where the request id comes from. Messages are
// short and lowercase; the details go in key/value attributes:
//
//	slog.ErrorContext(ctx, "notify: get user failed", "user_id", id, "err", err)
package logging

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/requestid"
)

const (
	FormatText = "text"
	FormatJSON = "json"
)

// Check validates a format / level pair as New would accept it: format is
// text or json, level is debug, info, warn or error (case-insensitive).
// Empty means the default (text, info).
func Check(format, level string) error {
	_, fErr := parseFormat(format)
	_, lErr := parseLevel(level)
	return errors.Join(fErr, lErr)
}

// New returns a logger writing to w in format at level and above.
func New(w io.Writer, format, level string) (*slog.Logger, error) {
	f, fErr := parseFormat(format)
	l, lErr := parseLevel(level)
	if err := errors.Join(fErr, lErr); err != nil {
		return nil, err
	}

	opts := &slog.HandlerOptions{Level: l}
	var h slog.Handler
	if f == FormatJSON {
		h = slog.NewJSONHandler(w, opts)
	} else {
		h = slog.NewTextHandler(w, opts)
	}
	return slog.New(handler{h}), nil
}

// Setup makes a logger writing to stdout the slog default.
func Setup(format, level string) error {
	l, err := New(os.Stdout, format, level)
	if err != nil {
		return err
	}
	slog.SetDefault(l)
	return nil
}

// Fatal logs msg at error level and exits with status 1. Deferred functions
// don't run.
func Fatal(msg string, args ...any) {
	slog.Error(msg, args...)
	os.Exit(1)
}

func parseFormat(format string) (string, error) {
	switch f := strings.ToLower(strings.TrimSpace(format)); f {
	case "":
		return FormatText, nil
	case FormatText, FormatJSON:
		return f, nil
	default:
		return "", fmt.Errorf("log format %q: want text or json", format)
	}
}

func parseLevel(level string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("log level %q: want debug, info, warn or error", level)
	}
}

// handler adds request_id from the record's context. WithAttrs / WithGroup
// re-wrap, so a logger derived with .With(...) keeps adding it.
type handler struct {
	slog.Handler
}

func (h handler) Handle(ctx context.Context, r slog.Record) error {
	if id := requestid.FromContext(ctx); id != "" {
		r.AddAttrs(slog.String(requestid.Key, id))
	}
	return h.Handler.Handle(ctx, r)
}

func (h handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return handler{h.Handler.WithAttrs(attrs)}
}

func (h handler) WithGroup(name string) slog.Handler {
	return handler{h.Handler.WithGroup(name)}
}
