package jwttoken

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	platformconstant "github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/constant"
	internalshareddto "github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/dto"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/enum"
	"github.com/stretchr/testify/require"
)

func newTestKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	return key
}

func newTestMaker(t *testing.T, key *ecdsa.PrivateKey, clientID, issuer string) *MakerES256 {
	t.Helper()
	return NewTokenMakerES256(key, &key.PublicKey, clientID, issuer).(*MakerES256)
}

var testUser = internalshareddto.User{ID: 42, Name: "Jane", Email: "jane@example.com", Role: enum.RoleAdmin}

func TestMakerES256_CreateToken_U(t *testing.T) {
	key := newTestKey(t)
	maker := newTestMaker(t, key, "client", "issuer")

	pair, err := maker.CreateToken(testUser, 15*time.Minute, 24*time.Hour)
	require.NoError(t, err)

	testCases := []struct {
		name      string
		verify    func(string) (jwt.MapClaims, error)
		token     string
		wantErr   bool
		wantClaim map[string]any
	}{
		{
			name:   "access token verifies as access, with the user's claims",
			verify: maker.VerifyAccessToken, token: pair.AccessToken,
			wantClaim: map[string]any{
				platformconstant.SubKey:       float64(42),
				platformconstant.RoleKey:      "admin",
				platformconstant.EmailKey:     "jane@example.com",
				platformconstant.UserNameKey:  "Jane",
				platformconstant.TokenTypeKey: platformconstant.TokenTypeAccess,
			},
		},
		{
			name:   "refresh token verifies as refresh, without a role",
			verify: maker.VerifyRefreshToken, token: pair.RefreshToken,
			wantClaim: map[string]any{
				platformconstant.SubKey:       float64(42),
				platformconstant.TokenTypeKey: platformconstant.TokenTypeRefresh,
			},
		},
		{name: "refresh token is not an access token", verify: maker.VerifyAccessToken, token: pair.RefreshToken, wantErr: true},
		{name: "access token is not a refresh token", verify: maker.VerifyRefreshToken, token: pair.AccessToken, wantErr: true},
		{name: "garbage", verify: maker.VerifyAccessToken, token: "not-a-jwt", wantErr: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			claims, err := tc.verify(tc.token)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			for k, v := range tc.wantClaim {
				require.Equal(t, v, claims[k], k)
			}
			_, hasRole := claims[platformconstant.RoleKey]
			require.Equal(t, claims[platformconstant.TokenTypeKey] == platformconstant.TokenTypeAccess, hasRole)
		})
	}

	require.WithinDuration(t, time.Now().Add(15*time.Minute), pair.AccessTokenExpiresAt, 5*time.Second)
	require.WithinDuration(t, time.Now().Add(24*time.Hour), pair.RefreshTokenExpiresAt, 5*time.Second)

	// The refresh token's jti is the id the pair reports (its refresh_tokens row).
	refreshClaims, err := maker.VerifyRefreshToken(pair.RefreshToken)
	require.NoError(t, err)
	jti, err := TokenID(refreshClaims)
	require.NoError(t, err)
	require.Equal(t, pair.RefreshTokenID, jti)
}

func TestMakerES256_Verify_Rejections_U(t *testing.T) {
	key := newTestKey(t)
	maker := newTestMaker(t, key, "client", "issuer")
	expired := newTestMaker(t, key, "client", "issuer")
	expired.now = func() time.Time { return time.Now().Add(-time.Hour) }

	otherAudience := newTestMaker(t, key, "other-client", "issuer")
	otherIssuer := newTestMaker(t, key, "client", "other-issuer")
	otherKey := newTestMaker(t, newTestKey(t), "client", "issuer")

	sign := func(m *MakerES256) string {
		token, _, err := m.GenerateAccessToken(testUser, time.Minute)
		require.NoError(t, err)
		return token
	}
	hs256, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": 1}).SignedString([]byte("secret"))
	require.NoError(t, err)

	// A hand-built token: the claims a real access token has, signed with the
	// maker's key, minus or plus one thing per case.
	signed := func(edit func(jwt.MapClaims)) string {
		now := time.Now()
		claims := jwt.MapClaims{
			platformconstant.SubKey:            float64(42),
			platformconstant.TokenTypeKey:      platformconstant.TokenTypeAccess,
			platformconstant.IssuerKey:         "issuer",
			platformconstant.AudienceKey:       "client",
			platformconstant.ExpirationKey:     now.Add(time.Minute).Unix(),
			platformconstant.IssuedAtKey:       now.Unix(),
			platformconstant.JSONWebTokenIDKey: uuid.New().String(),
		}
		edit(claims)
		token, err := jwt.NewWithClaims(jwt.SigningMethodES256, claims).SignedString(key)
		require.NoError(t, err)
		return token
	}
	unsigned, err := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{
		platformconstant.SubKey:            float64(42),
		platformconstant.TokenTypeKey:      platformconstant.TokenTypeAccess,
		platformconstant.IssuerKey:         "issuer",
		platformconstant.AudienceKey:       "client",
		platformconstant.ExpirationKey:     time.Now().Add(time.Minute).Unix(),
		platformconstant.IssuedAtKey:       time.Now().Unix(),
		platformconstant.JSONWebTokenIDKey: uuid.New().String(),
	}).SignedString(jwt.UnsafeAllowNoneSignatureType)
	require.NoError(t, err)

	testCases := []struct {
		name         string
		token        string
		wantOK       bool
		wantErrIs    error
		wantContains string
	}{
		{name: "control: the hand-built token verifies", token: signed(func(jwt.MapClaims) {}), wantOK: true},
		{name: "expired", token: sign(expired), wantErrIs: jwt.ErrTokenExpired},
		{name: "another audience", token: sign(otherAudience), wantErrIs: jwt.ErrTokenInvalidAudience},
		{name: "another issuer", token: sign(otherIssuer), wantErrIs: jwt.ErrTokenInvalidIssuer},
		{name: "signed by another key", token: sign(otherKey), wantErrIs: jwt.ErrTokenSignatureInvalid},
		{name: "another algorithm", token: hs256, wantErrIs: jwt.ErrTokenSignatureInvalid},
		{name: "alg none (no signature) is refused", token: unsigned, wantErrIs: jwt.ErrTokenSignatureInvalid},
		{
			name:      "a token with no exp is refused (it would never expire)",
			token:     signed(func(c jwt.MapClaims) { delete(c, platformconstant.ExpirationKey) }),
			wantErrIs: jwt.ErrTokenRequiredClaimMissing,
		},
		{
			name: "a token issued in the future is refused",
			token: signed(func(c jwt.MapClaims) {
				c[platformconstant.IssuedAtKey] = time.Now().Add(time.Hour).Unix()
			}),
			wantErrIs: jwt.ErrTokenUsedBeforeIssued,
		},
		{
			name:         "a token with no jti is refused",
			token:        signed(func(c jwt.MapClaims) { delete(c, platformconstant.JSONWebTokenIDKey) }),
			wantContains: "jti",
		},
		{
			name:         "a token with an empty jti is refused",
			token:        signed(func(c jwt.MapClaims) { c[platformconstant.JSONWebTokenIDKey] = "" }),
			wantContains: "jti",
		},
		{
			name:         "a token with no sub is refused",
			token:        signed(func(c jwt.MapClaims) { delete(c, platformconstant.SubKey) }),
			wantContains: "sub",
		},
		{
			name:         "a token whose sub is a string is refused",
			token:        signed(func(c jwt.MapClaims) { c[platformconstant.SubKey] = "42" }),
			wantContains: "sub",
		},
		{
			name:         "a token with no typ is refused",
			token:        signed(func(c jwt.MapClaims) { delete(c, platformconstant.TokenTypeKey) }),
			wantContains: "token type",
		},
		{
			name:         "a token of an unknown typ is refused",
			token:        signed(func(c jwt.MapClaims) { c[platformconstant.TokenTypeKey] = "id" }),
			wantContains: "token type",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			claims, err := maker.VerifyAccessToken(tc.token)
			if tc.wantOK {
				require.NoError(t, err)
				require.NotNil(t, claims)
				return
			}
			require.Error(t, err)
			require.Nil(t, claims)
			if tc.wantErrIs != nil {
				require.ErrorIs(t, err, tc.wantErrIs)
			}
			if tc.wantContains != "" {
				require.ErrorContains(t, err, tc.wantContains)
			}
		})
	}
}

func TestMakerES256_SignErrors_U(t *testing.T) {
	// ES256 signs only with a P-256 key: a P-384 key (a wrong key file) must
	// fail cleanly, not issue a token.
	p384, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	require.NoError(t, err)
	maker := newTestMaker(t, p384, "client", "issuer")

	testCases := []struct {
		name string
		call func() (any, error)
		zero any
	}{
		{
			name: "GenerateAccessToken returns an error and no token",
			call: func() (any, error) {
				token, _, err := maker.GenerateAccessToken(testUser, time.Minute)
				return token, err
			},
			zero: "",
		},
		{
			name: "CreateToken returns an error and an empty pair",
			call: func() (any, error) { return maker.CreateToken(testUser, time.Minute, time.Hour) },
			zero: Pair{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.call()
			require.ErrorIs(t, err, jwt.ErrInvalidKey)
			require.ErrorContains(t, err, "sign access token")
			require.Equal(t, tc.zero, got)
		})
	}
}

func TestSubjectID_U(t *testing.T) {
	testCases := []struct {
		name    string
		claims  jwt.MapClaims
		want    int64
		wantErr bool
	}{
		{name: "a float64 sub", claims: jwt.MapClaims{"sub": float64(7)}, want: 7},
		{name: "missing", claims: jwt.MapClaims{}, wantErr: true},
		{name: "zero", claims: jwt.MapClaims{"sub": float64(0)}, wantErr: true},
		{name: "a string", claims: jwt.MapClaims{"sub": "7"}, wantErr: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := SubjectID(tc.claims)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestTokenID_U(t *testing.T) {
	id := uuid.New()

	testCases := []struct {
		name    string
		claims  jwt.MapClaims
		want    uuid.UUID
		wantErr bool
	}{
		{name: "a uuid jti", claims: jwt.MapClaims{"jti": id.String()}, want: id},
		{name: "missing", claims: jwt.MapClaims{}, wantErr: true},
		{name: "not a uuid", claims: jwt.MapClaims{"jti": "abc"}, wantErr: true},
		{name: "not a string", claims: jwt.MapClaims{"jti": float64(1)}, wantErr: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := TokenID(tc.claims)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestLoadECKeys_U(t *testing.T) {
	dir := t.TempDir()
	key := newTestKey(t)

	privDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	pubDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	require.NoError(t, err)

	write := func(name string, data []byte) string {
		p := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(p, data, 0o600))
		return p
	}
	privPath := write("private.pem", pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: privDER}))
	pubPath := write("public.pem", pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER}))
	garbagePath := write("garbage.pem", []byte("not pem"))
	missingPath := filepath.Join(dir, "missing.pem")

	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	rsaPubDER, err := x509.MarshalPKIXPublicKey(&rsaKey.PublicKey)
	require.NoError(t, err)
	rsaPubPath := write("rsa-public.pem", pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: rsaPubDER}))
	rsaPrivPath := write("rsa-private.pem", pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(rsaKey)}))
	junkPubPath := write("junk-public.pem", pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: []byte("junk")}))
	// The private key PEM where the public one belongs (an operator mix-up).
	swappedPath := write("swapped.pem", pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: privDER}))

	testCases := []struct {
		name         string
		load         func() error
		wantErr      bool
		wantContains string
	}{
		{
			name:         "an RSA public key is refused",
			load:         func() error { _, err := LoadECPublicKey(rsaPubPath); return err },
			wantErr:      true,
			wantContains: "not an ECDSA key",
		},
		{name: "a PEM block that isn't a public key is refused", load: func() error { _, err := LoadECPublicKey(junkPubPath); return err }, wantErr: true},
		{name: "the private key file given as the public key is refused", load: func() error { _, err := LoadECPublicKey(swappedPath); return err }, wantErr: true},
		{name: "an RSA private key is refused", load: func() error { _, err := LoadECPrivateKey(rsaPrivPath); return err }, wantErr: true},
		{name: "private key", load: func() error { _, err := LoadECPrivateKey(privPath); return err }},
		{name: "public key", load: func() error { _, err := LoadECPublicKey(pubPath); return err }},
		{name: "private key not PEM", load: func() error { _, err := LoadECPrivateKey(garbagePath); return err }, wantErr: true},
		{name: "public key not PEM", load: func() error { _, err := LoadECPublicKey(garbagePath); return err }, wantErr: true},
		{name: "private key missing", load: func() error { _, err := LoadECPrivateKey(missingPath); return err }, wantErr: true},
		{name: "public key missing", load: func() error { _, err := LoadECPublicKey(missingPath); return err }, wantErr: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.load()
			if tc.wantErr {
				require.Error(t, err)
				if tc.wantContains != "" {
					require.ErrorContains(t, err, tc.wantContains)
				}
				return
			}
			require.NoError(t, err)
		})
	}
}
