package testhelper

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// Rotation is a refresh caught mid-flight, the way auth/refresh runs it: in
// one open transaction it has locked the session row, then the token row,
// inserted the successor and spent the old token. Nothing is visible to
// others until Commit. Tests use it to start a revoke while a refresh holds
// its locks.
type Rotation struct {
	NewJTI uuid.UUID
	tx     *sql.Tx
}

// BeginRotation starts a Rotation of oldJTI (a live token of familyID).
func BeginRotation(t *testing.T, db *sql.DB, familyID, oldJTI uuid.UUID, userID int64) *Rotation {
	t.Helper()
	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() }) // a no-op after Commit

	r := &Rotation{NewJTI: uuid.New(), tx: tx}
	_, err = tx.Exec(`SELECT 1 FROM auth_sessions WHERE family_id = $1 FOR UPDATE`, familyID)
	require.NoError(t, err)
	_, err = tx.Exec(`SELECT 1 FROM refresh_tokens WHERE jti = $1 FOR UPDATE`, oldJTI)
	require.NoError(t, err)
	_, err = tx.Exec(`INSERT INTO refresh_tokens (jti, user_id, family_id, expires_at)
		VALUES ($1, $2, $3, now() + interval '1 hour')`, r.NewJTI, userID, familyID)
	require.NoError(t, err)
	_, err = tx.Exec(`UPDATE refresh_tokens SET revoked_at = now(), replaced_by = $2 WHERE jti = $1`, oldJTI, r.NewJTI)
	require.NoError(t, err)
	return r
}

// Commit ends the Rotation: the successor becomes visible.
func (r *Rotation) Commit(t *testing.T) {
	t.Helper()
	require.NoError(t, r.tx.Commit())
}

// LockSessionRow opens a transaction holding the session row of familyID
// (FOR UPDATE), the way every revoke starts. The caller finishes it (revoke
// in it, then commit); it is rolled back at cleanup otherwise.
func LockSessionRow(t *testing.T, db *sql.DB, familyID uuid.UUID) *sql.Tx {
	t.Helper()
	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	_, err = tx.Exec(`SELECT 1 FROM auth_sessions WHERE family_id = $1 FOR UPDATE`, familyID)
	require.NoError(t, err)
	return tx
}

// Async runs fn in a goroutine and delivers its error.
func Async(fn func() error) <-chan error {
	done := make(chan error, 1)
	go func() { done <- fn() }()
	return done
}

// RequireBlocked asserts that done delivers nothing for d: the call behind it
// is waiting on a lock.
func RequireBlocked(t *testing.T, done <-chan error, d time.Duration) {
	t.Helper()
	select {
	case err := <-done:
		require.FailNow(t, "the call returned instead of waiting for the lock", "err: %v", err)
	case <-time.After(d):
	}
}

// Await returns what done delivers, failing the test after 10 s (a deadlock
// Postgres didn't break, or a lock never released).
func Await(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(10 * time.Second):
		require.FailNow(t, "the call never returned")
		return nil
	}
}
