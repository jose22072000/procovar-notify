-- Estado BOUNCED: un bounce/queja del proveedor marca la notificación (antes
-- solo alimentaba la suppression list y la fila quedaba SENT para siempre).

-- +goose Up
ALTER TYPE notification_status ADD VALUE IF NOT EXISTS 'BOUNCED';

-- +goose Down
-- Postgres no permite eliminar valores de un enum: no-op.
SELECT 1;
