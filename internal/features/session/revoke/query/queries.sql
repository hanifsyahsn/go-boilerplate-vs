-- name: LockLiveSession :one
-- The caller's session, locked, when it is live (as ListLiveSessions counts
-- it). Someone else's, a revoked one or one with no usable refresh token come
-- back as sql.ErrNoRows.
SELECT s.family_id
FROM auth_sessions s
WHERE s.family_id = sqlc.arg(family_id)::uuid
  AND s.user_id = sqlc.arg(user_id)::bigint
  AND s.revoked_at IS NULL
  AND EXISTS (SELECT 1
              FROM refresh_tokens rt
              WHERE rt.family_id = s.family_id
                AND rt.revoked_at IS NULL
                AND rt.expires_at > now()
                AND (rt.session_expires_at IS NULL OR rt.session_expires_at > now()))
FOR UPDATE OF s;
