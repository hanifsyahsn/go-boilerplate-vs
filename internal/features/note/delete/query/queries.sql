-- name: SoftDeleteNote :exec
-- Marks a note deleted; the row is kept. Run after the shared
-- LockNoteForUpdate, in the same transaction.
UPDATE notes
SET deleted_at = now(),
    deleted_by = $2
WHERE id = $1;
