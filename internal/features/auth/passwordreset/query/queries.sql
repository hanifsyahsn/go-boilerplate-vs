-- name: SetPasswordAfterReset :one
-- The new password after a reset link was used; the link proved the address,
-- so the email counts as verified too. Returns the address for the notice.
UPDATE users
SET password_hash     = sqlc.arg(password_hash)::varchar,
    email_verified_at = COALESCE(email_verified_at, now())
WHERE id = sqlc.arg(id)::bigint
RETURNING email;
