-- name: ListBaseTemplates :many
SELECT * FROM base_templates
WHERE is_active = true
ORDER BY category, key;

-- name: GetBaseTemplate :one
SELECT * FROM base_templates WHERE id = $1;

-- name: GetBaseTemplateByKey :one
SELECT * FROM base_templates WHERE key = $1;
