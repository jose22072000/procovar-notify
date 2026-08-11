-- name: CreateNotification :one
INSERT INTO notifications (
    application_id, template_key, template_version, notification_type, channel,
    recipient_user_id, recipient, payload, attachments, locale, priority,
    status, idempotency_key, scheduled_at, expires_at, max_retries
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16
)
RETURNING *;

-- name: GetNotification :one
-- Lectura con scope de tenant: SIEMPRE filtra por application_id para que un
-- tenant no pueda leer notificaciones de otro.
SELECT * FROM notifications
WHERE id = $1 AND application_id = $2;

-- name: ListNotificationsByApplication :many
SELECT * FROM notifications
WHERE application_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: GetNotificationByIdempotencyKey :one
-- Soporta la idempotencia por tenant (Fase 3): si ya existe, se devuelve.
SELECT * FROM notifications
WHERE application_id = $1 AND idempotency_key = $2;

-- name: UpdateNotificationStatus :one
UPDATE notifications SET status = $3
WHERE id = $1 AND application_id = $2
RETURNING *;

-- name: GetNotificationByID :one
-- Lectura por el worker (proceso de confianza): no filtra por tenant porque el
-- worker opera sobre un notificationId concreto sacado de la cola.
SELECT * FROM notifications WHERE id = $1;

-- name: SetNotificationStatusByID :exec
UPDATE notifications SET status = $2 WHERE id = $1;

-- name: MarkNotificationSent :exec
UPDATE notifications
SET status = 'SENT', sent_at = now()
WHERE id = $1;

-- name: MarkNotificationFailed :exec
UPDATE notifications
SET status = 'FAILED', failed_at = now(), retry_count = $2
WHERE id = $1;

-- name: MarkNotificationDelivered :exec
-- Confirmación de entrega del proveedor (ingesta de eventos). Solo avanza desde
-- SENT para no pisar estados terminales (FAILED/CANCELLED) ni retroceder.
UPDATE notifications
SET status = 'DELIVERED', delivered_at = now(), updated_at = now()
WHERE id = $1 AND application_id = $2 AND status = 'SENT';

-- name: MarkNotificationBounced :execrows
-- Bounce/queja del proveedor: solo avanza desde SENT/DELIVERED (no pisa
-- terminales ni retrocede estados de lectura).
UPDATE notifications
SET status = 'BOUNCED', updated_at = now()
WHERE id = $1 AND application_id = $2 AND status IN ('SENT', 'DELIVERED');

-- name: MarkNotificationRetry :exec
-- Entre reintentos la notificación vuelve a la cola (QUEUED). El estado RETRY
-- es propio de delivery_attempts, no de la notificación (ver §3).
UPDATE notifications
SET status = 'QUEUED', retry_count = $2
WHERE id = $1;

-- name: SetNotificationTemplateVersion :exec
UPDATE notifications SET template_version = $2
WHERE id = $1 AND application_id = $3;

-- name: ListNotificationsCursor :many
-- Listado con filtros opcionales y paginación por cursor (keyset sobre
-- created_at, id). Los filtros nulos no se aplican. Pide LIMIT+1 para saber si
-- hay más página.
-- El filtro `archived` decide qué se ve: NULL/false = solo las activas (lo que
-- pide la campana; si no, la bandeja sería una retortera), true = solo el
-- archivo.
SELECT * FROM notifications
WHERE application_id = @application_id
  AND (CASE WHEN sqlc.narg(archived)::boolean IS TRUE
            THEN archived_at IS NOT NULL
            ELSE archived_at IS NULL END)
  AND (sqlc.narg(status)::notification_status IS NULL OR status = sqlc.narg(status))
  AND (sqlc.narg(channel)::channel IS NULL OR channel = sqlc.narg(channel))
  AND (sqlc.narg(recipient_user_id)::text IS NULL OR recipient_user_id = sqlc.narg(recipient_user_id))
  AND (sqlc.narg(start_date)::timestamptz IS NULL OR created_at >= sqlc.narg(start_date))
  AND (sqlc.narg(end_date)::timestamptz IS NULL OR created_at <= sqlc.narg(end_date))
  AND (
        sqlc.narg(cursor_created)::timestamptz IS NULL
        OR created_at < sqlc.narg(cursor_created)
        OR (created_at = sqlc.narg(cursor_created) AND id < sqlc.narg(cursor_id))
      )
ORDER BY created_at DESC, id DESC
LIMIT @page_limit;

-- name: CancelNotification :one
-- Cancela una notificación si aún no se ha enviado. El worker, al tomar la
-- tarea, verá CANCELLED y no enviará.
UPDATE notifications
SET status = 'CANCELLED'
WHERE id = $1 AND application_id = $2
  AND status IN ('PENDING', 'QUEUED')
RETURNING *;

-- name: ArchiveNotification :one
-- Archivar es una acción del DESTINATARIO y es ortogonal al estado de entrega:
-- por eso es una columna y no un estado más (se archiva algo leído sin perder
-- que fue leído). Idempotente: re-archivar no mueve la fecha.
UPDATE notifications
SET archived_at = COALESCE(archived_at, now())
WHERE id = $1 AND application_id = $2
RETURNING *;

-- name: UnarchiveNotification :one
UPDATE notifications
SET archived_at = NULL
WHERE id = $1 AND application_id = $2
RETURNING *;

-- name: ArchiveReadNotificationsByUser :exec
-- «Archivar todas las leídas» de un usuario: es la acción que de verdad limpia la
-- bandeja. Scopeada por destinatario, así nunca puede tocar las de otro.
UPDATE notifications
SET archived_at = now()
WHERE application_id = $1
  AND recipient_user_id = $2
  AND status = 'READ'
  AND archived_at IS NULL;

-- name: MarkNotificationReadByID :one
UPDATE notifications
SET status = 'READ', read_at = now()
WHERE id = $1 AND application_id = $2
RETURNING *;

-- name: CountNotificationStates :many
-- Conteo por (estado, ¿programada a futuro?) para derivar los estados de cola
-- del monitor (§8.3) con domain.DeriveQueueState.
SELECT status,
       (scheduled_at IS NOT NULL AND scheduled_at > now()) AS scheduled_future,
       count(*)::int AS count
FROM notifications
WHERE application_id = $1
GROUP BY status, scheduled_future;

-- name: CountNotificationsByPriority :many
SELECT priority, count(*)::int AS count
FROM notifications
WHERE application_id = $1
GROUP BY priority;

-- name: RequeueNotification :exec
-- Reintento manual: deja la notificación lista para reenviar (vuelve a QUEUED).
UPDATE notifications
SET status = 'QUEUED', failed_at = NULL
WHERE id = $1 AND application_id = $2;

-- name: ListStaleProcessingForReconcile :many
-- PROCESSING atascadas (crash a mitad de envío, o el mark SENT best-effort
-- falló). El reaper las reconcilia contra su último intento: SUCCESS => el
-- mensaje llegó, solo falta marcar SENT (reenviar duplicaría); si no, se
-- reencola (EnqueueSend es idempotente por TaskID).
SELECT n.id, n.application_id, n.priority, n.max_retries,
       COALESCE((SELECT da.status::text FROM delivery_attempts da
         WHERE da.notification_id = n.id
         ORDER BY da.started_at DESC LIMIT 1), '')::text AS last_attempt_status
FROM notifications n
WHERE n.status = 'PROCESSING'
  AND n.updated_at < $1
ORDER BY n.updated_at
LIMIT $2;

-- name: ListStaleNotificationsForRequeue :many
-- Candidatas del reaper: PENDING/QUEUED sin transición reciente y sin
-- programación futura (huérfanas del hueco commit-luego-encolar). Compara
-- updated_at (lo refresca el trigger) para no pisar reintentos recientes.
SELECT id, application_id, priority, max_retries FROM notifications
WHERE status IN ('PENDING', 'QUEUED')
  AND (scheduled_at IS NULL OR scheduled_at <= now())
  AND updated_at < $1
ORDER BY updated_at
LIMIT $2;
