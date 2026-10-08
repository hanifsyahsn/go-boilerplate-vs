// Package logout ends a login: it revokes the refresh-token family server
// side and clears the auth cookies. The access token is a stateless JWT and
// stays valid until it expires (keep ACCESS_TOKEN_DURATION short).
package logout

import (
	"net/http"

	"github.com/gin-gonic/gin"
	authshared "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/handlerutil"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/jsonapi"
)

type H interface {
	Logout(c *gin.Context)
}

type handler struct {
	service S
	cookies authshared.CookieConfig
}

func NewHandler(service S, cookies authshared.CookieConfig) H {
	return &handler{service: service, cookies: cookies}
}

// Logout takes the refresh token from the cookie (browser) or an optional
// "sessions" document (other clients), and answers 204. The cookies are
// cleared whatever the outcome.
func (h *handler) Logout(c *gin.Context) {
	if _, ok := handlerutil.ParseQuery(c, authshared.SessionsRT, jsonapi.QuerySpec{}); !ok {
		return
	}

	token, ok := authshared.RefreshTokenFromRequest(c)
	if !ok {
		return
	}

	err := h.service.Logout(c, token)
	authshared.ClearCookies(c, h.cookies)
	if handlerutil.WriteError(c, err, "Logout Handler") {
		return
	}

	c.Status(http.StatusNoContent)
}
