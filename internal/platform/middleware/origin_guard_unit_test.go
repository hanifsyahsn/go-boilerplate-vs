package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/logging"
	"github.com/stretchr/testify/require"
)

func TestOriginGuard_U(t *testing.T) {
	const forbidden = `{"jsonapi":{"version":"1.1"},"errors":[{"status":"403","code":"forbidden","title":"Forbidden",
		"detail":"This request didn't come from an allowed origin.","source":{"header":"Origin"}}]}`
	listed := []string{"https://app.example.com"}

	testCases := []struct {
		name    string
		allowed []string
		method  string
		// origins are the Origin header values sent; nil = no header, {""} = an empty one.
		origins  []string
		keyOnly  bool // an Origin key with no value at all (another layer can leave one)
		wantCode int
	}{
		{name: "POST with an Origin key but no value is a 403, like an empty one", allowed: listed, method: http.MethodPost,
			keyOnly: true, wantCode: http.StatusForbidden},
		{name: "an empty list lets everything through", method: http.MethodPost,
			origins: []string{"https://evil.example"}, wantCode: http.StatusTeapot},
		{name: "GET from an unlisted origin passes", allowed: listed, method: http.MethodGet,
			origins: []string{"https://evil.example"}, wantCode: http.StatusTeapot},
		{name: "HEAD from an unlisted origin passes", allowed: listed, method: http.MethodHead,
			origins: []string{"https://evil.example"}, wantCode: http.StatusTeapot},
		{name: "OPTIONS from an unlisted origin passes", allowed: listed, method: http.MethodOptions,
			origins: []string{"https://evil.example"}, wantCode: http.StatusTeapot},
		{name: "POST without Origin passes", allowed: listed, method: http.MethodPost, wantCode: http.StatusTeapot},
		{name: "POST from a listed origin passes", allowed: listed, method: http.MethodPost,
			origins: []string{"https://app.example.com"}, wantCode: http.StatusTeapot},
		{name: "POST from an unlisted origin is a 403 pointing at Origin", allowed: listed, method: http.MethodPost,
			origins: []string{"https://evil.example"}, wantCode: http.StatusForbidden},
		{name: "POST from a listed hybrid app origin (capacitor://localhost) passes",
			allowed: []string{"https://app.example.com", "capacitor://localhost"}, method: http.MethodPost,
			origins: []string{"capacitor://localhost"}, wantCode: http.StatusTeapot},
		{name: "POST from a hybrid app origin that isn't listed is a 403", allowed: listed, method: http.MethodPost,
			origins: []string{"ionic://localhost"}, wantCode: http.StatusForbidden},
		{name: "Origin null is refused", allowed: listed, method: http.MethodPost,
			origins: []string{"null"}, wantCode: http.StatusForbidden},
		{name: "an empty Origin is refused", allowed: listed, method: http.MethodPost,
			origins: []string{""}, wantCode: http.StatusForbidden},
		{name: "two Origin headers are refused even when the first is listed", allowed: listed, method: http.MethodPost,
			origins: []string{"https://app.example.com", "https://evil.example"}, wantCode: http.StatusForbidden},
		{name: "PATCH is guarded like POST", allowed: listed, method: http.MethodPatch,
			origins: []string{"https://evil.example"}, wantCode: http.StatusForbidden},
		{name: "DELETE is guarded like POST", allowed: listed, method: http.MethodDelete,
			origins: []string{"https://evil.example"}, wantCode: http.StatusForbidden},
		{name: "PUT is guarded like POST", allowed: listed, method: http.MethodPut,
			origins: []string{"https://evil.example"}, wantCode: http.StatusForbidden},
		{name: "case and trailing slash don't matter", allowed: []string{"https://APP.example.com/"}, method: http.MethodDelete,
			origins: []string{"https://app.EXAMPLE.com/"}, wantCode: http.StatusTeapot},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			reached := false
			r := gin.New()
			r.Use(OriginGuard(tc.allowed))
			r.Handle(tc.method, "/x", func(c *gin.Context) { reached = true; c.Status(http.StatusTeapot) })

			req := httptest.NewRequest(tc.method, "/x", nil)
			for _, o := range tc.origins {
				req.Header.Add("Origin", o)
			}
			if tc.keyOnly {
				req.Header["Origin"] = []string{}
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			require.Equal(t, tc.wantCode, w.Code, w.Body.String())
			require.Equal(t, tc.wantCode == http.StatusTeapot, reached, "the handler runs only when the guard lets it")
			if tc.wantCode == http.StatusForbidden {
				require.JSONEq(t, forbidden, w.Body.String())
			}
		})
	}
}

// TestOriginRefusal_Log_U: a refusal for two Origin headers says so in the
// log; quoting only the first (which may be listed) reads as a config bug.
func TestOriginRefusal_Log_U(t *testing.T) {
	listed := []string{"https://app.example.com"}
	testCases := []struct {
		name      string
		mw        gin.HandlerFunc
		method    string
		preflight bool
	}{
		{name: "the guard logs two Origin headers as such", mw: OriginGuard(listed), method: http.MethodPost},
		{name: "CORS logs two Origin headers on a preflight as such", mw: CORS(listed), method: http.MethodOptions, preflight: true},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			l, err := logging.New(&buf, logging.FormatJSON, "debug")
			require.NoError(t, err)
			prev := slog.Default()
			slog.SetDefault(l)
			defer slog.SetDefault(prev)

			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.Use(tc.mw)
			r.Handle(tc.method, "/x", func(c *gin.Context) { c.Status(http.StatusTeapot) })
			req := httptest.NewRequest(tc.method, "/x", nil)
			req.Header.Add("Origin", "https://app.example.com")
			req.Header.Add("Origin", "https://evil.example")
			if tc.preflight {
				req.Header.Set("Access-Control-Request-Method", http.MethodPost)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			require.Equal(t, http.StatusForbidden, w.Code)
			require.Contains(t, buf.String(), "2 Origin headers")
			require.NotContains(t, buf.String(), `\"https://app.example.com\" not`, "not logged as if the listed origin were refused")
		})
	}
}
