-- name: ListLiveSessions :many
-- The user's live sessions: not revoked, and the family still has a refresh
-- token that can be used (not revoked, not expired, inside the session's
-- cap). session_expires_at is the cap (NULL = trusted, none). sort is one of
-- the list's ?sort values, validated by the handler; family_id breaks ties.
SELECT s.family_id,
       s.user_agent,
       s.client_ip,
       s.trusted,
       s.created_at,
       s.last_used_at,
       t.session_expires_at
FROM auth_sessions s
         JOIN LATERAL (SELECT rt.session_expires_at
                       FROM refresh_tokens rt
                       WHERE rt.family_id = s.family_id
                         AND rt.revoked_at IS NULL
                         AND rt.expires_at > now()
                         AND (rt.session_expires_at IS NULL OR rt.session_expires_at > now())
                       LIMIT 1) t ON true
WHERE s.user_id = sqlc.arg(user_id)::bigint
  AND s.revoked_at IS NULL
ORDER BY CASE WHEN sqlc.arg(sort)::text = 'last_used_at' THEN s.last_used_at END,
         CASE WHEN sqlc.arg(sort)::text = '-last_used_at' THEN s.last_used_at END DESC,
         CASE WHEN sqlc.arg(sort)::text = 'created_at' THEN s.created_at END,
         CASE WHEN sqlc.arg(sort)::text = '-created_at' THEN s.created_at END DESC,
         CASE WHEN left(sqlc.arg(sort)::text, 1) = '-' THEN s.family_id END DESC,
         s.family_id;
