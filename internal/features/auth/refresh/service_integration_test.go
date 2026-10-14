package refresh

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"testing"
	"time"

	"github.com/google/uuid"
	platformconstant "github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/constant"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/jwttoken"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	internalshareddto "github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/dto"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/enum"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/testhelper"
	"github.com/stretchr/testify/require"
)

func TestService_Refresh_I(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	const refreshTTL = 7 * 24 * time.Hour

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	maker := jwttoken.NewTokenMakerES256(key, &key.PublicKey, "client", "issuer")

	minuteAgo := time.Now().Add(-time.Minute)
	inAnHour := time.Now().Add(time.Hour)

	testCases := []struct {
		name string
		seed testhelper.SeedUserParams
		// stored is how the presented token's row looks; nil => no row.
		stored func(sibling uuid.UUID) *testhelper.SeedRefreshTokenParams
		// siblingRevoked: whether the family's other live token ends revoked.
		siblingRevoked bool
		// wantCap: the session cap the new token inherits, nil => none.
		wantCap  *time.Time
		wantCode apperror.Code
	}{
		{
			name:   "a live token: rotated, a new pair in the same family with the role from the DB",
			seed:   testhelper.SeedUserParams{Role: "admin"},
			stored: func(uuid.UUID) *testhelper.SeedRefreshTokenParams { return &testhelper.SeedRefreshTokenParams{} },
		},
		{
			name: "an untrusted device near its cap: the new refresh token ends at the cap and inherits it",
			seed: testhelper.SeedUserParams{Role: "user"},
			stored: func(uuid.UUID) *testhelper.SeedRefreshTokenParams {
				return &testhelper.SeedRefreshTokenParams{SessionExpiresAt: &inAnHour}
			},
			wantCap: &inAnHour,
		},
		{
			name: "an untrusted device past its cap is refused",
			stored: func(uuid.UUID) *testhelper.SeedRefreshTokenParams {
				return &testhelper.SeedRefreshTokenParams{SessionExpiresAt: &minuteAgo}
			},
			wantCode: apperror.CodeUnauthorized,
		},
		{
			name:     "a deactivated user gets none, the token stays live",
			seed:     testhelper.SeedUserParams{IsActive: testhelper.BoolPtr(false)},
			stored:   func(uuid.UUID) *testhelper.SeedRefreshTokenParams { return &testhelper.SeedRefreshTokenParams{} },
			wantCode: apperror.CodeUnauthorized,
		},
		{
			name: "a spent token presented again revokes the family",
			stored: func(sibling uuid.UUID) *testhelper.SeedRefreshTokenParams {
				return &testhelper.SeedRefreshTokenParams{RevokedAt: &minuteAgo, ReplacedBy: uuid.NullUUID{UUID: sibling, Valid: true}}
			},
			siblingRevoked: true,
			wantCode:       apperror.CodeUnauthorized,
		},
		{
			name:     "a token with no row (unknown) is refused",
			stored:   func(uuid.UUID) *testhelper.SeedRefreshTokenParams { return nil },
			wantCode: apperror.CodeUnauthorized,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer testhelper.Truncate(t, testDB)
			userID := testhelper.SeedUser(t, testDB, tc.seed)

			// The pair was issued while the user was a plain user; the DB
			// row may say otherwise now.
			pair, err := maker.CreateToken(internalshareddto.User{ID: userID, Role: enum.RoleUser}, time.Minute, refreshTTL)
			require.NoError(t, err)

			// Another live token of the same login (the presented one's
			// successor, in the reuse case).
			siblingID := uuid.New()
			var familyID uuid.UUID
			if stored := tc.stored(siblingID); stored != nil {
				stored.Jti = pair.RefreshTokenID
				stored.UserID = userID
				familyID = testhelper.SeedRefreshToken(t, testDB, *stored)
				testhelper.SeedRefreshToken(t, testDB, testhelper.SeedRefreshTokenParams{Jti: siblingID, UserID: userID, FamilyID: familyID})
			}

			got, err := NewService(NewStore(testDB), maker, time.Minute, refreshTTL).Refresh(context.Background(), pair.RefreshToken)
			if tc.wantCode != "" {
				var appErr *apperror.AppError
				require.ErrorAs(t, err, &appErr)
				require.Equal(t, tc.wantCode, appErr.Code)

				if familyID != uuid.Nil {
					require.Equal(t, tc.siblingRevoked, testhelper.GetRefreshToken(t, testDB, siblingID).RevokedAt.Valid)
				}
				return
			}
			require.NoError(t, err)

			claims, err := maker.VerifyAccessToken(got.Tokens.AccessToken)
			require.NoError(t, err)
			require.Equal(t, tc.seed.Role, claims[platformconstant.RoleKey])

			_, err = maker.VerifyRefreshToken(got.Tokens.RefreshToken)
			require.NoError(t, err)
			wantExp := time.Now().Add(refreshTTL)
			if tc.wantCap != nil {
				wantExp = *tc.wantCap
			}
			require.WithinDuration(t, wantExp, got.Tokens.RefreshTokenExpiresAt, 5*time.Second)

			old := testhelper.GetRefreshToken(t, testDB, pair.RefreshTokenID)
			require.True(t, old.RevokedAt.Valid)
			require.Equal(t, uuid.NullUUID{UUID: got.Tokens.RefreshTokenID, Valid: true}, old.ReplacedBy)

			next := testhelper.GetRefreshToken(t, testDB, got.Tokens.RefreshTokenID)
			require.Equal(t, familyID, next.FamilyID)
			require.Equal(t, userID, next.UserID)
			require.False(t, next.RevokedAt.Valid)
			if tc.wantCap != nil {
				require.True(t, next.SessionExpiresAt.Valid)
				require.WithinDuration(t, *tc.wantCap, next.SessionExpiresAt.Time, time.Millisecond)
			} else {
				require.False(t, next.SessionExpiresAt.Valid)
			}
		})
	}
}
