// Package shared holds what more than one auth slice needs: the auth cookies,
// reading the refresh token off a request, the response shapes of login /
// register / refresh, and the refresh_tokens queries (query/, sqlc/).
package shared

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	internalsharedconstant "github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/constant"
)

// refreshCookiePath limits the refresh-token cookie to the auth endpoints, so
// it isn't sent with every API call.
const refreshCookiePath = "/api/auth"

// CookieConfig is the environment-specific part of the auth cookies
// (COOKIE_SECURE / COOKIE_DOMAIN).
type CookieConfig struct {
	Secure bool
	Domain string
}

// SetAccessCookie stores the access token in an HttpOnly cookie that expires
// with the token.
func SetAccessCookie(c *gin.Context, cfg CookieConfig, token string, expiresAt time.Time) {
	setCookie(c, cfg, internalsharedconstant.AccessTokenCookieKey, token, "/", expiresAt)
}

// SetRefreshCookie stores the refresh token in an HttpOnly cookie scoped to
// /api/auth.
func SetRefreshCookie(c *gin.Context, cfg CookieConfig, token string, expiresAt time.Time) {
	setCookie(c, cfg, internalsharedconstant.RefreshTokenCookieKey, token, refreshCookiePath, expiresAt)
}

// ClearCookies expires both auth cookies (logout).
func ClearCookies(c *gin.Context, cfg CookieConfig) {
	expired := time.Unix(0, 0)
	setCookie(c, cfg, internalsharedconstant.AccessTokenCookieKey, "", "/", expired)
	setCookie(c, cfg, internalsharedconstant.RefreshTokenCookieKey, "", refreshCookiePath, expired)
}

func setCookie(c *gin.Context, cfg CookieConfig, name, value, path string, expiresAt time.Time) {
	maxAge := int(time.Until(expiresAt).Seconds())
	if maxAge <= 0 {
		maxAge = -1 // delete now
	}
	// Secure comes from COOKIE_SECURE, default true; only plain-HTTP local
	// development turns it off. HttpOnly and SameSite are always set.
	// nosemgrep: go.lang.security.audit.net.cookie-missing-secure.cookie-missing-secure
	http.SetCookie(c.Writer, &http.Cookie{ // #nosec G124 -- Secure is COOKIE_SECURE (default true)
		Name:     name,
		Value:    value,
		Path:     path,
		Domain:   cfg.Domain,
		Expires:  expiresAt,
		MaxAge:   maxAge,
		Secure:   cfg.Secure,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}
