package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
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
		wantCode int
	}{
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
