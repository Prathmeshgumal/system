-- name: CreateNote :one
INSERT INTO notes (body) VALUES ($1)
RETURNING id, body, created_at;

-- name: ListLatestNotes :many
SELECT id, body, created_at
FROM notes
ORDER BY created_at DESC, id DESC
LIMIT $1;
