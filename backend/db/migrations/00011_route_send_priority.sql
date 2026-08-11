-- Prioridad de envío por "Tipo de notificación": el admin la fija una vez y
-- los envíos del tipo la heredan (el cliente puede sobreescribirla por
-- request); antes dependía de que el integrador mandara priority en cada
-- llamada. Se llama send_priority para no confundir con la columna `priority`
-- (orden de fallback) eliminada en la 00006.

-- +goose Up
ALTER TABLE channel_routes ADD COLUMN send_priority priority NOT NULL DEFAULT 'NORMAL';

-- +goose Down
ALTER TABLE channel_routes DROP COLUMN send_priority;
