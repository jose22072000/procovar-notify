-- name: CreateApplication :one
INSERT INTO applications (name, slug)
VALUES ($1, $2)
RETURNING *;

-- name: GetApplication :one
SELECT * FROM applications WHERE id = $1;

-- name: GetApplicationBySlug :one
SELECT * FROM applications WHERE slug = $1;

-- name: ListApplications :many
SELECT * FROM applications ORDER BY created_at DESC;

-- name: SetApplicationStatus :one
UPDATE applications SET status = $2 WHERE id = $1
RETURNING *;

-- name: UpdateApplication :one
UPDATE applications SET name = $2 WHERE id = $1
RETURNING *;

-- name: UpdateApplicationSlug :one
-- El slug es UNIQUE. Cambiarlo es seguro para los clientes HMAC (autentican por
-- key_id -> application_id, nunca por slug). Un duplicado viola la constraint y
-- el handler lo traduce a 409.
UPDATE applications SET slug = $2 WHERE id = $1
RETURNING *;

-- name: SetApplicationRetention :one
UPDATE applications SET pii_retention_days = $2 WHERE id = $1
RETURNING *;

-- name: SetApplicationQuota :one
UPDATE applications SET daily_send_quota = $2, monthly_send_quota = $3 WHERE id = $1
RETURNING *;
