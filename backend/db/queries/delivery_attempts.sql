-- name: CreateDeliveryAttempt :one
INSERT INTO delivery_attempts (notification_id, attempt_number, asynq_task_id, status)
VALUES ($1, $2, $3, 'PROCESSING')
RETURNING *;

-- name: ListDeliveryAttemptsByNotification :many
SELECT * FROM delivery_attempts
WHERE notification_id = $1
ORDER BY attempt_number;

-- name: HasSuccessfulDeliveryAttempt :one
-- Guardia anti-duplicado del worker: con at-least-once, una reentrega tras un
-- crash post-envío reenviaría de verdad; si ya hay un intento SUCCESS, el
-- mensaje YA llegó y solo falta marcar SENT.
SELECT EXISTS (
    SELECT 1 FROM delivery_attempts
    WHERE notification_id = $1 AND status = 'SUCCESS'
) AS sent;

-- name: FinishDeliveryAttempt :exec
UPDATE delivery_attempts
SET status = $2, provider_ref = $3, error_code = $4, error_message = $5, finished_at = now()
WHERE id = $1;
