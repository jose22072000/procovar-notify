-- +goose Up
-- Nombre legible de la API key para saber de qué servicio es (qb-back, qb-panel,
-- qb-booking…). Sin esto la lista solo muestra el key_id y es imposible
-- identificar cuál pertenece a cada microservicio.
ALTER TABLE api_keys ADD COLUMN name text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE api_keys DROP COLUMN name;
