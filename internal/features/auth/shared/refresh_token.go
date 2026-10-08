package shared

import (
	"github.com/gin-gonic/gin"
	internalsharedconstant "github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/constant"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/handlerutil"
)

// RefreshTokenRequest is the attributes of the optional "sessions" document
// of POST /api/auth/refresh and /api/auth/logout. A browser client sends no
// body (the refresh cookie is used); another client sends the token here.
type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" binding:"omitempty,max=4096" label:"Refresh token"`
}

// RefreshTokenFromRequest returns the caller's refresh token: the cookie when
// present, else the document's refresh_token ("" when neither). ok is false
// when the body isn't a valid document; the error is already written.
func RefreshTokenFromRequest(c *gin.Context) (token string, ok bool) {
	var req RefreshTokenRequest
	if !handlerutil.BindResource(c, handlerutil.ResourceSpec{Type: SessionsRT, Optional: true}, &req) {
		return "", false
	}

	if cookie, err := c.Cookie(internalsharedconstant.RefreshTokenCookieKey); err == nil && cookie != "" {
		return cookie, true
	}
	return req.RefreshToken, true
}
