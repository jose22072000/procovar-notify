-- name: GetAdminUserByEmail :one
-- Usada por el login del panel (Fase 2).
SELECT * FROM admin_users WHERE email = $1;

-- name: GetAdminUser :one
SELECT * FROM admin_users WHERE id = $1;

-- name: TouchAdminLastLogin :exec
UPDATE admin_users SET last_login_at = now() WHERE id = $1;

-- name: CreateAdminUser :one
INSERT INTO admin_users (email, password_hash, role, application_id)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ListAdminUsers :many
SELECT * FROM admin_users ORDER BY created_at DESC;

-- name: SetAdminUserStatus :one
UPDATE admin_users SET status = $2 WHERE id = $1
RETURNING *;

-- name: IncrementAdminTokenVersion :one
-- Invalida todos los refresh tokens del admin (logout / revocación).
UPDATE admin_users SET token_version = token_version + 1 WHERE id = $1
RETURNING token_version;
