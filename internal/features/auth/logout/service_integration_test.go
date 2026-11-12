package logout

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/logout/sqlc"
	authshared "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared"
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
		{name: "the token's session is revoked as logout (its tokens too), the other login kept", presentOwn: true, wantFamilyRevoked: true},
		{name: "no token: nothing revoked", presentOwn: false, wantFamilyRevoked: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer testhelper.Truncate(t, testDB)
			userID := testhelper.SeedUser(t, testDB, testhelper.SeedUserParams{})

			familyID := uuid.New()
			pair, err := maker.CreateToken(internalshareddto.User{ID: userID}, familyID, time.Minute, time.Hour)
			require.NoError(t, err)
			testhelper.SeedRefreshToken(t, testDB, testhelper.SeedRefreshTokenParams{Jti: pair.RefreshTokenID, UserID: userID, FamilyID: familyID})
			sibling := uuid.New()
			testhelper.SeedRefreshToken(t, testDB, testhelper.SeedRefreshTokenParams{Jti: sibling, UserID: userID, FamilyID: familyID})
			otherLogin := uuid.New()
			otherFamily := testhelper.SeedRefreshToken(t, testDB, testhelper.SeedRefreshTokenParams{Jti: otherLogin, UserID: userID})

			token := ""
			if tc.presentOwn {
				token = pair.RefreshToken
			}
			require.NoError(t, NewService(sqlc.New(testDB), NewStore(testDB), maker).Logout(context.Background(), token))

			require.Equal(t, tc.wantFamilyRevoked, testhelper.GetRefreshToken(t, testDB, pair.RefreshTokenID).RevokedAt.Valid)
			require.Equal(t, tc.wantFamilyRevoked, testhelper.GetRefreshToken(t, testDB, sibling).RevokedAt.Valid)
			require.False(t, testhelper.GetRefreshToken(t, testDB, otherLogin).RevokedAt.Valid)

			session := testhelper.GetSession(t, testDB, familyID)
			require.Equal(t, tc.wantFamilyRevoked, session.RevokedAt.Valid)
			if tc.wantFamilyRevoked {
				require.Equal(t, authshared.RevokeReasonLogout, session.RevokeReason.String)
			}
			require.False(t, testhelper.GetSession(t, testDB, otherFamily).RevokedAt.Valid)
		})
	}
}

func TestService_Logout_RefreshInFlight_I(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	maker := jwttoken.NewTokenMakerES256(key, &key.PublicKey, "client", "issuer")

	testCases := []struct {
		name string
	}{
		{name: "a logout while a refresh of the same session is in flight waits for it, then revokes the token it rotated in too"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer testhelper.Truncate(t, testDB)
			userID := testhelper.SeedUser(t, testDB, testhelper.SeedUserParams{})
			familyID := uuid.New()
			pair, err := maker.CreateToken(internalshareddto.User{ID: userID}, familyID, time.Minute, time.Hour)
			require.NoError(t, err)
			testhelper.SeedRefreshToken(t, testDB, testhelper.SeedRefreshTokenParams{Jti: pair.RefreshTokenID, UserID: userID, FamilyID: familyID})

			rotation := testhelper.BeginRotation(t, testDB, familyID, pair.RefreshTokenID, userID)
			done := testhelper.Async(func() error {
				return NewService(sqlc.New(testDB), NewStore(testDB), maker).Logout(context.Background(), pair.RefreshToken)
			})
			testhelper.RequireBlocked(t, done, 300*time.Millisecond)
			rotation.Commit(t)
			require.NoError(t, testhelper.Await(t, done))

			session := testhelper.GetSession(t, testDB, familyID)
			require.True(t, session.RevokedAt.Valid)
			require.Equal(t, authshared.RevokeReasonLogout, session.RevokeReason.String)
			require.True(t, testhelper.GetRefreshToken(t, testDB, rotation.NewJTI).RevokedAt.Valid)
		})
	}
}
