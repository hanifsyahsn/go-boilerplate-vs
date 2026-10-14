package testhelper

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

// DefaultPassword is the plain password SeedUser hashes when none is given.
const DefaultPassword = "Passw0rd!"

type SeedUserParams struct {
	Name     string // "" => "Test User"
	Email    string // "" => a unique test-<n>@example.com
	Password string // "" => DefaultPassword
	Role     string // "" => "user"
	IsActive *bool  // nil => true
}

// SeedUser inserts a user and returns its id. The password is hashed at
// bcrypt's minimum cost, so seeding stays fast.
func SeedUser(t *testing.T, db *sql.DB, p SeedUserParams) int64 {
	t.Helper()

	if p.Name == "" {
		p.Name = "Test User"
	}
	if p.Email == "" {
		p.Email = fmt.Sprintf("test-%d@example.com", time.Now().UnixNano())
	}
	if p.Password == "" {
		p.Password = DefaultPassword
	}
	if p.Role == "" {
		p.Role = "user"
	}
	isActive := true
	if p.IsActive != nil {
		isActive = *p.IsActive
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(p.Password), bcrypt.MinCost)
	require.NoError(t, err)

	var id int64
	err = db.QueryRow(`
		INSERT INTO users (name, email, password_hash, role, is_active)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`, p.Name, p.Email, string(hash), p.Role, isActive).Scan(&id)
	require.NoError(t, err, "seed user")

	return id
}

type SeedNoteParams struct {
	Title     string // "" => "Test note"
	Body      string
	CreatedBy int64 // required
	Deleted   bool  // true => soft-deleted by its creator
}

// SeedNote inserts a note and returns its id.
func SeedNote(t *testing.T, db *sql.DB, p SeedNoteParams) int64 {
	t.Helper()

	if p.Title == "" {
		p.Title = "Test note"
	}

	var id int64
	err := db.QueryRow(`
		INSERT INTO notes (title, body, created_by, deleted_at, deleted_by)
		VALUES ($1, $2, $3,
		        CASE WHEN $4::boolean THEN now() END,
		        CASE WHEN $4::boolean THEN $3::bigint END)
		RETURNING id
	`, p.Title, p.Body, p.CreatedBy, p.Deleted).Scan(&id)
	require.NoError(t, err, "seed note")

	return id
}

type SeedRefreshTokenParams struct {
	Jti       uuid.UUID // required (the token's jti)
	UserID    int64     // required
	FamilyID  uuid.UUID // uuid.Nil => a new family
	ExpiresAt time.Time // zero => in a week
	// RevokedAt, when set, marks the token revoked at that time, replaced by
	// ReplacedBy (rotated) or not (logged out / family revoked).
	RevokedAt  *time.Time
	ReplacedBy uuid.NullUUID
	// SessionExpiresAt caps the login (untrusted device); nil => no cap.
	SessionExpiresAt *time.Time
}

// SeedRefreshToken inserts a refresh_tokens row and returns its family id.
func SeedRefreshToken(t *testing.T, db *sql.DB, p SeedRefreshTokenParams) uuid.UUID {
	t.Helper()

	if p.FamilyID == uuid.Nil {
		p.FamilyID = uuid.New()
	}
	if p.ExpiresAt.IsZero() {
		p.ExpiresAt = time.Now().Add(7 * 24 * time.Hour)
	}

	_, err := db.Exec(`
		INSERT INTO refresh_tokens (jti, user_id, family_id, expires_at, revoked_at, replaced_by, session_expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, p.Jti, p.UserID, p.FamilyID, p.ExpiresAt, p.RevokedAt, p.ReplacedBy, p.SessionExpiresAt)
	require.NoError(t, err, "seed refresh token")

	return p.FamilyID
}

// Truncate empties every table between test cases.
func Truncate(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.Exec(`TRUNCATE TABLE users, notes, refresh_tokens RESTART IDENTITY CASCADE`)
	require.NoError(t, err)
}

// BoolPtr returns a pointer to b (for SeedUserParams.IsActive).
func BoolPtr(b bool) *bool { return &b }
