-- name: GetUserPassword :one
-- The caller's password hash, to check the current password.
SELECT password_hash
FROM users
WHERE id = $1;

-- name: SetPassword :one
-- The caller's new password. Returns the address for the notice.
UPDATE users
SET password_hash = sqlc.arg(password_hash)::varchar
WHERE id = sqlc.arg(id)::bigint
RETURNING email;
