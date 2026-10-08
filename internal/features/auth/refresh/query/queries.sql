-- name: GetUserForRefresh :one
-- The refresh token's user, read live: a fresh access token carries the
-- current role, and a deactivated user gets none.
SELECT id, name, email, role, is_active
FROM users
WHERE id = $1;

-- name: LockRefreshToken :one
-- The presented token's row, locked so two concurrent refreshes of the same
-- token can't both rotate it.
SELECT jti, user_id, family_id, revoked_at, replaced_by
FROM refresh_tokens
WHERE jti = $1
FOR UPDATE;

-- name: RotateRefreshToken :exec
-- Revokes the presented token and points it at its successor.
UPDATE refresh_tokens
SET revoked_at  = now(),
    replaced_by = sqlc.arg(replaced_by)::uuid
WHERE jti = sqlc.arg(jti)::uuid;
