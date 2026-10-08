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
	JsonWebTokenIDKey = "jti"
	AudienceKey       = "aud"
	// TokenTypeKey tells an access token from a refresh token, so a refresh
	// token can't be replayed as an access token (and vice versa).
	TokenTypeKey = "typ"
)

// Token types, the values of TokenTypeKey.
const (
	TokenTypeAccess  = "access"
	TokenTypeRefresh = "refresh"
)
