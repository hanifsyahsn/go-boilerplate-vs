package revoke

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	authshared "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/apperror"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/testhelper"
	"github.com/stretchr/testify/require"
)

func TestService_Revoke_I(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	hourAgo := time.Now().Add(-time.Hour)

	testCases := []struct {
		name string
		// arrange seeds the target session (and its token) and returns it.
		arrange  func(t *testing.T, me, other int64) (family, token uuid.UUID)
		wantCode apperror.Code
		// wantReason is the target's revoke_reason afterwards ("" = untouched).
		wantReason string
	}{
		{
			name: "my other live session: revoked as signed_out_remotely with its refresh tokens",
			arrange: func(t *testing.T, me, _ int64) (uuid.UUID, uuid.UUID) {
				token := uuid.New()
				return testhelper.SeedRefreshToken(t, testDB, testhelper.SeedRefreshTokenParams{Jti: token, UserID: me}), token
			},
			wantReason: authshared.RevokeReasonSignedOutRemotely,
		},
		{
			name: "another user's session is a 404 and stays live",
			arrange: func(t *testing.T, _, other int64) (uuid.UUID, uuid.UUID) {
				token := uuid.New()
				return testhelper.SeedRefreshToken(t, testDB, testhelper.SeedRefreshTokenParams{Jti: token, UserID: other}), token
			},
			wantCode: apperror.CodeNotFound,
		},
		{
			name: "a session already signed out is a 404 and keeps its first reason",
			arrange: func(t *testing.T, me, _ int64) (uuid.UUID, uuid.UUID) {
				family := testhelper.SeedSession(t, testDB, testhelper.SeedSessionParams{UserID: me, RevokeReason: authshared.RevokeReasonLogout})
				token := uuid.New()
				testhelper.SeedRefreshToken(t, testDB, testhelper.SeedRefreshTokenParams{Jti: token, UserID: me, FamilyID: family, RevokedAt: &hourAgo})
				return family, token
			},
			wantCode: apperror.CodeNotFound, wantReason: authshared.RevokeReasonLogout,
		},
		{
			name: "a session whose tokens all expired is a 404",
			arrange: func(t *testing.T, me, _ int64) (uuid.UUID, uuid.UUID) {
				token := uuid.New()
				return testhelper.SeedRefreshToken(t, testDB, testhelper.SeedRefreshTokenParams{Jti: token, UserID: me, ExpiresAt: hourAgo}), token
			},
			wantCode: apperror.CodeNotFound,
		},
		{
			name: "an untrusted session past its cap is a 404 (not live, as the list shows it)",
			arrange: func(t *testing.T, me, _ int64) (uuid.UUID, uuid.UUID) {
				token := uuid.New()
				return testhelper.SeedRefreshToken(t, testDB, testhelper.SeedRefreshTokenParams{Jti: token, UserID: me, SessionExpiresAt: &hourAgo}), token
			},
			wantCode: apperror.CodeNotFound,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer testhelper.Truncate(t, testDB)
			me := testhelper.SeedUser(t, testDB, testhelper.SeedUserParams{})
			other := testhelper.SeedUser(t, testDB, testhelper.SeedUserParams{})
			mine := uuid.New()
			mySession := testhelper.SeedRefreshToken(t, testDB, testhelper.SeedRefreshTokenParams{Jti: mine, UserID: me})
			family, token := tc.arrange(t, me, other)

			err := NewService(NewStore(testDB)).Revoke(context.Background(), me, family.String())
			if !tc.wantCode.IsZero() {
				var appErr *apperror.AppError
				require.ErrorAs(t, err, &appErr)
				require.Equal(t, tc.wantCode, appErr.Code)
			} else {
				require.NoError(t, err)
				require.True(t, testhelper.GetRefreshToken(t, testDB, token).RevokedAt.Valid, "the session's refresh token is revoked")
			}

			session := testhelper.GetSession(t, testDB, family)
			require.Equal(t, tc.wantReason, session.RevokeReason.String)
			require.Equal(t, tc.wantReason != "", session.RevokedAt.Valid)
			require.False(t, testhelper.GetSession(t, testDB, mySession).RevokedAt.Valid, "the caller's other session is untouched")
			require.False(t, testhelper.GetRefreshToken(t, testDB, mine).RevokedAt.Valid)
		})
	}
}
