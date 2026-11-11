-- Each delete takes at most batch_size rows, so one run never holds a long
-- lock; the worker repeats a delete until a batch comes back short.

-- name: DeleteExpiredRefreshTokens :execrows
-- Refresh tokens past their own expiry or their session's cap. A revoked
-- token that hasn't expired is kept: reuse detection needs its row (a spent
-- token presented again must still be found).
DELETE FROM refresh_tokens
WHERE jti IN (SELECT rt.jti
              FROM refresh_tokens rt
              WHERE rt.expires_at < now()
                 OR rt.session_expires_at < now()
              LIMIT sqlc.arg(batch_size)::int);

-- name: DeleteOrphanSessions :execrows
-- Sessions whose family has no refresh token left (all expired and deleted).
-- Login inserts a session with its first token in one statement, so a live
-- login is never caught between the two.
DELETE FROM auth_sessions
WHERE family_id IN (SELECT s.family_id
                    FROM auth_sessions s
                    WHERE NOT EXISTS (SELECT 1 FROM refresh_tokens rt WHERE rt.family_id = s.family_id)
                    LIMIT sqlc.arg(batch_size)::int);

-- name: DeleteExpiredAuthTokens :execrows
-- One-time email tokens past their expiry (used or not).
DELETE FROM auth_tokens
WHERE id IN (SELECT t.id
             FROM auth_tokens t
             WHERE t.expires_at < now()
             LIMIT sqlc.arg(batch_size)::int);

-- name: DeleteOldActivityLogs :execrows
-- Activity rows older than the retention.
DELETE FROM api_activity_logs
WHERE id IN (SELECT a.id
             FROM api_activity_logs a
             WHERE a.occurred_at < sqlc.arg(before)::timestamptz
             LIMIT sqlc.arg(batch_size)::int);
