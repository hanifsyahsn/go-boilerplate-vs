package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/ratelimit"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/jsonapi"
	"github.com/stretchr/testify/require"
)

func TestRateLimitByIP_U(t *testing.T) {
	const tooMany = `{"jsonapi":{"version":"1.1"},"errors":[{"status":"429","code":"too_many_requests",
		"title":"Too Many Requests","detail":"Too many attempts. Please wait a few minutes and try again."}]}`

	testCases := []struct {
		name string
		// ips are the callers' addresses, one request each, against a
		// 2-per-minute limiter.
		ips      []string
		wantLast int
	}{
		{name: "requests within the limit reach the handler", ips: []string{"10.0.0.1", "10.0.0.1"}, wantLast: http.StatusTeapot},
		{name: "the request past the limit is a 429 and never reaches the handler",
			ips: []string{"10.0.0.1", "10.0.0.1", "10.0.0.1"}, wantLast: http.StatusTooManyRequests},
		{name: "each IP has its own window",
			ips: []string{"10.0.0.1", "10.0.0.1", "10.0.0.2"}, wantLast: http.StatusTeapot},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			reached := 0
			r := gin.New()
			r.Use(RateLimitByIP(ratelimit.New(2, time.Minute), "auth"))
			r.GET("/", func(c *gin.Context) { reached++; c.Status(http.StatusTeapot) })

			var w *httptest.ResponseRecorder
			for _, ip := range tc.ips {
				req := httptest.NewRequest(http.MethodGet, "/", nil)
				req.RemoteAddr = ip + ":1234"
				w = httptest.NewRecorder()
				r.ServeHTTP(w, req)
			}

			require.Equal(t, tc.wantLast, w.Code)
			if tc.wantLast == http.StatusTooManyRequests {
				require.Equal(t, jsonapi.MediaType, w.Header().Get("Content-Type"))
				require.JSONEq(t, tooMany, w.Body.String())
				require.Equal(t, len(tc.ips)-1, reached, "the refused request never reaches the handler")
			} else {
				require.Equal(t, len(tc.ips), reached)
			}
		})
	}
}
