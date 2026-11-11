-- name: GetUserForVerification :one
-- The caller's address and whether it is verified already.
SELECT email, (email_verified_at IS NOT NULL)::boolean AS email_verified
FROM users
WHERE id = $1;
