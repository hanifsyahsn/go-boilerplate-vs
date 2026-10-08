package jwttoken

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	internalshareddto "github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/dto"
)

// M issues and verifies the app's ES256 JWTs. An access token carries the
// user's id / name / email / role; a refresh token only the user id. Each
// carries a `typ` claim, and each Verify* rejects the other type.
type M interface {
	// CreateToken issues an access + refresh token pair (login, and every
	// refresh: refresh tokens rotate).
	CreateToken(user internalshareddto.User, accessTokenDuration, refreshTokenDuration time.Duration) (Pair, error)
	// GenerateAccessToken issues an access token only.
	GenerateAccessToken(user internalshareddto.User, accessTokenDuration time.Duration) (token string, expiresAt time.Time, err error)
	VerifyAccessToken(tokenString string) (jwt.MapClaims, error)
	VerifyRefreshToken(tokenString string) (jwt.MapClaims, error)
}

// Pair is a freshly issued access + refresh token.
type Pair struct {
	AccessToken          string
	AccessTokenExpiresAt time.Time
	RefreshToken         string
	// RefreshTokenID is the refresh token's jti, the key of its
	// refresh_tokens row.
	RefreshTokenID        uuid.UUID
	RefreshTokenExpiresAt time.Time
}
