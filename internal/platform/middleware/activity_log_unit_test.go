package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	internalsharedconstant "github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/constant"
	"github.com/stretchr/testify/require"
)

// recordingRecorder keeps what it is given.
type recordingRecorder struct {
	mu  sync.Mutex
	got []Activity
}

func (r *recordingRecorder) Record(a Activity) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.got = append(r.got, a)
}

func TestActivityLog_U(t *testing.T) {
	sid := uuid.New()
	signedIn := func(c *gin.Context) {
		c.Set(internalsharedconstant.UserIDKey, int64(7))
		c.Set(internalsharedconstant.SessionIDKey, sid)
		c.Next()
	}

	testCases := []struct {
		name      string
		method    string
		target    string
		userAgent string
		auth      bool
		apiClient string // set as APIKey sets it (a machine call)
		// panics makes the handler panic (a recovery after ActivityLog answers 500).
		panics bool
		check  func(t *testing.T, got []Activity)
	}{
		{name: "a signed-in request carries user, session, route, status, size and the request id", method: http.MethodGet,
			target: "/api/notes/42", userAgent: "Mozilla/5.0", auth: true, check: func(t *testing.T, got []Activity) {
				require.Len(t, got, 1)
				a := got[0]
				require.Equal(t, int64(7), a.UserID)
				require.Equal(t, sid, a.SessionID)
				require.Equal(t, http.MethodGet, a.Method)
				require.Equal(t, "/api/notes/:id", a.Route)
				require.Equal(t, "/api/notes/42", a.Path)
				require.Equal(t, http.StatusTeapot, a.Status)
				require.Equal(t, len("short"), a.ResponseBytes)
				require.Equal(t, "req-1", a.RequestID)
				require.Equal(t, "Mozilla/5.0", a.UserAgent)
				require.Equal(t, "192.0.2.1", a.ClientIP)
				require.WithinDuration(t, time.Now(), a.OccurredAt, 5*time.Second)
			}},
		{name: "an anonymous request has no user, session or API client", method: http.MethodGet, target: "/api/notes/42", check: func(t *testing.T, got []Activity) {
			require.Len(t, got, 1)
			require.Zero(t, got[0].UserID)
			require.Equal(t, uuid.Nil, got[0].SessionID)
			require.Empty(t, got[0].APIClient)
		}},
		{name: "a machine call carries the API client's name, never its key", method: http.MethodGet, target: "/api/notes/42",
			apiClient: "billing", check: func(t *testing.T, got []Activity) {
				require.Len(t, got, 1)
				require.Equal(t, "billing", got[0].APIClient)
			}},
		{name: "a long API client name is cut to the column size in runes", method: http.MethodGet, target: "/api/notes/42",
			apiClient: strings.Repeat("ß", 150), check: func(t *testing.T, got []Activity) {
				require.Len(t, []rune(got[0].APIClient), 100)
			}},
		{name: "a 404 has an empty route", method: http.MethodGet, target: "/api/nope", check: func(t *testing.T, got []Activity) {
			require.Len(t, got, 1)
			require.Empty(t, got[0].Route)
			require.Equal(t, http.StatusNotFound, got[0].Status)
			require.Equal(t, "/api/nope", got[0].Path)
		}},
		{name: "the query string is not kept", method: http.MethodGet, target: "/api/notes/42?token=secret", check: func(t *testing.T, got []Activity) {
			require.Equal(t, "/api/notes/42", got[0].Path)
		}},
		{name: "long values are cut to the column size in runes", method: http.MethodGet,
			target: "/api/long/" + strings.Repeat("é", 3000), userAgent: strings.Repeat("ü", 600), check: func(t *testing.T, got []Activity) {
				require.Len(t, []rune(got[0].Path), 2048)
				require.Len(t, []rune(got[0].UserAgent), 512)
			}},
		{name: "a panic is recorded as the 500 recovery wrote", method: http.MethodGet, target: "/api/notes/42", panics: true,
			check: func(t *testing.T, got []Activity) {
				require.Len(t, got, 1)
				require.Equal(t, http.StatusInternalServerError, got[0].Status)
			}},
		{name: "healthz is not recorded", method: http.MethodGet, target: "/healthz", check: func(t *testing.T, got []Activity) {
			require.Empty(t, got)
		}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			rec := &recordingRecorder{}
			r := gin.New()
			r.Use(func(c *gin.Context) { c.Header("X-Request-ID", "req-1"); c.Next() }, ActivityLog(rec),
				gin.CustomRecovery(func(c *gin.Context, _ any) { c.AbortWithStatus(http.StatusInternalServerError) }))
			handler := func(c *gin.Context) {
				if tc.panics {
					panic("boom")
				}
				c.String(http.StatusTeapot, "short")
			}
			switch {
			case tc.auth:
				r.GET("/api/notes/:id", signedIn, handler)
			case tc.apiClient != "":
				r.GET("/api/notes/:id", func(c *gin.Context) { c.Set(APIClientKey, tc.apiClient); c.Next() }, handler)
			default:
				r.GET("/api/notes/:id", handler)
			}
			r.GET("/api/long/*rest", handler)
			r.GET("/healthz", handler)

			req := httptest.NewRequest(tc.method, tc.target, nil)
			req.RemoteAddr = "192.0.2.1:1234"
			if tc.userAgent != "" {
				req.Header.Set("User-Agent", tc.userAgent)
			}
			r.ServeHTTP(httptest.NewRecorder(), req)
			tc.check(t, rec.got)
		})
	}
}
