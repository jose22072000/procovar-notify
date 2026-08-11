-- name: ListChannelRoutes :many
SELECT * FROM channel_routes
WHERE application_id = $1
ORDER BY notification_type, channel;

-- name: ListRoutesForResolution :many
-- Ruta(s) para (app, tipo, canal). Con el modelo 1:1 devuelve como mucho una,
-- pero se mantiene :many para no romper la resolución del procesador.
SELECT * FROM channel_routes
WHERE application_id = $1 AND notification_type = $2 AND channel = $3;

-- name: GetChannelRouteByID :one
-- Resuelve el "Tipo de notificación" por id (dentro de la app).
SELECT * FROM channel_routes
WHERE id = $1 AND application_id = $2;

-- name: GetChannelRouteByName :one
-- Resuelve el "Tipo de notificación" por su nombre único (notification_type).
SELECT * FROM channel_routes
WHERE application_id = $1 AND notification_type = $2;

-- name: CreateChannelRoute :one
INSERT INTO channel_routes
    (application_id, notification_type, channel, template_key, smtp_connection_id, channel_provider_id,
     send_priority, pii_retention_days, retention_days)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: UpdateChannelRouteSettings :one
-- Ajustes operables del Tipo (prioridad y retención); el resto se recrea.
-- Los NULL en retención son valores válidos (heredar / no borrar), no "sin cambio".
UPDATE channel_routes
SET send_priority = $3, pii_retention_days = $4, retention_days = $5
WHERE id = $1 AND application_id = $2
RETURNING *;

-- name: DeleteChannelRoute :exec
DELETE FROM channel_routes WHERE id = $1 AND application_id = $2;
