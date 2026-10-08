-- name: GetMe :one
-- The caller's own profile (GET /api/me), read live from the DB.
SELECT id, name, email, role, created_at
FROM users
WHERE id = $1;
