package jwttoken

import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	platformconstant "github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/constant"
	internalshareddto "github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/dto"
)

type MakerES256 struct {
	privateKey *ecdsa.PrivateKey
	publicKey  *ecdsa.PublicKey
	clientID   string
	issuer     string
	now        func() time.Time
}

// NewTokenMakerES256 builds the token maker. clientID becomes the `aud`
// claim and issuer the `iss` claim; both are checked on verify.
func NewTokenMakerES256(privateKey *ecdsa.PrivateKey, publicKey *ecdsa.PublicKey, clientID, issuer string) M {
	return &MakerES256{
		privateKey: privateKey,
		publicKey:  publicKey,
		clientID:   clientID,
		issuer:     issuer,
		now:        time.Now,
	}
}

func (maker *MakerES256) CreateToken(user internalshareddto.User, accessTokenDuration, refreshTokenDuration time.Duration) (Pair, error) {
	accessToken, accessExp, err := maker.GenerateAccessToken(user, accessTokenDuration)
	if err != nil {
		return Pair{}, err
	}

	now := maker.now()
	refreshExp := now.Add(refreshTokenDuration)
	refreshID := uuid.New()
	refreshClaims := jwt.MapClaims{
		platformconstant.SubKey:            user.ID,
		platformconstant.TokenTypeKey:      platformconstant.TokenTypeRefresh,
		platformconstant.IssuerKey:         maker.issuer,
		platformconstant.AudienceKey:       maker.clientID,
		platformconstant.ExpirationKey:     refreshExp.Unix(),
		platformconstant.IssuedAtKey:       now.Unix(),
		platformconstant.JsonWebTokenIDKey: refreshID.String(),
	}
	refreshToken, err := jwt.NewWithClaims(jwt.SigningMethodES256, refreshClaims).SignedString(maker.privateKey)
	if err != nil {
		// only fails with a nil / invalid private key
		return Pair{}, fmt.Errorf("sign refresh token: %w", err)
	}

	return Pair{
		AccessToken:           accessToken,
		AccessTokenExpiresAt:  accessExp,
		RefreshToken:          refreshToken,
		RefreshTokenID:        refreshID,
		RefreshTokenExpiresAt: refreshExp,
	}, nil
}

func (maker *MakerES256) GenerateAccessToken(user internalshareddto.User, accessTokenDuration time.Duration) (string, time.Time, error) {
	now := maker.now()
	exp := now.Add(accessTokenDuration)
	claims := jwt.MapClaims{
		platformconstant.SubKey:            user.ID,
		platformconstant.TokenTypeKey:      platformconstant.TokenTypeAccess,
		platformconstant.EmailKey:          user.Email,
		platformconstant.UserNameKey:       user.Name,
		platformconstant.RoleKey:           user.Role.String(),
		platformconstant.IssuerKey:         maker.issuer,
		platformconstant.AudienceKey:       maker.clientID,
		platformconstant.ExpirationKey:     exp.Unix(),
		platformconstant.IssuedAtKey:       now.Unix(),
		platformconstant.JsonWebTokenIDKey: uuid.New().String(),
	}

	token, err := jwt.NewWithClaims(jwt.SigningMethodES256, claims).SignedString(maker.privateKey)
	if err != nil {
		// only fails with a nil / invalid private key
		return "", time.Time{}, fmt.Errorf("sign access token: %w", err)
	}
	return token, exp, nil
}

func (maker *MakerES256) VerifyAccessToken(tokenString string) (jwt.MapClaims, error) {
	return maker.verify(tokenString, platformconstant.TokenTypeAccess)
}

func (maker *MakerES256) VerifyRefreshToken(tokenString string) (jwt.MapClaims, error) {
	return maker.verify(tokenString, platformconstant.TokenTypeRefresh)
}

// verify checks the signature (ES256 only), exp / iat, iss, aud, and that the
// token is of the wanted type and carries a sub and a jti.
func (maker *MakerES256) verify(tokenString, wantType string) (jwt.MapClaims, error) {
	token, err := jwt.Parse(
		tokenString,
		func(token *jwt.Token) (any, error) { return maker.publicKey, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodES256.Alg()}),
		jwt.WithIssuer(maker.issuer),
		jwt.WithAudience(maker.clientID),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithTimeFunc(maker.now),
	)
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		// defensive: jwt.Parse returns valid MapClaims on success
		return nil, jwt.ErrSignatureInvalid
	}

	if typ, _ := claims[platformconstant.TokenTypeKey].(string); typ != wantType {
		return nil, fmt.Errorf("token type %q, want %q", typ, wantType)
	}
	if jti, _ := claims[platformconstant.JsonWebTokenIDKey].(string); jti == "" {
		return nil, errors.New("token has no jti claim")
	}
	if _, err = SubjectID(claims); err != nil {
		return nil, err
	}

	return claims, nil
}

// SubjectID reads the `sub` claim as a user id. JSON numbers decode as
// float64, so that's what a parsed token holds.
func SubjectID(claims jwt.MapClaims) (int64, error) {
	sub, ok := claims[platformconstant.SubKey].(float64)
	if !ok || sub <= 0 {
		return 0, errors.New("token has no valid sub claim")
	}
	return int64(sub), nil
}

// TokenID reads the `jti` claim as a UUID (every token this maker issues has
// one). The refresh token's jti is its row in refresh_tokens.
func TokenID(claims jwt.MapClaims) (uuid.UUID, error) {
	jti, _ := claims[platformconstant.JsonWebTokenIDKey].(string)
	id, err := uuid.Parse(jti)
	if err != nil {
		return uuid.Nil, fmt.Errorf("token has no valid jti claim: %w", err)
	}
	return id, nil
}

func LoadECPrivateKey(path string) (*ecdsa.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("invalid private key PEM")
	}
	return x509.ParseECPrivateKey(block.Bytes)
}

func LoadECPublicKey(path string) (*ecdsa.PublicKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("invalid public key PEM")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	key, ok := pub.(*ecdsa.PublicKey)
	if !ok {
		return nil, errors.New("public key is not an ECDSA key")
	}
	return key, nil
}
