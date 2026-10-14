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

			got, err := NewService(sqlc.New(testDB), authsharedsqlc.New(testDB), maker, time.Minute, time.Hour, untrustedTTL).Login(context.Background(), tc.req)
			if tc.wantCode != "" {
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
