package shared

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/jwttoken"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/jsonapi"
)

// UsersRT is the JSON:API resource type of a user.
const UsersRT = "users"

// SessionsRT is the resource type of the login / refresh / logout request
// documents: a session is what logging in opens and logging out closes.
const SessionsRT = "sessions"

// UserQuery is the query support of the endpoints answering with the
// account (register, login): sparse fieldsets on it.
var UserQuery = jsonapi.QuerySpec{Fields: map[string]any{UsersRT: UserResource{}}}

// UserResource is the account login / register answer with.
type UserResource struct {
	ID    string `json:"id" jsonapi:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

// NewUserResource builds the response resource of one account.
func NewUserResource(id int64, name, email, role string) UserResource {
	return UserResource{ID: strconv.FormatInt(id, 10), Name: name, Email: email, Role: role}
}

// TokenMeta is written under the response "meta" of login / refresh, for
// clients that don't use the cookies (mobile, server-to-server). A refresh
// rotates the refresh token, so both carry a new one: the client must store
// it, the old one is spent.
type TokenMeta struct {
	AccessToken           string    `json:"access_token"`
	AccessTokenExpiresAt  time.Time `json:"access_token_expires_at"`
	RefreshToken          string    `json:"refresh_token"`
	RefreshTokenExpiresAt time.Time `json:"refresh_token_expires_at"`
}

// NewTokenMeta builds the meta of a freshly issued pair.
func NewTokenMeta(p jwttoken.Pair) TokenMeta {
	return TokenMeta{
		AccessToken:           p.AccessToken,
		AccessTokenExpiresAt:  p.AccessTokenExpiresAt,
		RefreshToken:          p.RefreshToken,
		RefreshTokenExpiresAt: p.RefreshTokenExpiresAt,
	}
}

// SetTokenCookies stores a freshly issued pair in the two auth cookies.
func SetTokenCookies(c *gin.Context, cfg CookieConfig, p jwttoken.Pair) {
	SetAccessCookie(c, cfg, p.AccessToken, p.AccessTokenExpiresAt)
	SetRefreshCookie(c, cfg, p.RefreshToken, p.RefreshTokenExpiresAt)
}
