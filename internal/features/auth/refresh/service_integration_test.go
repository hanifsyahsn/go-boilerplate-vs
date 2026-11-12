package refresh

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"testing"
	"time"

	"github.com/google/uuid"
	authshared "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared"
	authsharedsqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared/sqlc"
	platformconstant "github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/constant"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/jwttoken"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/middleware"
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
		// sessionRevokedAs, when set, seeds the family's session already
		// revoked with that reason (a signed-out device).
		sessionRevokedAs string
		// siblingRevoked: whether the family's other live token ends revoked.
		siblingRevoked bool
		// wantSessionReason: the session's revoke_reason after the call ("" = live).
		wantSessionReason string
		// wantCap: the session cap the new token inherits, nil => none.
		wantCap  *time.Time
		wantCode apperror.Code
		// wantDetail, when set, is the error's user-facing message.
		wantDetail string
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
			siblingRevoked:    true,
			wantSessionReason: authshared.RevokeReasonTokenReused,
			wantCode:          apperror.CodeUnauthorized,
		},
		{
			name:              "a live token of a signed-out session is refused with 'your session has ended' and the session stays as it was",
			stored:            func(uuid.UUID) *testhelper.SeedRefreshTokenParams { return &testhelper.SeedRefreshTokenParams{} },
			sessionRevokedAs:  authshared.RevokeReasonSignedOutRemotely,
			wantSessionReason: authshared.RevokeReasonSignedOutRemotely,
			wantCode:          apperror.CodeUnauthorized,
			wantDetail:        middleware.SessionEndedMessage,
		},
		{
			name:              "a token of an account an admin deactivated (its session revoked with it) is refused as 'no longer active'",
			seed:              testhelper.SeedUserParams{IsActive: testhelper.BoolPtr(false)},
			stored:            func(uuid.UUID) *testhelper.SeedRefreshTokenParams { return &testhelper.SeedRefreshTokenParams{} },
			sessionRevokedAs:  authshared.RevokeReasonDeactivated,
			wantSessionReason: authshared.RevokeReasonDeactivated,
			wantCode:          apperror.CodeUnauthorized,
			wantDetail:        accountInactiveMessage,
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
			familyID := uuid.New()
			pair, err := maker.CreateToken(internalshareddto.User{ID: userID, Role: enum.RoleUser}, familyID, time.Minute, refreshTTL)
			require.NoError(t, err)
			if tc.sessionRevokedAs != "" {
				testhelper.SeedSession(t, testDB, testhelper.SeedSessionParams{FamilyID: familyID, UserID: userID, RevokeReason: tc.sessionRevokedAs})
			}

			// Another live token of the same login (the presented one's
			// successor, in the reuse case).
			siblingID := uuid.New()
			seeded := false
			if stored := tc.stored(siblingID); stored != nil {
				stored.Jti = pair.RefreshTokenID
				stored.UserID = userID
				stored.FamilyID = familyID
				testhelper.SeedRefreshToken(t, testDB, *stored)
				testhelper.SeedRefreshToken(t, testDB, testhelper.SeedRefreshTokenParams{Jti: siblingID, UserID: userID, FamilyID: familyID})
				seeded = true
			}

			client := authshared.SessionClient{UserAgent: "Mozilla/5.0 (phone)", IP: "198.51.100.4"}
			got, err := NewService(NewStore(testDB), maker, time.Minute, refreshTTL).Refresh(context.Background(), pair.RefreshToken, client)
			if !tc.wantCode.IsZero() {
				var appErr *apperror.AppError
				require.ErrorAs(t, err, &appErr)
				require.Equal(t, tc.wantCode, appErr.Code)
				if tc.wantDetail != "" {
					require.Equal(t, tc.wantDetail, appErr.Message)
				}

				if seeded {
					require.Equal(t, tc.siblingRevoked, testhelper.GetRefreshToken(t, testDB, siblingID).RevokedAt.Valid)
					session := testhelper.GetSession(t, testDB, familyID)
					require.Equal(t, tc.wantSessionReason, session.RevokeReason.String)
					require.Equal(t, tc.wantSessionReason != "", session.RevokedAt.Valid)
					require.Empty(t, session.UserAgent, "a refused refresh doesn't touch the session")
				}
				return
			}
			require.NoError(t, err)

			claims, err := maker.VerifyAccessToken(got.Tokens.AccessToken)
			require.NoError(t, err)
			require.Equal(t, tc.seed.Role, claims[platformconstant.RoleKey])
			require.Equal(t, familyID.String(), claims[platformconstant.SessionIDKey], "the new access token keeps the session")

			// The session's use is recorded.
			session := testhelper.GetSession(t, testDB, familyID)
			require.False(t, session.RevokedAt.Valid)
			require.Equal(t, "Mozilla/5.0 (phone)", session.UserAgent)
			require.Equal(t, "198.51.100.4", session.ClientIP)
			require.WithinDuration(t, time.Now(), session.LastUsedAt, 5*time.Second)

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

func TestService_Refresh_RevokeInFlight_I(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	maker := jwttoken.NewTokenMakerES256(key, &key.PublicKey, "client", "issuer")

	testCases := []struct {
		name string
	}{
		{name: "a refresh while a revoke holds the session waits for it, then is refused as 'session ended' with nothing issued (no deadlock)"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer testhelper.Truncate(t, testDB)
			userID := testhelper.SeedUser(t, testDB, testhelper.SeedUserParams{})
			familyID := uuid.New()
			pair, err := maker.CreateToken(internalshareddto.User{ID: userID, Role: enum.RoleUser}, familyID, time.Minute, time.Hour)
			require.NoError(t, err)
			testhelper.SeedRefreshToken(t, testDB, testhelper.SeedRefreshTokenParams{Jti: pair.RefreshTokenID, UserID: userID, FamilyID: familyID})

			// A revoke has locked the session row (the order every revoke follows).
			revoker := testhelper.LockSessionRow(t, testDB, familyID)

			done := testhelper.Async(func() error {
				_, err := NewService(NewStore(testDB), maker, time.Minute, time.Hour).
					Refresh(context.Background(), pair.RefreshToken, authshared.SessionClient{})
				return err
			})
			testhelper.RequireBlocked(t, done, 300*time.Millisecond)

			_, err = authsharedsqlc.New(testDB).WithTx(revoker).RevokeSession(context.Background(), authsharedsqlc.RevokeSessionParams{
				FamilyID: familyID, Reason: authshared.RevokeReasonSignedOutRemotely,
			})
			require.NoError(t, err, "the revoke never waits on the refresh")
			require.NoError(t, revoker.Commit())

			var appErr *apperror.AppError
			require.ErrorAs(t, testhelper.Await(t, done), &appErr)
			require.Equal(t, apperror.CodeUnauthorized, appErr.Code)
			require.Equal(t, middleware.SessionEndedMessage, appErr.Message)

			var tokens int
			require.NoError(t, testDB.QueryRow(`SELECT count(*) FROM refresh_tokens WHERE family_id = $1`, familyID).Scan(&tokens))
			require.Equal(t, 1, tokens, "no successor was issued")
			require.False(t, testhelper.GetRefreshToken(t, testDB, pair.RefreshTokenID).ReplacedBy.Valid)
		})
	}
}
