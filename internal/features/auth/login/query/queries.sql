-- name: GetUserForLogin :one
-- The account behind a login attempt, matched on email ignoring case.
SELECT id, name, email, password_hash, role, is_active
FROM users
WHERE lower(email) = lower(sqlc.arg(email)::varchar);
