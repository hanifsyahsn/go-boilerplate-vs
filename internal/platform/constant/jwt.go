package constant

// JWT claim keys.
const (
	UserNameKey       = "name"
	EmailKey          = "email"
	RoleKey           = "role"
	SubKey            = "sub"
	ExpirationKey     = "exp"
	IssuedAtKey       = "iat"
	IssuerKey         = "iss"
	JSONWebTokenIDKey = "jti"
	AudienceKey       = "aud"
	// TokenTypeKey tells an access token from a refresh token, so a refresh
	// token can't be replayed as an access token (and vice versa).
	TokenTypeKey = "typ"
	// SessionIDKey is the access token's session: its refresh-token family
	// (auth_sessions.family_id), so a signed-out session's access tokens stop
	// working at once.
	SessionIDKey = "sid"
)

// Token types, the values of TokenTypeKey.
const (
	TokenTypeAccess  = "access"
	TokenTypeRefresh = "refresh"
)
