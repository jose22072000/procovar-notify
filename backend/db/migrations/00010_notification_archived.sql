-- +goose Up
-- Archivado de notificaciones in-app. Sin esto la bandeja crece sin fin: el
-- usuario puede marcar como leída, pero no quitarla de en medio, y la campana
-- acaba siendo una retortera.
--
-- Se modela como columna (no como un estado más) a propósito: el estado describe
-- el ciclo de ENTREGA (SENT/READ/FAILED…), y archivar es una acción del
-- DESTINATARIO, ortogonal a él. Así se puede archivar algo leído sin perder que
-- fue leído, y el histórico sigue intacto para auditoría.
ALTER TABLE notifications ADD COLUMN archived_at timestamptz;

-- La bandeja consulta siempre por (application_id, recipient_user_id) y ahora
-- excluye las archivadas; el índice parcial mantiene esa consulta barata sin
-- indexar las que ya no se muestran.
CREATE INDEX idx_notifications_inbox_active
    ON notifications (application_id, recipient_user_id, created_at DESC, id DESC)
    WHERE archived_at IS NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_notifications_inbox_active;
ALTER TABLE notifications DROP COLUMN archived_at;
