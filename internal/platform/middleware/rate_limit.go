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

// RateLimitByIP answers 429 once the caller's IP used up its window on the
// routes it guards (the public auth endpoints). name separates the counters
// of differently limited routes sharing one limiter.
//
// The IP is gin's c.ClientIP(), which only reads X-Forwarded-For /
// X-Real-IP from a trusted proxy (TRUSTED_PROXIES, see server.Security);
// otherwise it's the connection's address, so a client can't pick its own.
func RateLimitByIP(l *ratelimit.Limiter, name string) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		if !l.Allow(name + ":" + ip) {
			jsonapi.WriteErrorsContext(handlerutil.RequestContext(c), c.Writer,
				apperror.New(apperror.CodeTooManyRequests, TooManyRequestsMessage,
					fmt.Errorf("rate limit %s exceeded by %s (Rate Limit Middleware)", name, ip)))
			c.Abort()
			return
		}
		c.Next()
	}
}
