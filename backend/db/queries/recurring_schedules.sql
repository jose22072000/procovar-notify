-- name: ListEnabledRecurringSchedules :many
-- Usada por el PeriodicTaskManager del worker para registrar las entradas cron.
SELECT * FROM recurring_schedules WHERE enabled = true;

-- name: GetRecurringSchedule :one
SELECT * FROM recurring_schedules WHERE id = $1;

-- name: ListRecurringSchedulesByApp :many
SELECT * FROM recurring_schedules
WHERE application_id = $1
ORDER BY created_at DESC;

-- name: CreateRecurringSchedule :one
INSERT INTO recurring_schedules (
    application_id, name, template_key, notification_type, channel,
    recipient, recipient_user_id, payload, locale, priority, cron
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING *;

-- name: DeleteRecurringSchedule :exec
DELETE FROM recurring_schedules WHERE id = $1 AND application_id = $2;
