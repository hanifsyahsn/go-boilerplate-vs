-- name: ListNotes :many
-- The caller's live notes. q matches title or body (substring,
-- case-insensitive). sort is one of the list's ?sort values (a field, "-"
-- prefixed when descending), validated by the handler; id breaks ties.
SELECT n.id, n.title, n.body, n.created_by, n.created_at, n.updated_at,
       u.name  AS creator_name,
       u.email AS creator_email
FROM notes n
JOIN users u ON u.id = n.created_by
WHERE n.created_by = sqlc.arg('user_id')
  AND n.deleted_at IS NULL
  AND (sqlc.narg('q')::varchar IS NULL
    OR n.title ILIKE '%' || sqlc.narg('q')::varchar || '%'
    OR n.body ILIKE '%' || sqlc.narg('q')::varchar || '%')
ORDER BY CASE WHEN sqlc.arg('sort')::text = 'title' THEN n.title END,
         CASE WHEN sqlc.arg('sort')::text = '-title' THEN n.title END DESC,
         CASE WHEN sqlc.arg('sort')::text = 'created_at' THEN n.created_at END,
         CASE WHEN sqlc.arg('sort')::text = '-created_at' THEN n.created_at END DESC,
         CASE WHEN sqlc.arg('sort')::text = 'updated_at' THEN n.updated_at END,
         CASE WHEN sqlc.arg('sort')::text = '-updated_at' THEN n.updated_at END DESC,
         CASE WHEN left(sqlc.arg('sort')::text, 1) = '-' THEN n.id END DESC,
         n.id
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CountNotes :one
-- Same filters as ListNotes.
SELECT count(*)
FROM notes n
WHERE n.created_by = sqlc.arg('user_id')
  AND n.deleted_at IS NULL
  AND (sqlc.narg('q')::varchar IS NULL
    OR n.title ILIKE '%' || sqlc.narg('q')::varchar || '%'
    OR n.body ILIKE '%' || sqlc.narg('q')::varchar || '%');
