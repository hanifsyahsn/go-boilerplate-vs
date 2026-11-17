package refresh

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	authshared "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/middleware"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
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
// since the old refresh token is spent. A refresh that can never succeed
// again clears both cookies (see endsSession).
func (h *handler) Refresh(c *gin.Context) {
	if _, ok := handlerutil.ParseQuery(c, authshared.SessionsRT, jsonapi.QuerySpec{}); !ok {
		return
	}

	token, ok := authshared.RefreshTokenFromRequest(c)
	if !ok {
		return
	}

	res, err := h.service.Refresh(c, token, authshared.SessionClient{UserAgent: c.Request.UserAgent(), IP: c.ClientIP()})
	if endsSession(err) {
		authshared.ClearCookies(c, h.cookies)
	}
	if handlerutil.WriteError(c, err, "Refresh Handler") {
		return
	}

	authshared.SetTokenCookies(c, h.cookies, res.Tokens)
	handlerutil.WriteMeta(c, http.StatusOK, authshared.NewTokenMeta(res.Tokens))
}

// endsSession reports whether a refresh error means this session can't be
// refreshed any more (decision 0020): session_expired, session_ended,
// account_inactive. The handler then clears both cookies, so a stale access
// cookie (it lives as long as the refresh token) doesn't make every later page
// load look signed in. refresh_superseded keeps them — the request that won
// the race already set the new ones — and so does a 500 (nothing was decided).
func endsSession(err error) bool {
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) {
		return false
	}
	switch appErr.Code {
	case CodeSessionExpired, middleware.CodeSessionEnded, middleware.CodeAccountInactive:
		return true
	}
	return false
}
