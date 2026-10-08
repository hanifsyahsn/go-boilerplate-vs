-- name: ListUsers :many
-- The user directory, active or not. q matches name or email (substring,
-- case-insensitive); is_active narrows when given. sort is one of the list's
-- ?sort values (a field, "-" prefixed when descending), validated by the
-- handler; id breaks ties.
SELECT id, name, email, role, is_active, created_at, updated_at
FROM users
WHERE (sqlc.narg('q')::varchar IS NULL
    OR name ILIKE '%' || sqlc.narg('q')::varchar || '%'
    OR email ILIKE '%' || sqlc.narg('q')::varchar || '%')
  AND (sqlc.narg('is_active')::boolean IS NULL OR is_active = sqlc.narg('is_active')::boolean)
ORDER BY CASE WHEN sqlc.arg('sort')::text = 'name' THEN name END,
         CASE WHEN sqlc.arg('sort')::text = '-name' THEN name END DESC,
         CASE WHEN sqlc.arg('sort')::text = 'email' THEN email END,
         CASE WHEN sqlc.arg('sort')::text = '-email' THEN email END DESC,
         CASE WHEN sqlc.arg('sort')::text = 'created_at' THEN created_at END,
         CASE WHEN sqlc.arg('sort')::text = '-created_at' THEN created_at END DESC,
         CASE WHEN left(sqlc.arg('sort')::text, 1) = '-' THEN id END DESC,
         id
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CountUsers :one
-- Same filters as ListUsers.
SELECT count(*)
FROM users
WHERE (sqlc.narg('q')::varchar IS NULL
    OR name ILIKE '%' || sqlc.narg('q')::varchar || '%'
    OR email ILIKE '%' || sqlc.narg('q')::varchar || '%')
  AND (sqlc.narg('is_active')::boolean IS NULL OR is_active = sqlc.narg('is_active')::boolean);

-- name: GetUser :one
-- One user, for a PATCH that changes nothing (JSON:API: the response is the
-- resource as it stands). A missing user comes back as sql.ErrNoRows.
SELECT id, name, email, role, is_active, created_at, updated_at
FROM users
WHERE id = $1;

-- name: SetUserActive :one
-- Activates / deactivates a user. A missing user comes back as sql.ErrNoRows.
-- A deactivated user is locked out on their next request (the auth
-- middleware re-reads is_active).
UPDATE users
SET is_active = $2
WHERE id = $1
RETURNING id, name, email, role, is_active, created_at, updated_at;
