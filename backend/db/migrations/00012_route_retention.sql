-- Retención por "Tipo de notificación": la vida de un mensaje depende de su
-- tipo (un OTP puede borrarse en días; una factura debe conservarse), así que
-- el Tipo manda y el valor de la aplicación queda como fallback.
--   pii_retention_days: NULL = hereda applications.pii_retention_days.
--   retention_days: TTL de borrado físico (notificación + intentos + logs,
--   vía ON DELETE CASCADE). NULL = no se borra nunca.

-- +goose Up
ALTER TABLE channel_routes ADD COLUMN pii_retention_days int CHECK (pii_retention_days > 0);
ALTER TABLE channel_routes ADD COLUMN retention_days int CHECK (retention_days > 0);

-- El barrido de borrado filtra por expires_at (se fija al crear la notificación).
CREATE INDEX idx_notifications_expires ON notifications(expires_at) WHERE expires_at IS NOT NULL;

-- +goose Down
DROP INDEX idx_notifications_expires;
ALTER TABLE channel_routes DROP COLUMN retention_days;
ALTER TABLE channel_routes DROP COLUMN pii_retention_days;
