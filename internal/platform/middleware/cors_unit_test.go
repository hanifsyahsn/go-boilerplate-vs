package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCORS_U(t *testing.T) {
	const forbidden = `{"jsonapi":{"version":"1.1"},"errors":[{"status":"403","code":"forbidden","title":"Forbidden",
		"detail":"This origin isn't allowed.","source":{"header":"Origin"}}]}`
	listed := []string{"https://app.example.com", "http://localhost:5173"}
	corsHeaders := []string{"Access-Control-Allow-Origin", "Access-Control-Allow-Credentials",
		"Access-Control-Expose-Headers", "Access-Control-Allow-Methods", "Access-Control-Allow-Headers",
		"Access-Control-Max-Age"}

	testCases := []struct {
		name      string
		allowed   []string
		method    string
		origin    string
		preflight bool // sets Access-Control-Request-Method
		// withMediaType mounts JSONAPIMediaType after CORS, as on /api.
		withMediaType bool
		wantCode      int
		wantHandler   bool
		wantHeaders   map[string]string // "" = must be absent
		wantVary      []string
		wantBody      string
	}{
		{name: "an empty list adds no CORS header", allowed: nil, method: http.MethodGet, origin: "https://app.example.com",
			wantCode: http.StatusTeapot, wantHandler: true, wantHeaders: map[string]string{"Access-Control-Allow-Origin": ""}},
		{name: "an empty list leaves a preflight to the router", allowed: nil, method: http.MethodOptions,
			origin: "https://evil.example", preflight: true, wantCode: http.StatusTeapot, wantHandler: true},
		{name: "a request without Origin gets only Vary", allowed: listed, method: http.MethodGet,
			wantCode: http.StatusTeapot, wantHandler: true, wantVary: []string{"Origin"}},
		{name: "a listed origin is echoed with credentials", allowed: listed, method: http.MethodPost, origin: "https://app.example.com",
			wantCode: http.StatusTeapot, wantHandler: true, wantVary: []string{"Origin"}, wantHeaders: map[string]string{
				"Access-Control-Allow-Origin":      "https://app.example.com",
				"Access-Control-Allow-Credentials": "true",
				"Access-Control-Expose-Headers":    "Location, Retry-After, X-Request-ID",
				"Access-Control-Allow-Methods":     "",
			}},
		{name: "a listed origin matches ignoring case and a trailing slash", allowed: []string{"https://APP.example.com/"},
			method: http.MethodGet, origin: "https://app.EXAMPLE.com", wantCode: http.StatusTeapot, wantHandler: true,
			wantHeaders: map[string]string{"Access-Control-Allow-Origin": "https://app.EXAMPLE.com"}},
		{name: "a listed origin's preflight is a 204 with methods, headers and max age", allowed: listed,
			method: http.MethodOptions, origin: "http://localhost:5173", preflight: true, wantCode: http.StatusNoContent,
			wantVary: []string{"Origin"}, wantHeaders: map[string]string{
				"Access-Control-Allow-Origin":      "http://localhost:5173",
				"Access-Control-Allow-Credentials": "true",
				"Access-Control-Allow-Methods":     "GET, POST, PATCH, DELETE",
				"Access-Control-Allow-Headers":     "Accept, Authorization, Content-Type, X-Request-ID",
				"Access-Control-Max-Age":           "600",
			}},
		{name: "an OPTIONS without Access-Control-Request-Method is not a preflight", allowed: listed,
			method: http.MethodOptions, origin: "http://localhost:5173", wantCode: http.StatusTeapot, wantHandler: true,
			wantHeaders: map[string]string{"Access-Control-Allow-Origin": "http://localhost:5173", "Access-Control-Max-Age": ""}},
		{name: "an unlisted origin gets no CORS header", allowed: listed, method: http.MethodGet, origin: "https://evil.example",
			wantCode: http.StatusTeapot, wantHandler: true, wantVary: []string{"Origin"}},
		{name: "an unlisted origin's preflight is a 403 pointing at the Origin header", allowed: listed,
			method: http.MethodOptions, origin: "https://evil.example", preflight: true, wantCode: http.StatusForbidden,
			wantVary: []string{"Origin"}, wantBody: forbidden},
		{name: "Vary keeps Accept", allowed: listed, method: http.MethodGet, origin: "https://app.example.com",
			withMediaType: true, wantCode: http.StatusTeapot, wantHandler: true, wantVary: []string{"Origin", "Accept"},
			wantHeaders: map[string]string{"Access-Control-Allow-Origin": "https://app.example.com"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			reached := false
			r := gin.New()
			r.Use(CORS(tc.allowed))
			if tc.withMediaType {
				r.Use(JSONAPIMediaType())
			}
			r.Handle(tc.method, "/", func(c *gin.Context) { reached = true; c.Status(http.StatusTeapot) })

			req := httptest.NewRequest(tc.method, "/", nil)
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			if tc.preflight {
				req.Header.Set("Access-Control-Request-Method", http.MethodPost)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			require.Equal(t, tc.wantCode, w.Code, w.Body.String())
			require.Equal(t, tc.wantHandler, reached)
			wantVary := tc.wantVary
			if wantVary == nil && len(tc.allowed) > 0 {
				wantVary = []string{"Origin"} // every response once the list is set
			}
			require.Equal(t, wantVary, w.Header().Values("Vary"))
			if tc.wantHeaders == nil {
				for _, h := range corsHeaders {
					require.Empty(t, w.Header().Get(h), h)
				}
			}
			for h, want := range tc.wantHeaders {
				require.Equal(t, want, w.Header().Get(h), h)
			}
			if tc.wantBody != "" {
				require.JSONEq(t, tc.wantBody, w.Body.String())
			}
		})
	}
}
