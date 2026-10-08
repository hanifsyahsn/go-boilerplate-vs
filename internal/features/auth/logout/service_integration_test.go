package logout

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"testing"
	"time"

	"github.com/google/uuid"
	authsharedsqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/jwttoken"
	internalshareddto "github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/dto"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/testhelper"
	"github.com/stretchr/testify/require"
)

func TestService_Logout_I(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	maker := jwttoken.NewTokenMakerES256(key, &key.PublicKey, "client", "issuer")

	testCases := []struct {
		name string
		// presentOwn: log out with the user's token (else with no token).
		presentOwn bool
		// wantFamilyRevoked: the presented token's family (it + an earlier
		// token of the same login); another login of the same user is
		// never touched.
		wantFamilyRevoked bool
	}{
		{name: "the token's login is revoked, the other login kept", presentOwn: true, wantFamilyRevoked: true},
		{name: "no token: nothing revoked", presentOwn: false, wantFamilyRevoked: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer testhelper.Truncate(t, testDB)
			userID := testhelper.SeedUser(t, testDB, testhelper.SeedUserParams{})

			pair, err := maker.CreateToken(internalshareddto.User{ID: userID}, time.Minute, time.Hour)
			require.NoError(t, err)
			familyID := testhelper.SeedRefreshToken(t, testDB, testhelper.SeedRefreshTokenParams{Jti: pair.RefreshTokenID, UserID: userID})
			sibling := uuid.New()
			testhelper.SeedRefreshToken(t, testDB, testhelper.SeedRefreshTokenParams{Jti: sibling, UserID: userID, FamilyID: familyID})
			otherLogin := uuid.New()
			testhelper.SeedRefreshToken(t, testDB, testhelper.SeedRefreshTokenParams{Jti: otherLogin, UserID: userID})

			token := ""
			if tc.presentOwn {
				token = pair.RefreshToken
			}
			require.NoError(t, NewService(authsharedsqlc.New(testDB), maker).Logout(context.Background(), token))

			require.Equal(t, tc.wantFamilyRevoked, testhelper.GetRefreshToken(t, testDB, pair.RefreshTokenID).RevokedAt.Valid)
			require.Equal(t, tc.wantFamilyRevoked, testhelper.GetRefreshToken(t, testDB, sibling).RevokedAt.Valid)
			require.False(t, testhelper.GetRefreshToken(t, testDB, otherLogin).RevokedAt.Valid)
		})
	}
}
