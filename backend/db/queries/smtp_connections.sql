-- name: GetSmtpConnection :one
SELECT * FROM smtp_connections
WHERE id = $1 AND application_id = $2;

-- name: ListSmtpConnections :many
SELECT * FROM smtp_connections
WHERE application_id = $1 ORDER BY name;

-- name: CreateSmtpConnection :one
INSERT INTO smtp_connections
    (application_id, name, host, port, username, password_enc, from_email, from_name, secure)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: UpdateSmtpConnection :one
UPDATE smtp_connections SET
    name = $3, host = $4, port = $5, username = $6, password_enc = $7,
    from_email = $8, from_name = $9, secure = $10, status = $11
WHERE id = $1 AND application_id = $2
RETURNING *;

-- name: DeleteSmtpConnection :exec
DELETE FROM smtp_connections WHERE id = $1 AND application_id = $2;
