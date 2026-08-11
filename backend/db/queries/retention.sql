-- name: AnonymizeExpiredNotifications :execrows
-- Vacía el contenido PII (payload, recipient) de notificaciones más antiguas
-- que el periodo de retención: el del Tipo (channel_routes.pii_retention_days)
-- si lo define, o el de la aplicación como fallback.
UPDATE notifications n
SET payload = '{}'::jsonb, recipient = '{}'::jsonb
FROM applications a
WHERE n.application_id = a.id
  AND n.created_at < now() - make_interval(days => COALESCE(
        (SELECT cr.pii_retention_days FROM channel_routes cr
          WHERE cr.application_id = n.application_id
            AND cr.notification_type = n.notification_type),
        a.pii_retention_days))
  AND n.payload <> '{}'::jsonb;

-- name: DeleteExpiredNotifications :execrows
-- Borrado físico de notificaciones vencidas (expires_at, fijado al crear según
-- retention_days del Tipo). Solo estados terminales; los intentos y logs caen
-- por ON DELETE CASCADE. Ahorra disco de verdad (la anonimización deja la fila).
DELETE FROM notifications
WHERE expires_at IS NOT NULL
  AND expires_at < now()
  AND status IN ('SENT', 'DELIVERED', 'READ', 'FAILED', 'CANCELLED');

-- name: AnonymizeNotificationsByRecipient :execrows
-- Derecho al olvido: anonimiza todas las notificaciones de un usuario del tenant.
UPDATE notifications
SET payload = '{}'::jsonb, recipient = '{}'::jsonb
WHERE application_id = $1 AND recipient_user_id = $2;
