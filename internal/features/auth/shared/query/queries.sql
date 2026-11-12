-- name: StartSession :exec
-- Starts a login: its auth_sessions row and the family's first refresh token,
-- in one statement (both or neither). session_expires_at is NULL for a
-- trusted device (no cap).
WITH session AS (
    INSERT INTO auth_sessions (family_id, user_id, trusted, user_agent, client_ip)
    VALUES (sqlc.arg(family_id)::uuid, sqlc.arg(user_id)::bigint, sqlc.arg(trusted)::boolean,
            sqlc.arg(user_agent)::varchar, sqlc.arg(client_ip)::varchar)
)
INSERT INTO refresh_tokens (jti, user_id, family_id, expires_at, session_expires_at)
VALUES (sqlc.arg(jti)::uuid, sqlc.arg(user_id)::bigint, sqlc.arg(family_id)::uuid, sqlc.arg(expires_at)::timestamptz,
        sqlc.narg(session_expires_at)::timestamptz);

-- name: InsertRefreshToken :exec
-- Records a refresh token of an existing family (refresh continues it).
-- session_expires_at is NULL for a trusted device (no cap).
INSERT INTO refresh_tokens (jti, user_id, family_id, expires_at, session_expires_at)
VALUES ($1, $2, $3, $4, $5);

-- name: FoldEmail :one
-- An email folded to lowercase by Postgres, the way the lookups match it
-- (lower(email) = lower($1)): a per-email rate-limit key built from this and
-- the lookup always agree. Go's strings.ToLower doesn't: its Unicode tables
-- can differ from the database's (U+A7DC lowers to U+019B in glibc's, not in
-- Go's), which would split one account's count across two keys.
SELECT lower(sqlc.arg(email)::varchar)::varchar AS folded;

-- name: LockSession :exec
-- Locks a session row before its tokens are revoked (EndSession). Every path
-- that touches a session and its tokens locks the session row first, so a
-- refresh and a revoke never wait on each other in opposite orders.
SELECT family_id
FROM auth_sessions
WHERE family_id = $1
FOR UPDATE;

-- name: LockUserSessions :exec
-- Locks a user's live session rows, except keep_family_id (NULL = all), in
-- family_id order (two revokers never cross), before EndUserSessions revokes
-- them.
SELECT family_id
FROM auth_sessions
WHERE user_id = sqlc.arg(user_id)::bigint
  AND revoked_at IS NULL
  AND (sqlc.narg(keep_family_id)::uuid IS NULL OR family_id <> sqlc.narg(keep_family_id)::uuid)
ORDER BY family_id
FOR UPDATE;

-- name: RevokeSession :execrows
-- Ends one session: revokes its family's live refresh tokens and marks the
-- session revoked with the reason, in one statement. Run it after the session
-- row is locked (EndSession, or a slice's own lock), never alone. The row count is the
-- sessions revoked (0 = it was already revoked or doesn't exist).
WITH tokens AS (
    UPDATE refresh_tokens
    SET revoked_at = now()
    WHERE family_id = sqlc.arg(family_id)::uuid
      AND revoked_at IS NULL
)
UPDATE auth_sessions
SET revoked_at    = now(),
    revoke_reason = sqlc.arg(reason)::varchar
WHERE family_id = sqlc.arg(family_id)::uuid
  AND revoked_at IS NULL;

-- name: RevokeUserSessions :execrows
-- Ends every live session of a user except keep_family_id (NULL = all): a
-- password change keeps the caller's own, a reset or a deactivation none.
-- Run it after LockUserSessions (EndUserSessions), never alone.
-- The row count is the sessions revoked.
WITH tokens AS (
    UPDATE refresh_tokens
    SET revoked_at = now()
    WHERE user_id = sqlc.arg(user_id)::bigint
      AND revoked_at IS NULL
      AND (sqlc.narg(keep_family_id)::uuid IS NULL OR family_id <> sqlc.narg(keep_family_id)::uuid)
)
UPDATE auth_sessions
SET revoked_at    = now(),
    revoke_reason = sqlc.arg(reason)::varchar
WHERE user_id = sqlc.arg(user_id)::bigint
  AND revoked_at IS NULL
  AND (sqlc.narg(keep_family_id)::uuid IS NULL OR family_id <> sqlc.narg(keep_family_id)::uuid);

-- name: SpendUnusedAuthTokens :exec
-- Spends the user's unused tokens of a purpose before a new one is issued:
-- only the latest link works.
UPDATE auth_tokens
SET used_at = now()
WHERE user_id = $1
  AND purpose = $2
  AND used_at IS NULL;

-- name: InsertAuthToken :exec
-- Records an issued one-time token (its hash only).
INSERT INTO auth_tokens (purpose, user_id, token_hash, expires_at)
VALUES ($1, $2, $3, $4);

-- name: ConsumeAuthToken :one
-- Atomic single use: two parallel requests with one token → one row, the other gets no rows.
UPDATE auth_tokens
SET used_at = now()
WHERE token_hash = $1
  AND purpose = $2
  AND used_at IS NULL
  AND expires_at > now()
RETURNING user_id;
