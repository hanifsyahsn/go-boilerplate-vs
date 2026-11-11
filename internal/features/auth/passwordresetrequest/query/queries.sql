-- name: GetUserForPasswordReset :one
-- The account a reset link would go to, matched on email ignoring case.
SELECT id, email, is_active
FROM users
WHERE lower(email) = lower(sqlc.arg(email)::varchar);
