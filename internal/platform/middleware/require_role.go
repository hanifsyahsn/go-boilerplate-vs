package middleware

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	internalsharedconstant "github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/constant"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/enum"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/jsonapi"
)

// RequireRole gates a route group on the caller's role, read from the access
// token (set into the gin context by AccessTokenMiddleware, which must run
// first). The caller's role must be one of allowed; otherwise the request is
// aborted with 403. An empty / missing role never matches.
func RequireRole(allowed ...enum.Role) gin.HandlerFunc {
	set := make(map[enum.Role]struct{}, len(allowed))
	for _, r := range allowed {
		set[r] = struct{}{}
	}

	return func(c *gin.Context) {
		role := enum.Role(c.GetString(internalsharedconstant.UserRoleKey))
		if _, ok := set[role]; role == "" || !ok {
			jsonapi.WriteError(
				c.Writer,
				apperror.CodeForbidden,
				"You don't have access to this.",
				fmt.Errorf("RequireRole: caller role %q not in %v", role, allowed),
			)
			c.Abort()
			return
		}

		c.Next()
	}
}
