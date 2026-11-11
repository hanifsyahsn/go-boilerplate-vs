-- Queries for the auth middleware.

-- name: GetAccessStatus :one
-- Whether the token's user may still use the API and its session is still
-- live. Read on every authenticated request, so a deactivated user or a
-- signed-out session is locked out at once instead of when the access token
-- expires. A missing user row (deleted) comes back as sql.ErrNoRows.
SELECT u.is_active,
       EXISTS (SELECT 1
               FROM auth_sessions s
               WHERE s.family_id = sqlc.arg(session_id)::uuid
                 AND s.user_id = u.id
                 AND s.revoked_at IS NULL) AS session_live
FROM users u
WHERE u.id = sqlc.arg(user_id)::bigint;
