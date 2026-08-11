-- Fase 18 (v2.1): webhooks de estado al tenant.

-- +goose Up

-- Endpoints de webhook por aplicación: el tenant recibe POSTs firmados (HMAC)
-- cuando una notificación cambia de estado (sent/failed/...).
CREATE TABLE webhook_endpoints (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    application_id uuid NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    url            text NOT NULL,
    secret_enc     bytea NOT NULL,                 -- secreto de firma cifrado AES-256-GCM
    events         text[] NOT NULL DEFAULT '{}',   -- p.ej. {notification.sent, notification.failed}; vacío = todos
    status         status_active_disabled NOT NULL DEFAULT 'ACTIVE',
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_webhook_endpoints_app ON webhook_endpoints(application_id);

-- +goose Down

DROP TABLE webhook_endpoints;
