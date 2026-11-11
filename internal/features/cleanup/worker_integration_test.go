package cleanup

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/cleanup/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/testhelper"
	"github.com/stretchr/testify/require"
)

func countRows(t *testing.T, table string) int {
	t.Helper()
	var n int
	require.NoError(t, testDB.QueryRow(`SELECT count(*) FROM `+table).Scan(&n)) // #nosec G202 -- a table name from the test itself
	return n
}

func exists(t *testing.T, query string, arg any) bool {
	t.Helper()
	var ok bool
	require.NoError(t, testDB.QueryRow(`SELECT EXISTS (`+query+`)`, arg).Scan(&ok)) // #nosec G202 -- a query from the test itself
	return ok
}

func TestWorker_RunOnce_I(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	hourAgo, inAnHour := time.Now().Add(-time.Hour), time.Now().Add(time.Hour)

	testCases := []struct {
		name      string
		retention time.Duration
		batchSize int32
		run       func(t *testing.T, w *Worker, userID int64)
	}{
		{name: "refresh tokens: expired and past-cap ones deleted, a live one and a revoked-but-unexpired one kept",
			retention: 0, batchSize: 10_000, run: func(t *testing.T, w *Worker, userID int64) {
				expired, capped, live, revoked := uuid.New(), uuid.New(), uuid.New(), uuid.New()
				testhelper.SeedRefreshToken(t, testDB, testhelper.SeedRefreshTokenParams{Jti: expired, UserID: userID, ExpiresAt: hourAgo})
				testhelper.SeedRefreshToken(t, testDB, testhelper.SeedRefreshTokenParams{Jti: capped, UserID: userID, SessionExpiresAt: &hourAgo})
				testhelper.SeedRefreshToken(t, testDB, testhelper.SeedRefreshTokenParams{Jti: live, UserID: userID})
				testhelper.SeedRefreshToken(t, testDB, testhelper.SeedRefreshTokenParams{Jti: revoked, UserID: userID, RevokedAt: &hourAgo})

				res := w.RunOnce(context.Background())
				require.Equal(t, int64(2), res.RefreshTokens)
				const q = `SELECT 1 FROM refresh_tokens WHERE jti = $1`
				require.False(t, exists(t, q, expired))
				require.False(t, exists(t, q, capped))
				require.True(t, exists(t, q, live))
				require.True(t, exists(t, q, revoked), "reuse detection still needs a spent token's row")
				require.Equal(t, int64(2), res.AuthSessions, "the two sessions left without a token go too")
			}},
		{name: "sessions: one whose family has no token left is deleted, one with a token kept",
			retention: 0, batchSize: 10_000, run: func(t *testing.T, w *Worker, userID int64) {
				orphan := testhelper.SeedSession(t, testDB, testhelper.SeedSessionParams{UserID: userID})
				kept := testhelper.SeedRefreshToken(t, testDB, testhelper.SeedRefreshTokenParams{Jti: uuid.New(), UserID: userID})

				res := w.RunOnce(context.Background())
				require.Equal(t, int64(1), res.AuthSessions)
				const q = `SELECT 1 FROM auth_sessions WHERE family_id = $1`
				require.False(t, exists(t, q, orphan))
				require.True(t, exists(t, q, kept))
			}},
		{name: "one-time tokens: expired ones deleted (used or not), unexpired kept",
			retention: 0, batchSize: 10_000, run: func(t *testing.T, w *Worker, userID int64) {
				old := testhelper.SeedAuthToken(t, testDB, testhelper.SeedAuthTokenParams{UserID: userID, Plain: "a", ExpiresAt: hourAgo})
				usedOld := testhelper.SeedAuthToken(t, testDB, testhelper.SeedAuthTokenParams{UserID: userID, Plain: "b", ExpiresAt: hourAgo, Used: true})
				fresh := testhelper.SeedAuthToken(t, testDB, testhelper.SeedAuthTokenParams{UserID: userID, Plain: "c", ExpiresAt: inAnHour})

				res := w.RunOnce(context.Background())
				require.Equal(t, int64(2), res.AuthTokens)
				const q = `SELECT 1 FROM auth_tokens WHERE id = $1`
				require.False(t, exists(t, q, old))
				require.False(t, exists(t, q, usedOld))
				require.True(t, exists(t, q, fresh))
			}},
		{name: "activity rows older than the retention deleted, newer kept",
			retention: 24 * time.Hour, batchSize: 10_000, run: func(t *testing.T, w *Worker, _ int64) {
				seedActivity(t, time.Now().Add(-48*time.Hour), 3)
				seedActivity(t, time.Now().Add(-time.Hour), 2)

				res := w.RunOnce(context.Background())
				require.Equal(t, int64(3), res.ActivityLogs)
				require.Equal(t, 2, countRows(t, "api_activity_logs"))
			}},
		{name: "a retention of 0 keeps every activity row",
			retention: 0, batchSize: 10_000, run: func(t *testing.T, w *Worker, _ int64) {
				seedActivity(t, time.Now().Add(-100000*time.Hour), 3)

				res := w.RunOnce(context.Background())
				require.Zero(t, res.ActivityLogs)
				require.Equal(t, 3, countRows(t, "api_activity_logs"))
			}},
		{name: "more than one batch is deleted fully",
			retention: time.Hour, batchSize: 2, run: func(t *testing.T, w *Worker, _ int64) {
				seedActivity(t, time.Now().Add(-48*time.Hour), 5)

				res := w.RunOnce(context.Background())
				require.Equal(t, int64(5), res.ActivityLogs)
				require.Zero(t, countRows(t, "api_activity_logs"))
			}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer testhelper.Truncate(t, testDB)
			userID := testhelper.SeedUser(t, testDB, testhelper.SeedUserParams{})
			w := NewWorker(sqlc.New(testDB), tc.retention)
			w.batchSize = tc.batchSize
			tc.run(t, w, userID)
		})
	}
}

// seedActivity inserts n activity rows at occurredAt.
func seedActivity(t *testing.T, occurredAt time.Time, n int) {
	t.Helper()
	for range n {
		_, err := testDB.Exec(`INSERT INTO api_activity_logs (occurred_at, method, path, status, latency_ms, response_bytes)
			VALUES ($1, 'GET', '/api/x', 200, 1, 0)`, occurredAt)
		require.NoError(t, err)
	}
}
