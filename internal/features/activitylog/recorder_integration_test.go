package activitylog

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/features/activitylog/sqlc"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/config"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/platform/middleware"
	"github.com/hanifsyahsn/go_boilerplate_vs/internal/shared/testhelper"
	"github.com/stretchr/testify/require"
)

// TestRecorder_ShutdownFlush_I: rows recorded right before shutdown are all
// in the table once Run returned (the shutdown flush of decision 0008).
func TestRecorder_ShutdownFlush_I(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	testCases := []struct {
		name string
		rows int
	}{
		{name: "500 rows recorded, then cancel: Run returns and all 500 are in the table", rows: 500},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer testhelper.Truncate(t, testDB)
			r := NewRecorder(sqlc.New(testDB))
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			go func() { r.Run(ctx); close(done) }()

			for i := range tc.rows {
				r.Record(middleware.Activity{OccurredAt: time.Now(), Method: "GET", Route: "/api/notes/:id", Path: "/api/notes/1", Status: 200, Latency: time.Duration(i)})
			}
			cancel()
			select {
			case <-done:
			case <-time.After(30 * time.Second):
				t.Fatal("Run didn't return after cancel")
			}

			var n int
			require.NoError(t, testDB.QueryRow(`SELECT count(*) FROM api_activity_logs`).Scan(&n))
			require.Equal(t, tc.rows, n)
		})
	}
}

// TestRecorder_Columns_I: one row lands with every column, NULL for an
// anonymous caller.
func TestRecorder_Columns_I(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	sid := uuid.New()
	at := time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)

	testCases := []struct {
		name      string
		a         middleware.Activity
		wantUser  any
		wantSesID any
	}{
		{name: "a machine call keeps the API client's name",
			a:        middleware.Activity{OccurredAt: at, Method: "POST", Path: "/api/internal/sync", Status: 200, APIClient: "billing"},
			wantUser: nil, wantSesID: nil},
		{name: "a signed-in request keeps user and session",
			a: middleware.Activity{OccurredAt: at, RequestID: "req-1", UserID: 7, SessionID: sid, Method: "PATCH", Route: "/api/notes/:id",
				Path: "/api/notes/1", Status: 200, Latency: 1500 * time.Millisecond, ResponseBytes: 321, ClientIP: "203.0.113.7", UserAgent: "UA"},
			wantUser: int64(7), wantSesID: sid.String()},
		{name: "an anonymous request stores NULL user and session",
			a:        middleware.Activity{OccurredAt: at, Method: "POST", Path: "/api/auth/login", Status: 401},
			wantUser: nil, wantSesID: nil},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer testhelper.Truncate(t, testDB)
			r := NewRecorder(sqlc.New(testDB))
			r.write(context.Background(), []middleware.Activity{tc.a})

			var (
				occurredAt                                        time.Time
				requestID, method, route, path, ip, ua, apiClient string
				userID, sessionID                                 any
				status, latencyMs, responseBytes                  int
			)
			require.NoError(t, testDB.QueryRow(`
				SELECT occurred_at, request_id, user_id, session_id::text, method, route, path, status, latency_ms,
				       response_bytes, client_ip, user_agent, api_client
				FROM api_activity_logs`).Scan(&occurredAt, &requestID, &userID, &sessionID, &method, &route, &path, &status,
				&latencyMs, &responseBytes, &ip, &ua, &apiClient))
			require.True(t, occurredAt.Equal(tc.a.OccurredAt))
			require.Equal(t, tc.a.RequestID, requestID)
			require.Equal(t, tc.wantUser, userID)
			require.Equal(t, tc.wantSesID, sessionID)
			require.Equal(t, tc.a.Method, method)
			require.Equal(t, tc.a.Route, route)
			require.Equal(t, tc.a.Path, path)
			require.Equal(t, tc.a.Status, status)
			require.Equal(t, int(tc.a.Latency.Milliseconds()), latencyMs)
			require.Equal(t, tc.a.ResponseBytes, responseBytes)
			require.Equal(t, tc.a.ClientIP, ip)
			require.Equal(t, tc.a.UserAgent, ua)
			require.Equal(t, tc.a.APIClient, apiClient, "\"\" = not a machine call")
		})
	}
}

// TestMigration_LockTimeout_I: a migration that ALTERs api_activity_logs
// (written on every request) gives up after its lock_timeout instead of
// waiting behind a running write (a cleanup DELETE) while every recorder
// INSERT queues behind it — and the timeout ends with it: migrate runs every
// file of a run on one connection, so a session-level SET would reach the
// later migrations too.
func TestMigration_LockTimeout_I(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	const dir = "../../platform/db/migration/"
	up, down := dir+"000009_activity_log_api_client.up.sql", dir+"000009_activity_log_api_client.down.sql"
	testCases := []struct {
		name string
		run  func(t *testing.T, ctx context.Context, conn *sql.Conn)
	}{
		{name: "the up file gives up on a held table instead of queueing writes behind it", run: func(t *testing.T, ctx context.Context, conn *sql.Conn) {
			requireLockTimeout(t, ctx, conn, up)
		}},
		{name: "the down file gives up on a held table too", run: func(t *testing.T, ctx context.Context, conn *sql.Conn) {
			requireLockTimeout(t, ctx, conn, down)
		}},
		{name: "the timeout ends with the migration: the connection's later statements don't inherit it", run: func(t *testing.T, ctx context.Context, conn *sql.Conn) {
			_, err := conn.ExecContext(ctx, `ALTER TABLE api_activity_logs DROP COLUMN IF EXISTS api_client`) // back to before 000009
			require.NoError(t, err)
			sqlText, err := os.ReadFile(up)
			require.NoError(t, err)
			_, err = conn.ExecContext(ctx, string(sqlText))
			require.NoError(t, err)
			var timeout string
			require.NoError(t, conn.QueryRowContext(ctx, `SHOW lock_timeout`).Scan(&timeout))
			require.Equal(t, "0", timeout)
		}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// a regression must fail here, not hang until go test's own timeout
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			conn, err := testDB.Conn(ctx)
			require.NoError(t, err)
			defer func() { _ = conn.Close() }()
			tc.run(t, ctx, conn)
		})
	}
}

// requireLockTimeout runs a migration file while another transaction holds a
// write lock on api_activity_logs, and wants it to give up with a lock
// timeout.
func requireLockTimeout(t *testing.T, ctx context.Context, conn *sql.Conn, file string) {
	t.Helper()
	sqlText, err := os.ReadFile(file)
	require.NoError(t, err)
	// its own ctx: database/sql rolls a transaction back when its ctx ends,
	// which would free the table for the migration under test
	holder, err := testDB.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	defer func() { _ = holder.Rollback() }()
	_, err = holder.ExecContext(context.Background(), `DELETE FROM api_activity_logs WHERE false`) // a write's table lock
	require.NoError(t, err)

	_, err = conn.ExecContext(ctx, string(sqlText))
	require.ErrorContains(t, err, "lock timeout")
	_, _ = conn.ExecContext(context.Background(), `ROLLBACK`) // leave no aborted transaction on the connection
}

func TestRecorder_APIClientColumn_I(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	testCases := []struct {
		name string
	}{
		{name: "the api_client column holds as many characters as config accepts in a client name"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var size int
			require.NoError(t, testDB.QueryRow(`SELECT character_maximum_length FROM information_schema.columns
				WHERE table_name = 'api_activity_logs' AND column_name = 'api_client'`).Scan(&size))
			require.Equal(t, config.MaxAPIClientNameLength, size)
		})
	}
}
