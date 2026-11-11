package constant

// Gin context keys set by middleware.AccessTokenMiddleware, and the auth
// cookie names.
const (
	UserIDKey   = "user_id"
	UserRoleKey = "user_role"
	// SessionIDKey holds the caller's session (a uuid.UUID, the access
	// token's sid).
	SessionIDKey          = "session_id"
	AccessTokenCookieKey  = "app-access-token"  // #nosec G101 -- a cookie name, not a credential
	RefreshTokenCookieKey = "app-refresh-token" // #nosec G101 -- a cookie name, not a credential
)
