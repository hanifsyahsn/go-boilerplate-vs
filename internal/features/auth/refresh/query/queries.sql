-- name: GetUserForRefresh :one
-- The refresh token's user, read live: a fresh access token carries the
-- current role, and a deactivated user gets none.
SELECT id, name, email, role, is_active
FROM users
WHERE id = $1;

-- name: LockSessionOfToken :exec
-- Locks the presented token's session row first (the lock order every path
-- follows: session, then tokens — see authshared.EndSession), so a refresh
-- waits for a revoke in flight instead of deadlocking with it. A token with no
-- session locks nothing; LockRefreshToken then reports it not live.
SELECT s.family_id
FROM auth_sessions s
         JOIN refresh_tokens rt ON rt.family_id = s.family_id
WHERE rt.jti = $1
FOR UPDATE OF s;

-- name: LockRefreshToken :one
-- The presented token's row, locked so two concurrent refreshes of the same
-- token can't both rotate it, with its session's state (session_live false =
-- signed out, or a family without a session). Run after LockSessionOfToken.
SELECT rt.jti,
       rt.user_id,
       rt.family_id,
       rt.revoked_at,
       rt.replaced_by,
       rt.session_expires_at,
       (s.family_id IS NOT NULL AND s.revoked_at IS NULL)::boolean AS session_live
FROM refresh_tokens rt
         LEFT JOIN auth_sessions s ON s.family_id = rt.family_id
WHERE rt.jti = $1
FOR UPDATE OF rt;

-- name: RotateRefreshToken :exec
-- Revokes the presented token and points it at its successor.
UPDATE refresh_tokens
SET revoked_at  = now(),
    replaced_by = sqlc.arg(replaced_by)::uuid
WHERE jti = sqlc.arg(jti)::uuid;

-- name: TouchSession :exec
-- Records the session's latest use: when, from which browser and IP.
UPDATE auth_sessions
SET last_used_at = now(),
    user_agent   = sqlc.arg(user_agent)::varchar,
    client_ip    = sqlc.arg(client_ip)::varchar
WHERE family_id = sqlc.arg(family_id)::uuid;
