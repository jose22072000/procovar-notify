-- Fase 7: retención PII configurable por tenant y horarios recurrentes.

-- +goose Up

-- Retención PII por aplicación (días tras los que se anonimiza el contenido).
ALTER TABLE applications ADD COLUMN pii_retention_days int NOT NULL DEFAULT 90;

-- Horarios recurrentes (digests/recordatorios) por aplicación (cron en UTC).
CREATE TABLE recurring_schedules (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    application_id    uuid NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    name              text NOT NULL,
    template_key      text NOT NULL,
    notification_type text NOT NULL,
    channel           channel NOT NULL,
    recipient         jsonb NOT NULL,
    recipient_user_id text,
    payload           jsonb NOT NULL DEFAULT '{}',
    locale            text,
    priority          priority NOT NULL DEFAULT 'NORMAL',
    cron              text NOT NULL,             -- expresión cron (UTC)
    enabled           boolean NOT NULL DEFAULT true,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_recurring_schedules_app ON recurring_schedules(application_id);
CREATE INDEX idx_recurring_schedules_enabled ON recurring_schedules(enabled) WHERE enabled = true;

CREATE TRIGGER trg_recurring_schedules_updated BEFORE UPDATE ON recurring_schedules
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TABLE IF EXISTS recurring_schedules;
ALTER TABLE applications DROP COLUMN IF EXISTS pii_retention_days;
