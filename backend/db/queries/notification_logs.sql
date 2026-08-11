-- name: CreateNotificationLog :exec
INSERT INTO notification_logs (application_id, notification_id, event, message, details)
VALUES ($1, $2, $3, $4, $5);

-- name: ListNotificationLogsByNotification :many
SELECT * FROM notification_logs
WHERE notification_id = $1
ORDER BY created_at;
