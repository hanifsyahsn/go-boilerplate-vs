package middleware

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/handlerutil"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/jsonapi"
)

// OriginNotAllowedForWriteMessage is the 403 detail of a state-changing
// request from an origin that isn't in ALLOWED_ORIGINS.
const OriginNotAllowedForWriteMessage = "This request didn't come from an allowed origin."

// OriginGuard refuses a state-changing request whose Origin header isn't
// listed. Empty list = off.
//
// It closes the CSRF paths SameSite=Lax leaves open (a same-site sibling, an
// old browser): GET / HEAD / OPTIONS pass; any other method passes with no
// Origin header (curl, a server, an old client — no page made them send a
// victim's cookies) or with exactly one listed one (compared like CORS:
// lowercase, no trailing /), and is a 403 on source.header Origin otherwise,
// the literal "null" (a sandboxed frame, a file:// page) included.
func OriginGuard(allowedOrigins []string) gin.HandlerFunc {
	allowed := newOriginSet(allowedOrigins)
	return func(c *gin.Context) {
		if len(allowed) == 0 {
			c.Next()
			return
		}
		switch c.Request.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			c.Next()
			return
		}
		value, count := singleHeader(c.Request.Header, "Origin")
		if count == 0 {
			c.Next()
			return
		}
		if _, ok := allowed[normalizeOrigin(value)]; ok && count == 1 {
			c.Next()
			return
		}
		jsonapi.WriteErrorsContext(handlerutil.RequestContext(c), c.Writer,
			apperror.New(apperror.CodeForbidden, OriginNotAllowedForWriteMessage,
				fmt.Errorf("%s not allowed for %s %s", describeOrigin(value, count), c.Request.Method, c.Request.URL.Path)).WithHeader("Origin"))
		c.Abort()
	}
}
