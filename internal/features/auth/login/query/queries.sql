-- name: GetUserForLogin :one
-- The account behind a login attempt, matched on email ignoring case.
SELECT id, name, email, password_hash, role, is_active
FROM users
WHERE lower(email) = lower(sqlc.arg(email)::varchar);

-- name: RehashPassword :execrows
-- Replaces a hash made before passwords were NFC-normalized (decision 0019)
-- with the NFC one, only if it is still the hash the login checked: a password
-- changed in between is never overwritten (0 rows).
UPDATE users
SET password_hash = sqlc.arg(new_hash)
WHERE id = sqlc.arg(id)
  AND password_hash = sqlc.arg(old_hash);
