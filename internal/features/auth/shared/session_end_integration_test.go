package shared

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	authsharedsqlc "github.com/hanifsyahsn/go_boilerplate_vs/internal/features/auth/shared/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/testhelper"
	"github.com/stretchr/testify/require"
)

// TestEndSessions_RefreshInFlight_I: a revoke that starts while a refresh
// holds the session waits for it, then also revokes the token that refresh
// rotated in. Before the session-first lock order, the revoke's statement
// missed that token (it was committed after its snapshot) and left it live
// under a revoked session.
func TestEndSessions_RefreshInFlight_I(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	testCases := []struct {
		name string
		end  func(ctx context.Context, q authsharedsqlc.Querier, userID int64, familyID uuid.UUID) error
	}{
		{
			name: "EndSession (logout, reuse): waits for the refresh, then the rotated-in token is revoked too",
			end: func(ctx context.Context, q authsharedsqlc.Querier, _ int64, familyID uuid.UUID) error {
				_, err := EndSession(ctx, q, familyID, RevokeReasonLogout)
				return err
			},
		},
		{
			name: "EndUserSessions (password change / reset, deactivation): waits for the refresh, then the rotated-in token is revoked too",
			end: func(ctx context.Context, q authsharedsqlc.Querier, userID int64, _ uuid.UUID) error {
				_, err := EndUserSessions(ctx, q, userID, uuid.NullUUID{}, RevokeReasonPasswordReset)
				return err
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer testhelper.Truncate(t, testDB)
			userID := testhelper.SeedUser(t, testDB, testhelper.SeedUserParams{})
			familyID := uuid.New()
			oldJTI := uuid.New()
			testhelper.SeedRefreshToken(t, testDB, testhelper.SeedRefreshTokenParams{Jti: oldJTI, UserID: userID, FamilyID: familyID})

			rotation := testhelper.BeginRotation(t, testDB, familyID, oldJTI, userID)

			done := testhelper.Async(func() error {
				tx, err := testDB.BeginTx(context.Background(), nil)
				if err != nil {
					return err
				}
				defer func() { _ = tx.Rollback() }()
				if err = tc.end(context.Background(), authsharedsqlc.New(tx), userID, familyID); err != nil {
					return err
				}
				return tx.Commit()
			})
			testhelper.RequireBlocked(t, done, 300*time.Millisecond)
			rotation.Commit(t)
			require.NoError(t, testhelper.Await(t, done))

			require.True(t, testhelper.GetSession(t, testDB, familyID).RevokedAt.Valid)
			require.True(t, testhelper.GetRefreshToken(t, testDB, rotation.NewJTI).RevokedAt.Valid,
				"the token the refresh rotated in is revoked with its session")
		})
	}
}
