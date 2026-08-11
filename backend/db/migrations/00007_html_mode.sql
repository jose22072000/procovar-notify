-- Fase 23 (v2.3): edición avanzada de plantillas (modo HTML).
-- Cada plantilla lleva un `kind`: 'builder' (bloques del editor visual, valor por
-- defecto) o 'html' (HTML/CSS crudo saneado). En 'html' el `body` guarda el HTML
-- final y `structure` conserva los últimos bloques (para "volver a visual") o '{}'.
-- Las base_templates ganan `body`/`subject` para poder ofrecer las 12 en modo html.

-- +goose Up

ALTER TABLE templates ADD COLUMN kind text NOT NULL DEFAULT 'builder'
    CHECK (kind IN ('builder','html'));

ALTER TABLE base_templates ADD COLUMN kind text NOT NULL DEFAULT 'builder'
    CHECK (kind IN ('builder','html'));
ALTER TABLE base_templates ADD COLUMN body text;      -- HTML crudo de las bases 'html'
ALTER TABLE base_templates ADD COLUMN subject text;   -- asunto sugerido al clonar

-- +goose Down

ALTER TABLE base_templates DROP COLUMN subject;
ALTER TABLE base_templates DROP COLUMN body;
ALTER TABLE base_templates DROP COLUMN kind;
ALTER TABLE templates DROP COLUMN kind;
