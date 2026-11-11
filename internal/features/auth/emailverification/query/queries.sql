-- name: MarkEmailVerified :exec
-- The account's email is verified (a link was opened); the first time is kept.
UPDATE users
SET email_verified_at = COALESCE(email_verified_at, now())
WHERE id = $1;
