-- name: CreateNote :one
INSERT INTO notes (title, body, created_by)
VALUES ($1, $2, $3)
RETURNING id;
