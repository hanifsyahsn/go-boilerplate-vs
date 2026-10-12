package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/logging"
	internalsharedconstant "github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/constant"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/requestid"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/requestuser"
	"github.com/stretchr/testify/require"
)

func TestRequestID_U(t *testing.T) {
	testCases := []struct {
		name     string
		incoming string // "" => no header
		wantKept bool
	}{
		{name: "no header: a new UUID"},
		{name: "a well-formed id is kept", incoming: "3f2a9c1e-7b4d-4e0a-9f1b-2c3d4e5f6a7b", wantKept: true},
		{name: "letters, digits and ._:- are kept", incoming: "nginx.abc_DEF:123-x", wantKept: true},
		{name: "128 characters are kept", incoming: strings.Repeat("a", 128), wantKept: true},
		{name: "129 characters are replaced", incoming: strings.Repeat("a", 129)},
		{name: "a space is replaced", incoming: "abc def"},
		{name: "a quote is replaced", incoming: `abc"def`},
		{name: "a forged log line is replaced", incoming: "abc\nlevel=ERROR msg=forged"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.Use(RequestID())

			var ginID, ctxID, headerInHandler string
			r.GET("/x", func(c *gin.Context) {
				ginID = c.GetString(requestid.Key)
				ctxID = requestid.FromContext(c.Request.Context())
				headerInHandler = c.Writer.Header().Get(requestid.Header)
				c.Status(http.StatusTeapot)
			})

			req := httptest.NewRequest(http.MethodGet, "/x", nil)
			if tc.incoming != "" {
				req.Header.Set(requestid.Header, tc.incoming)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			got := w.Header().Get(requestid.Header)
			if tc.wantKept {
				require.Equal(t, tc.incoming, got)
			} else {
				_, err := uuid.Parse(got)
				require.NoError(t, err, "a UUID, got %q", got)
			}
			require.Equal(t, got, headerInHandler, "set before the handler runs")
			require.Equal(t, got, ginID)
			require.Equal(t, got, ctxID)
		})
	}
}

func TestAccessLog_U(t *testing.T) {
	testCases := []struct {
		name      string
		status    int
		body      string
		target    string
		userID    int64 // set by the "auth" handler when non-zero, as AccessTokenMiddleware does
		ginOnly   bool  // set it with c.Set alone, not on the request context
		cancelled bool  // the client went away
		wantLevel string
	}{
		{name: "a 200 at info", status: http.StatusOK, body: "hello", target: "/x?secret=1", wantLevel: "INFO"},
		{name: "a 404 at info", status: http.StatusNotFound, target: "/x", wantLevel: "INFO"},
		{name: "a 500 at error", status: http.StatusInternalServerError, body: "oops", target: "/x", wantLevel: "ERROR"},
		{name: "an authenticated caller's user_id", status: http.StatusOK, target: "/x", userID: 7, wantLevel: "INFO"},
		{name: "a cancelled 500 is warn", status: http.StatusInternalServerError, target: "/x", userID: 7, cancelled: true, wantLevel: "WARN"},
		{name: "a cancelled 200 is warn too", status: http.StatusOK, target: "/x", cancelled: true, wantLevel: "WARN"},
		{name: "a gin key alone isn't on the request context: no user_id", status: http.StatusOK, target: "/x", userID: 7, ginOnly: true, wantLevel: "INFO"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			l, err := logging.New(&buf, logging.FormatJSON, "info")
			require.NoError(t, err)
			prev := slog.Default()
			slog.SetDefault(l)
			defer slog.SetDefault(prev)

			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.Use(RequestID(), AccessLog())
			r.GET("/x", func(c *gin.Context) {
				if tc.userID != 0 {
					c.Set(internalsharedconstant.UserIdKey, tc.userID)
					if !tc.ginOnly {
						c.Request = c.Request.WithContext(requestuser.With(c.Request.Context(), tc.userID))
					}
				}
				c.String(tc.status, tc.body)
			})

			req := httptest.NewRequest(http.MethodGet, tc.target, nil)
			if tc.cancelled {
				ctx, cancel := context.WithCancel(req.Context())
				cancel()
				req = req.WithContext(ctx)
			}
			req.Header.Set(requestid.Header, "req-1")
			req.RemoteAddr = "203.0.113.7:5555"
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			line := bytes.TrimSpace(buf.Bytes())
			var rec map[string]any
			require.NoError(t, json.Unmarshal(line, &rec), buf.String())
			require.Equal(t, tc.wantLevel, rec["level"])
			require.Equal(t, "request", rec["msg"])
			require.Equal(t, http.MethodGet, rec["method"])
			require.Equal(t, "/x", rec["path"], "no query string")
			require.Equal(t, float64(tc.status), rec["status"])
			require.Equal(t, "203.0.113.7", rec["client_ip"])
			require.Equal(t, float64(len(tc.body)), rec["bytes"])
			require.Contains(t, rec, "latency_ms")
			require.Equal(t, "req-1", rec[requestid.Key])
			if tc.userID != 0 && !tc.ginOnly {
				require.Equal(t, float64(tc.userID), rec["user_id"])
				require.Equal(t, 1, bytes.Count(line, []byte(`"user_id"`)), "user_id logged exactly once")
			} else {
				require.NotContains(t, rec, "user_id")
			}
			if tc.cancelled {
				require.Equal(t, true, rec["cancelled"])
			} else {
				require.NotContains(t, rec, "cancelled")
			}
		})
	}
}
