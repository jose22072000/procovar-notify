-- Fase 20 (v2.1): cuotas de envío por aplicación.

-- +goose Up

-- Límites de envío por aplicación (0 = ilimitado). Se cuentan en Redis y se
-- rechaza con 429 al excederlos.
ALTER TABLE applications ADD COLUMN daily_send_quota   int NOT NULL DEFAULT 0;
ALTER TABLE applications ADD COLUMN monthly_send_quota int NOT NULL DEFAULT 0;

-- +goose Down

ALTER TABLE applications DROP COLUMN daily_send_quota;
ALTER TABLE applications DROP COLUMN monthly_send_quota;
