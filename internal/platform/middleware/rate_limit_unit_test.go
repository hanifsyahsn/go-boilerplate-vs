package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/ratelimit"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/jsonapi"
	"github.com/stretchr/testify/require"
)

// brokenLimiter is a Limiter whose store is down.
type brokenLimiter struct{}

func (brokenLimiter) Take(context.Context, string) (ratelimit.Decision, error) {
	return ratelimit.Decision{}, errors.New("store down")
}

func (brokenLimiter) Reset(context.Context, string) error { return errors.New("store down") }

func (brokenLimiter) Return(context.Context, string) error { return errors.New("store down") }

func TestRateLimitByIP_U(t *testing.T) {
	const tooMany = `{"jsonapi":{"version":"1.1"},"errors":[{"status":"429","code":"too_many_requests",
		"title":"Too Many Requests","detail":"Too many attempts. Please wait a few minutes and try again."}]}`
	const internal = `{"jsonapi":{"version":"1.1"},"errors":[{"status":"500","code":"internal_error",
		"title":"Internal Server Error","detail":"Something went wrong, please try again."}]}`

	testCases := []struct {
		name string
		// limiter is nil for a 2-per-minute Memory (a token every 30s).
		limiter ratelimit.Limiter
		// ips are the callers' addresses, one request each.
		ips            []string
		wantLast       int
		wantRetryAfter string
		wantBody       string
	}{
		{name: "requests within the limit reach the handler", ips: []string{"10.0.0.1", "10.0.0.1"}, wantLast: http.StatusTeapot},
		{name: "the request past the limit is a 429 with Retry-After and never reaches the handler",
			ips: []string{"10.0.0.1", "10.0.0.1", "10.0.0.1"}, wantLast: http.StatusTooManyRequests,
			wantRetryAfter: "30", wantBody: tooMany},
		{name: "each IP has its own bucket",
			ips: []string{"10.0.0.1", "10.0.0.1", "10.0.0.2"}, wantLast: http.StatusTeapot},
		{name: "IPv6 addresses in one /64 share a bucket",
			ips: []string{"[2001:db8:1:2::1]", "[2001:db8:1:2::2]", "[2001:db8:1:2::3]"}, wantLast: http.StatusTooManyRequests,
			wantRetryAfter: "30", wantBody: tooMany},
		{name: "another IPv6 /64 has its own bucket",
			ips: []string{"[2001:db8:1:2::1]", "[2001:db8:1:2::2]", "[2001:db8:1:3::1]"}, wantLast: http.StatusTeapot},
		{name: "a limiter that fails is a 500 and the handler never runs",
			limiter: brokenLimiter{}, ips: []string{"10.0.0.1"}, wantLast: http.StatusInternalServerError, wantBody: internal},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			l := tc.limiter
			if l == nil {
				l = ratelimit.NewMemory(ratelimit.Rate{Burst: 2, Per: time.Minute})
			}
			reached := 0
			r := gin.New()
			r.Use(RateLimitByIP(l, "auth"))
			r.GET("/", func(c *gin.Context) { reached++; c.Status(http.StatusTeapot) })

			var w *httptest.ResponseRecorder
			for _, ip := range tc.ips {
				req := httptest.NewRequest(http.MethodGet, "/", nil)
				req.RemoteAddr = ip + ":1234"
				w = httptest.NewRecorder()
				r.ServeHTTP(w, req)
			}

			require.Equal(t, tc.wantLast, w.Code)
			require.Equal(t, tc.wantRetryAfter, w.Header().Get("Retry-After"))
			if tc.wantBody == "" {
				require.Equal(t, len(tc.ips), reached)
				return
			}
			require.Equal(t, jsonapi.MediaType, w.Header().Get("Content-Type"))
			require.JSONEq(t, tc.wantBody, w.Body.String())
			require.Equal(t, len(tc.ips)-1, reached, "the refused request never reaches the handler")
		})
	}
}

func TestRetryAfterSeconds_U(t *testing.T) {
	testCases := []struct {
		name string
		wait time.Duration
		want string
	}{
		{name: "whole seconds stay as they are", wait: 30 * time.Second, want: "30"},
		{name: "a part of a second rounds up", wait: 2500 * time.Millisecond, want: "3"},
		{name: "never less than one second", wait: 0, want: "1"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, RetryAfterSeconds(tc.wait))
		})
	}
}
