package testhelper

import (
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// SeededNote is one notes row, as read back by tests.
type SeededNote struct {
	ID        int64
	Title     string
	Body      string
	CreatedBy int64
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt sql.NullTime
	DeletedBy sql.NullInt64
}

// GetNote reads a note row, deleted or not.
func GetNote(t *testing.T, db *sql.DB, id int64) SeededNote {
	t.Helper()

	var n SeededNote
	err := db.QueryRow(`
		SELECT id, title, body, created_by, created_at, updated_at, deleted_at, deleted_by
		FROM notes WHERE id = $1
	`, id).Scan(&n.ID, &n.Title, &n.Body, &n.CreatedBy, &n.CreatedAt, &n.UpdatedAt, &n.DeletedAt, &n.DeletedBy)
	require.NoError(t, err, "get note %d", id)
	return n
}

// SeededUser is one users row, as read back by tests.
type SeededUser struct {
	ID           int64
	Name         string
	Email        string
	PasswordHash string
	Role         string
	IsActive     bool
	// EmailVerifiedAt is NULL until the email was verified (a link opened).
	EmailVerifiedAt sql.NullTime
}

// GetUser reads a user row.
func GetUser(t *testing.T, db *sql.DB, id int64) SeededUser {
	t.Helper()

	var u SeededUser
	err := db.QueryRow(`
		SELECT id, name, email, password_hash, role, is_active, email_verified_at
		FROM users WHERE id = $1
	`, id).Scan(&u.ID, &u.Name, &u.Email, &u.PasswordHash, &u.Role, &u.IsActive, &u.EmailVerifiedAt)
	require.NoError(t, err, "get user %d", id)
	return u
}

// SeededRefreshToken is one refresh_tokens row, as read back by tests.
type SeededRefreshToken struct {
	Jti        uuid.UUID
	UserID     int64
	FamilyID   uuid.UUID
	ExpiresAt  time.Time
	RevokedAt  sql.NullTime
	ReplacedBy uuid.NullUUID
	// SessionExpiresAt is the login's cap (untrusted device); NULL = none.
	SessionExpiresAt sql.NullTime
}

// GetRefreshToken reads a refresh_tokens row.
func GetRefreshToken(t *testing.T, db *sql.DB, jti uuid.UUID) SeededRefreshToken {
	t.Helper()

	var r SeededRefreshToken
	err := db.QueryRow(`
		SELECT jti, user_id, family_id, expires_at, revoked_at, replaced_by, session_expires_at
		FROM refresh_tokens WHERE jti = $1
	`, jti).Scan(&r.Jti, &r.UserID, &r.FamilyID, &r.ExpiresAt, &r.RevokedAt, &r.ReplacedBy, &r.SessionExpiresAt)
	require.NoError(t, err, "get refresh token %s", jti)
	return r
}

// SeededSession is one auth_sessions row, as read back by tests.
type SeededSession struct {
	FamilyID     uuid.UUID
	UserID       int64
	Trusted      bool
	UserAgent    string
	ClientIP     string
	CreatedAt    time.Time
	LastUsedAt   time.Time
	RevokedAt    sql.NullTime
	RevokeReason sql.NullString
}

// GetSession reads an auth_sessions row.
func GetSession(t *testing.T, db *sql.DB, familyID uuid.UUID) SeededSession {
	t.Helper()

	var s SeededSession
	err := db.QueryRow(`
		SELECT family_id, user_id, trusted, user_agent, client_ip, created_at, last_used_at, revoked_at, revoke_reason
		FROM auth_sessions WHERE family_id = $1
	`, familyID).Scan(&s.FamilyID, &s.UserID, &s.Trusted, &s.UserAgent, &s.ClientIP, &s.CreatedAt, &s.LastUsedAt,
		&s.RevokedAt, &s.RevokeReason)
	require.NoError(t, err, "get session %s", familyID)
	return s
}

// SeededAuthToken is one auth_tokens row, as read back by tests.
type SeededAuthToken struct {
	ID        int64
	Purpose   string
	UserID    int64
	TokenHash string
	ExpiresAt time.Time
	UsedAt    sql.NullTime
}

// GetAuthTokens reads every auth_tokens row of a user, oldest first.
func GetAuthTokens(t *testing.T, db *sql.DB, userID int64) []SeededAuthToken {
	t.Helper()

	rows, err := db.Query(`
		SELECT id, purpose, user_id, token_hash, expires_at, used_at
		FROM auth_tokens WHERE user_id = $1 ORDER BY id
	`, userID)
	require.NoError(t, err, "get auth tokens of %d", userID)
	defer func() { _ = rows.Close() }()

	var out []SeededAuthToken
	for rows.Next() {
		var a SeededAuthToken
		require.NoError(t, rows.Scan(&a.ID, &a.Purpose, &a.UserID, &a.TokenHash, &a.ExpiresAt, &a.UsedAt))
		out = append(out, a)
	}
	require.NoError(t, rows.Err())
	return out
}
