package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/requestid"
	"github.com/stretchr/testify/require"
)

func TestCheck_U(t *testing.T) {
	testCases := []struct {
		name    string
		format  string
		level   string
		wantErr []string
	}{
		{name: "defaults", format: "", level: ""},
		{name: "text / debug", format: "text", level: "debug"},
		{name: "json / error, any case", format: "JSON", level: "Error"},
		{name: "warn", format: "json", level: "warn"},
		{name: "an unknown format", format: "xml", level: "info", wantErr: []string{`log format "xml"`}},
		{name: "an unknown level", format: "text", level: "loud", wantErr: []string{`log level "loud"`}},
		{name: "both wrong", format: "xml", level: "loud", wantErr: []string{"log format", "log level"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := Check(tc.format, tc.level)
			_, newErr := New(&bytes.Buffer{}, tc.format, tc.level)
			if len(tc.wantErr) == 0 {
				require.NoError(t, err)
				require.NoError(t, newErr)
				return
			}
			for _, want := range tc.wantErr {
				require.ErrorContains(t, err, want)
				require.ErrorContains(t, newErr, want)
			}
		})
	}
}

func TestNew_FormatAndLevel_U(t *testing.T) {
	testCases := []struct {
		name      string
		format    string
		level     string
		log       func(l *slog.Logger)
		wantEmpty bool
		check     func(t *testing.T, out string)
	}{
		{
			name: "text by default", format: "", level: "",
			log: func(l *slog.Logger) { l.Info("hello", "k", "v") },
			check: func(t *testing.T, out string) {
				require.Contains(t, out, "level=INFO")
				require.Contains(t, out, "msg=hello")
				require.Contains(t, out, "k=v")
			},
		},
		{
			name: "json", format: "json", level: "info",
			log: func(l *slog.Logger) { l.Warn("hello", "k", "v") },
			check: func(t *testing.T, out string) {
				rec := decode(t, out)
				require.Equal(t, "WARN", rec["level"])
				require.Equal(t, "hello", rec["msg"])
				require.Equal(t, "v", rec["k"])
			},
		},
		{
			name: "below the level is dropped", format: "json", level: "warn",
			log:       func(l *slog.Logger) { l.Info("quiet") },
			wantEmpty: true,
		},
		{
			name: "debug enabled at debug", format: "json", level: "debug",
			log: func(l *slog.Logger) { l.Debug("loud") },
			check: func(t *testing.T, out string) {
				require.Equal(t, "DEBUG", decode(t, out)["level"])
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			l, err := New(&buf, tc.format, tc.level)
			require.NoError(t, err)

			tc.log(l)

			if tc.wantEmpty {
				require.Empty(t, buf.String())
				return
			}
			tc.check(t, buf.String())
		})
	}
}

func TestHandler_RequestID_U(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ginCtx := func() context.Context {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Set(requestid.Key, "gin-id")
		return c
	}

	testCases := []struct {
		name   string
		ctx    func() context.Context
		log    func(l *slog.Logger, ctx context.Context)
		wantID string // "" => no request_id attribute
		check  func(t *testing.T, rec map[string]any)
	}{
		{
			name:   "no id in the context",
			ctx:    context.Background,
			log:    func(l *slog.Logger, ctx context.Context) { l.InfoContext(ctx, "m") },
			wantID: "",
		},
		{
			name:   "a plain context",
			ctx:    func() context.Context { return requestid.With(context.Background(), "plain-id") },
			log:    func(l *slog.Logger, ctx context.Context) { l.InfoContext(ctx, "m") },
			wantID: "plain-id",
		},
		{
			name:   "a *gin.Context",
			ctx:    ginCtx,
			log:    func(l *slog.Logger, ctx context.Context) { l.ErrorContext(ctx, "m") },
			wantID: "gin-id",
		},
		{
			name:   "a detached context",
			ctx:    func() context.Context { return requestid.Detach(ginCtx()) },
			log:    func(l *slog.Logger, ctx context.Context) { l.InfoContext(ctx, "m") },
			wantID: "gin-id",
		},
		{
			name: "a logger from With keeps the wrapper",
			ctx:  func() context.Context { return requestid.With(context.Background(), "with-id") },
			log: func(l *slog.Logger, ctx context.Context) {
				l.With("worker", "mailer").InfoContext(ctx, "m")
			},
			wantID: "with-id",
			check: func(t *testing.T, rec map[string]any) {
				require.Equal(t, "mailer", rec["worker"])
			},
		},
		{
			name: "a logger from WithGroup keeps the wrapper",
			ctx:  func() context.Context { return requestid.With(context.Background(), "group-id") },
			log: func(l *slog.Logger, ctx context.Context) {
				l.WithGroup("job").InfoContext(ctx, "m", "id", 7)
			},
			check: func(t *testing.T, rec map[string]any) {
				group, ok := rec["job"].(map[string]any)
				require.True(t, ok, rec)
				require.Equal(t, float64(7), group["id"])
				require.Equal(t, "group-id", group[requestid.Key])
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			l, err := New(&buf, FormatJSON, "debug")
			require.NoError(t, err)

			tc.log(l, tc.ctx())

			rec := decode(t, buf.String())
			if tc.wantID != "" {
				require.Equal(t, tc.wantID, rec[requestid.Key])
			} else if tc.check == nil {
				require.NotContains(t, rec, requestid.Key)
			}
			if tc.check != nil {
				tc.check(t, rec)
			}
		})
	}
}

func TestSetup_U(t *testing.T) {
	testCases := []struct {
		name    string
		format  string
		level   string
		wantErr bool
	}{
		{name: "valid: becomes the default", format: "json", level: "warn"},
		{name: "invalid: the default is kept", format: "xml", level: "info", wantErr: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			prev := slog.Default()
			defer slog.SetDefault(prev)

			err := Setup(tc.format, tc.level)
			if tc.wantErr {
				require.Error(t, err)
				require.Same(t, prev, slog.Default())
				return
			}
			require.NoError(t, err)
			require.NotSame(t, prev, slog.Default())
			require.False(t, slog.Default().Enabled(context.Background(), slog.LevelInfo))
			require.True(t, slog.Default().Enabled(context.Background(), slog.LevelWarn))
		})
	}
}

// decode parses the single JSON record in out.
func decode(t *testing.T, out string) map[string]any {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(out), "\n")
	require.Len(t, lines, 1, out)
	var rec map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &rec), out)
	return rec
}

// TestFatal_U runs Fatal in a child process (it calls os.Exit) and checks the
// exit status and the error record.
func TestFatal_U(t *testing.T) {
	if os.Getenv("LOGGING_FATAL_CHILD") == "1" {
		l, err := New(os.Stdout, FormatJSON, "info")
		if err != nil {
			os.Exit(2)
		}
		slog.SetDefault(l)
		Fatal("load config failed", "err", "boom")
		return
	}

	testCases := []struct {
		name     string
		wantCode int
		wantMsg  string
	}{
		{name: "logs at error and exits 1", wantCode: 1, wantMsg: "load config failed"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestFatal_U$")
			cmd.Env = append(os.Environ(), "LOGGING_FATAL_CHILD=1")
			out, err := cmd.Output()

			var exitErr *exec.ExitError
			require.ErrorAs(t, err, &exitErr)
			require.Equal(t, tc.wantCode, exitErr.ExitCode())

			rec := decode(t, string(out))
			require.Equal(t, "ERROR", rec["level"])
			require.Equal(t, tc.wantMsg, rec["msg"])
			require.Equal(t, "boom", rec["err"])
		})
	}
}
