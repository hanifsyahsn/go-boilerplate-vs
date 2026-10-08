-- name: InsertRefreshToken :exec
-- Records an issued refresh token (login starts a family, refresh continues
-- it).
INSERT INTO refresh_tokens (jti, user_id, family_id, expires_at)
VALUES ($1, $2, $3, $4);

-- name: RevokeRefreshTokenFamily :exec
-- Revokes every live token of the family the given token belongs to (logout,
-- or a rotated token presented again).
UPDATE refresh_tokens
SET revoked_at = now()
WHERE family_id = (SELECT rt.family_id FROM refresh_tokens rt WHERE rt.jti = $1)
  AND revoked_at IS NULL;
