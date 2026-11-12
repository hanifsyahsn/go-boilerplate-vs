package middleware

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/handlerutil"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/jsonapi"
)

// The CORS answer for a listed origin.
const (
	corsExposeHeaders = "Location, Retry-After, X-Request-ID"
	corsAllowMethods  = "GET, POST, PATCH, DELETE"
	corsAllowHeaders  = "Accept, Authorization, Content-Type, X-Request-ID"
	corsMaxAge        = "600"
)

// OriginNotAllowedMessage is the 403 detail of a preflight from an origin
// that isn't in ALLOWED_ORIGINS.
const OriginNotAllowedMessage = "This origin isn't allowed."

// CORS answers browsers for the listed origins only. Empty list = off (no
// CORS headers at all).
//
// A listed Origin is echoed back with credentials allowed (the auth cookies),
// never "*"; a preflight (OPTIONS with Access-Control-Request-Method) from it
// is a 204. An unlisted origin gets no CORS header: its preflight is a 403,
// its ordinary request goes on (the browser won't let the page read the
// answer, and OriginGuard refuses its writes). Every response varies on
// Origin.
func CORS(allowedOrigins []string) gin.HandlerFunc {
	allowed := newOriginSet(allowedOrigins)
	return func(c *gin.Context) {
		if len(allowed) == 0 {
			c.Next()
			return
		}
		c.Writer.Header().Add("Vary", "Origin")

		origin, count := singleHeader(c.Request.Header, "Origin")
		if count == 0 {
			c.Next()
			return
		}
		preflight := c.Request.Method == http.MethodOptions && c.GetHeader("Access-Control-Request-Method") != ""

		// Exactly one listed origin, as OriginGuard requires: two Origin
		// headers (or an empty one) are no origin a browser sends.
		if _, ok := allowed[normalizeOrigin(origin)]; !ok || count != 1 {
			if preflight {
				jsonapi.WriteErrorsContext(handlerutil.RequestContext(c), c.Writer,
					apperror.New(apperror.CodeForbidden, OriginNotAllowedMessage,
						fmt.Errorf("%s not in ALLOWED_ORIGINS", describeOrigin(origin, count))).WithHeader("Origin"))
				c.Abort()
				return
			}
			c.Next()
			return
		}

		h := c.Writer.Header()
		h.Set("Access-Control-Allow-Origin", origin)
		h.Set("Access-Control-Allow-Credentials", "true")
		h.Set("Access-Control-Expose-Headers", corsExposeHeaders)
		if preflight {
			h.Set("Access-Control-Allow-Methods", corsAllowMethods)
			h.Set("Access-Control-Allow-Headers", corsAllowHeaders)
			h.Set("Access-Control-Max-Age", corsMaxAge)
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
