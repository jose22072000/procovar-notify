-- name: CreateAuditLog :one
-- Registra una acción del panel de administración. Se invoca dentro de la misma
-- transacción que la acción mutante (ver internal/audit). Usado desde la Fase 6.
INSERT INTO admin_audit_logs (
    actor_admin_id, application_id, action, target_type, target_id, details, ip, user_agent
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
)
RETURNING *;

-- name: ListAuditLogsByApplication :many
SELECT * FROM admin_audit_logs
WHERE application_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;
