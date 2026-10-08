-- name: UpdateNote :exec
-- A partial update (JSON:API PATCH): a NULL title / body keeps the current
-- value. Run after the shared LockNoteForUpdate, in the same transaction.
UPDATE notes
SET title = COALESCE(sqlc.narg('title')::varchar, title),
    body  = COALESCE(sqlc.narg('body')::text, body)
WHERE id = sqlc.arg('id');
