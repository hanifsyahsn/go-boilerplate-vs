package login

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"testing"
	"time"

	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/login/sqlc"
	authsharedsqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared/sqlc"
	platformconstant "github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/constant"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/jwttoken"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/ratelimit"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/testhelper"
	"github.com/stretchr/testify/require"
)

func TestService_Login_I(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	maker := jwttoken.NewTokenMakerES256(key, &key.PublicKey, "client", "issuer")

	const untrustedTTL = 30 * time.Minute

	testCases := []struct {
		name     string
		seed     testhelper.SeedUserParams
		req      Request
		wantCode apperror.Code
	}{
		{
			name: "matching credentials, email case ignored, untrusted device: the session capped",
			seed: testhelper.SeedUserParams{Email: "jane@example.com", Role: "admin"},
			req:  Request{Email: "JANE@example.com", Password: testhelper.DefaultPassword},
		},
		{
			name: "trusted device: no cap",
			seed: testhelper.SeedUserParams{Email: "jane@example.com", Role: "admin"},
			req:  Request{Email: "jane@example.com", Password: testhelper.DefaultPassword, TrustDevice: true},
		},
		{
			name:     "wrong password",
			seed:     testhelper.SeedUserParams{Email: "jane@example.com"},
			req:      Request{Email: "jane@example.com", Password: "wrong"},
			wantCode: apperror.CodeUnauthorized,
		},
		{
			name:     "deactivated",
			seed:     testhelper.SeedUserParams{Email: "jane@example.com", IsActive: testhelper.BoolPtr(false)},
			req:      Request{Email: "jane@example.com", Password: testhelper.DefaultPassword},
			wantCode: apperror.CodeUnauthorized,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer testhelper.Truncate(t, testDB)
			userID := testhelper.SeedUser(t, testDB, tc.seed)

			got, err := NewService(sqlc.New(testDB), authsharedsqlc.New(testDB), maker, time.Minute, time.Hour, untrustedTTL,
				Limiters{
					Email: ratelimit.NewMemory(ratelimit.Rate{Burst: 5, Per: time.Minute}),
					IP:    ratelimit.NewMemory(ratelimit.Rate{Burst: 5, Per: time.Minute}),
				}).Login(context.Background(), tc.req, Client{IP: "203.0.113.7", UserAgent: "Mozilla/5.0 (test)"})
			if !tc.wantCode.IsZero() {
				var appErr *apperror.AppError
				require.ErrorAs(t, err, &appErr)
				require.Equal(t, tc.wantCode, appErr.Code)
				return
			}
			require.NoError(t, err)

			claims, err := maker.VerifyAccessToken(got.Tokens.AccessToken)
			require.NoError(t, err)
			require.Equal(t, float64(userID), claims[platformconstant.SubKey])
			require.Equal(t, "admin", claims[platformconstant.RoleKey])

			_, err = maker.VerifyRefreshToken(got.Tokens.RefreshToken)
			require.NoError(t, err)

			stored := testhelper.GetRefreshToken(t, testDB, got.Tokens.RefreshTokenID)
			require.Equal(t, userID, stored.UserID)
			require.False(t, stored.RevokedAt.Valid)

			// The login's session: the family, carried by the access token as sid.
			require.Equal(t, stored.FamilyID.String(), claims[platformconstant.SessionIDKey])
			session := testhelper.GetSession(t, testDB, stored.FamilyID)
			require.Equal(t, userID, session.UserID)
			require.Equal(t, tc.req.TrustDevice, session.Trusted)
			require.Equal(t, "Mozilla/5.0 (test)", session.UserAgent)
			require.Equal(t, "203.0.113.7", session.ClientIP)
			require.False(t, session.RevokedAt.Valid)
			if tc.req.TrustDevice {
				require.False(t, stored.SessionExpiresAt.Valid)
				require.WithinDuration(t, time.Now().Add(time.Hour), got.Tokens.RefreshTokenExpiresAt, 5*time.Second)
			} else {
				require.True(t, stored.SessionExpiresAt.Valid)
				require.WithinDuration(t, time.Now().Add(untrustedTTL), stored.SessionExpiresAt.Time, 5*time.Second)
				require.WithinDuration(t, time.Now().Add(untrustedTTL), got.Tokens.RefreshTokenExpiresAt, 5*time.Second)
			}
		})
	}
}

// TestService_Login_FoldedLikeTheLookup_I: Postgres (glibc, Unicode 16) lowers
// U+A7DC to U+019B ("ƛ"); Go's strings.ToLower leaves it. Both spellings reach
// the same account, so they must share one failure count, or an attacker gets
// twice the guesses by alternating them.
func TestService_Login_FoldedLikeTheLookup_I(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	maker := jwttoken.NewTokenMakerES256(key, &key.PublicKey, "client", "issuer")

	testCases := []struct {
		name      string
		spellings []string // used in turn for the wrong guesses
	}{
		{name: "two spellings the database folds to one account share one count: the 6th wrong guess is a 429",
			spellings: []string{"ƛjane@example.com", "Ƛjane@example.com"}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer testhelper.Truncate(t, testDB)
			testhelper.SeedUser(t, testDB, testhelper.SeedUserParams{Email: "ƛjane@example.com"})

			var folded string
			require.NoError(t, testDB.QueryRow(`SELECT lower($1::varchar)`, "Ƛjane@example.com").Scan(&folded))
			require.Equal(t, "ƛjane@example.com", folded, "precondition: this database folds U+A7DC to U+019B")

			svc := NewService(sqlc.New(testDB), authsharedsqlc.New(testDB), maker, time.Minute, time.Hour, time.Hour,
				Limiters{
					Email: ratelimit.NewMemory(ratelimit.Rate{Burst: 5, Per: 15 * time.Minute}),
					IP:    ratelimit.NewMemory(ratelimit.Rate{Burst: 100, Per: time.Hour}),
				})
			for i := range 6 {
				email := tc.spellings[i%len(tc.spellings)]
				_, err := svc.Login(context.Background(), Request{Email: email, Password: "wrong-password"}, Client{IP: "203.0.113.7"})
				var appErr *apperror.AppError
				require.ErrorAs(t, err, &appErr, "attempt %d", i+1)
				want := apperror.CodeUnauthorized
				if i == 5 {
					want = apperror.CodeTooManyRequests
				}
				require.Equal(t, want, appErr.Code, "attempt %d (%q)", i+1, email)
			}
		})
	}
}
