-- name: ListWebhookEndpoints :many
SELECT * FROM webhook_endpoints
WHERE application_id = $1 ORDER BY created_at DESC;

-- name: ListActiveWebhookEndpoints :many
SELECT * FROM webhook_endpoints
WHERE application_id = $1 AND status = 'ACTIVE';

-- name: GetWebhookEndpoint :one
SELECT * FROM webhook_endpoints
WHERE id = $1 AND application_id = $2;

-- name: CreateWebhookEndpoint :one
INSERT INTO webhook_endpoints (application_id, url, secret_enc, events)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: DeleteWebhookEndpoint :exec
DELETE FROM webhook_endpoints WHERE id = $1 AND application_id = $2;
