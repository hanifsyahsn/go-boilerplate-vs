-- Queries used by more than one note slice. Rule: a query two slices need
-- lives here, never copied into both.

-- name: GetNoteByID :one
-- A live (not soft-deleted) note with its creator. A missing or deleted note
-- comes back as sql.ErrNoRows. Used by create (read-back), detail and update
-- (read-back).
SELECT n.id, n.title, n.body, n.created_by, n.created_at, n.updated_at,
       u.name  AS creator_name,
       u.email AS creator_email
FROM notes n
JOIN users u ON u.id = n.created_by
WHERE n.id = $1
  AND n.deleted_at IS NULL;

-- name: LockNoteForUpdate :one
-- Locks a live note's row for the rest of the transaction, so a concurrent
-- update / delete waits instead of interleaving. Used by update and delete.
SELECT id, created_by
FROM notes
WHERE id = $1
  AND deleted_at IS NULL
FOR UPDATE;
