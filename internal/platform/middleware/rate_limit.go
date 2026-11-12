package middleware

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/ratelimit"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/handlerutil"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/jsonapi"
)

// TooManyRequestsMessage is the 429 detail of the per-IP limit.
const TooManyRequestsMessage = "Too many attempts. Please wait a few minutes and try again."

// RateLimitByIP answers 429, with Retry-After, once the caller's IP has no
// token left on the routes it guards (the public auth endpoints). name
// separates the buckets of differently limited routes sharing one limiter.
//
// The IP is gin's c.ClientIP(), which only reads X-Forwarded-For /
// X-Real-IP from a trusted proxy (TRUSTED_PROXIES, see server.Security);
// otherwise it's the connection's address, so a client can't pick its own.
// An IPv6 client is limited per /64 (ratelimit.IPKey).
//
// A limiter error is a 500: the limiter decides how to survive its store
// going down (decision 0003), so an error reaching here means it couldn't.
func RateLimitByIP(l ratelimit.Limiter, name string) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := handlerutil.RequestContext(c)
		ip := c.ClientIP()
		d, err := l.Take(ctx, name+":"+ratelimit.IPKey(ip))
		if err != nil {
			jsonapi.WriteErrorsContext(ctx, c.Writer, apperror.New(apperror.CodeInternal,
				apperror.CodeInternal.SafeToThrow(), fmt.Errorf("rate limit %s: Take() (Rate Limit Middleware): %w", name, err)))
			c.Abort()
			return
		}
		if !d.Allowed {
			jsonapi.WriteErrorsContext(ctx, c.Writer,
				apperror.New(apperror.CodeTooManyRequests, TooManyRequestsMessage,
					fmt.Errorf("rate limit %s exceeded by %s (Rate Limit Middleware)", name, ip)).WithRetryAfter(d.RetryAfter))
			c.Abort()
			return
		}
		c.Next()
	}
}
