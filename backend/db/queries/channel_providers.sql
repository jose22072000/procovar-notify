-- name: ListChannelProviders :many
SELECT * FROM channel_providers
WHERE application_id = $1 ORDER BY name;

-- name: GetChannelProvider :one
SELECT * FROM channel_providers
WHERE id = $1 AND application_id = $2;

-- name: CreateChannelProvider :one
INSERT INTO channel_providers (application_id, channel, provider, name, config_enc)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: UpdateChannelProvider :one
UPDATE channel_providers SET
    channel = $3, provider = $4, name = $5, config_enc = $6, status = $7
WHERE id = $1 AND application_id = $2
RETURNING *;

-- name: DeleteChannelProvider :exec
DELETE FROM channel_providers WHERE id = $1 AND application_id = $2;
