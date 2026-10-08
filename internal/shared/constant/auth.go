package constant

// Gin context keys set by middleware.AccessTokenMiddleware, and the auth
// cookie names.
const (
	UserIdKey             = "user_id"
	UserRoleKey           = "user_role"
	AccessTokenCookieKey  = "app-access-token"
	RefreshTokenCookieKey = "app-refresh-token"
)
