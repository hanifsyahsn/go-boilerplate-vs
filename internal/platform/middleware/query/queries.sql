-- Queries for the auth middleware.

-- name: GetUserIsActive :one
-- Whether the token's user may still use the API. Read on every authenticated
-- request, so a deactivated user is locked out at once instead of when their
-- token expires. A missing row (deleted user) comes back as sql.ErrNoRows.
SELECT is_active
FROM users
WHERE id = $1;
