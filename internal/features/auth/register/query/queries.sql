-- name: CreateUser :one
-- A new account, always with role 'user' (an admin is promoted by hand).
-- A taken email (case-insensitive, users_email_lower_key) fails with pq 23505.
INSERT INTO users (name, email, password_hash)
VALUES ($1, $2, $3)
RETURNING id, name, email, role, is_active, created_at;
