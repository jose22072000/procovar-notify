-- name: GetLatestActiveTemplate :one
-- Resuelve la última versión activa de un template para (app, key, locale).
SELECT * FROM templates
WHERE application_id = $1 AND key = $2 AND locale = $3 AND is_active = true
ORDER BY version DESC
LIMIT 1;

-- name: GetTemplateByVersion :one
SELECT * FROM templates
WHERE application_id = $1 AND key = $2 AND locale = $3 AND version = $4;

-- name: GetTemplateByID :one
SELECT * FROM templates WHERE id = $1 AND application_id = $2;

-- name: GetMaxTemplateVersion :one
-- Mayor versión existente para (app, key, locale); 0 si no hay ninguna.
SELECT COALESCE(MAX(version), 0)::int AS max_version
FROM templates
WHERE application_id = $1 AND key = $2 AND locale = $3;

-- name: DeactivateTemplateVersions :exec
UPDATE templates SET is_active = false
WHERE application_id = $1 AND key = $2 AND locale = $3;

-- name: CreateTemplateFull :one
INSERT INTO templates (
    application_id, key, channel, base_template_id, name, description,
    subject, structure, body, required_variables, locale, version, kind, is_active
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, true
)
RETURNING *;

-- name: ListActiveTemplates :many
-- Listado para el panel: una fila por template activo del tenant.
SELECT * FROM templates
WHERE application_id = $1 AND is_active = true
ORDER BY key, locale;

-- name: ListTemplateVersions :many
SELECT * FROM templates
WHERE application_id = $1 AND key = $2 AND locale = $3
ORDER BY version DESC;

-- name: DeleteTemplatesByKey :exec
-- Elimina todas las versiones de una plantilla (por app, key, locale).
DELETE FROM templates WHERE application_id = $1 AND key = $2 AND locale = $3;
