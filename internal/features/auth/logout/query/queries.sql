-- name: GetRefreshTokenFamily :one
-- The session (family) a refresh token belongs to; sql.ErrNoRows for an
-- unknown jti.
SELECT family_id
FROM refresh_tokens
WHERE jti = $1;
