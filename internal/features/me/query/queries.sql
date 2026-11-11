-- name: GetMe :one
-- The caller's own profile (GET /api/me), read live from the DB.
SELECT id, name, email, role, created_at, (email_verified_at IS NOT NULL)::boolean AS email_verified
FROM users
WHERE id = $1;
