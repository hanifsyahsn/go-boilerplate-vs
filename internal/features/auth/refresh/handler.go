package refresh

import (
	"net/http"

	"github.com/gin-gonic/gin"
	authshared "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/handlerutil"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/jsonapi"
)

type H interface {
	Refresh(c *gin.Context)
}

type handler struct {
	service S
	cookies authshared.CookieConfig
}

func NewHandler(service S, cookies authshared.CookieConfig) H {
	return &handler{service: service, cookies: cookies}
}

// Refresh reads the refresh token from the cookie (browser) or an optional
// "sessions" document (other clients), and answers with a new pair: both
// cookies are replaced and both tokens are returned in a meta-only document,
// since the old refresh token is spent.
func (h *handler) Refresh(c *gin.Context) {
	if _, ok := handlerutil.ParseQuery(c, authshared.SessionsRT, jsonapi.QuerySpec{}); !ok {
		return
	}

	token, ok := authshared.RefreshTokenFromRequest(c)
	if !ok {
		return
	}

	res, err := h.service.Refresh(c, token)
	if handlerutil.WriteError(c, err, "Refresh Handler") {
		return
	}

	authshared.SetTokenCookies(c, h.cookies, res.Tokens)
	handlerutil.WriteMeta(c, http.StatusOK, authshared.NewTokenMeta(res.Tokens))
}
