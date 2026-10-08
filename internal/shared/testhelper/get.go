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
}

// GetUser reads a user row.
func GetUser(t *testing.T, db *sql.DB, id int64) SeededUser {
	t.Helper()

	var u SeededUser
	err := db.QueryRow(`
		SELECT id, name, email, password_hash, role, is_active
		FROM users WHERE id = $1
	`, id).Scan(&u.ID, &u.Name, &u.Email, &u.PasswordHash, &u.Role, &u.IsActive)
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
}

// GetRefreshToken reads a refresh_tokens row.
func GetRefreshToken(t *testing.T, db *sql.DB, jti uuid.UUID) SeededRefreshToken {
	t.Helper()

	var r SeededRefreshToken
	err := db.QueryRow(`
		SELECT jti, user_id, family_id, expires_at, revoked_at, replaced_by
		FROM refresh_tokens WHERE jti = $1
	`, jti).Scan(&r.Jti, &r.UserID, &r.FamilyID, &r.ExpiresAt, &r.RevokedAt, &r.ReplacedBy)
	require.NoError(t, err, "get refresh token %s", jti)
	return r
}
