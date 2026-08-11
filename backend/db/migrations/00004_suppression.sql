-- Fase 19 (v2.1): suppression list por aplicación (bounces/complaints).

-- +goose Up

-- Destinatarios suprimidos: el envío los salta (no se reintenta a direcciones
-- que rebotan o se quejan). recipient es el email o teléfono normalizado.
CREATE TABLE suppression_list (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    application_id uuid NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    channel        channel NOT NULL,
    recipient      text NOT NULL,                 -- email o teléfono (según canal)
    reason         text NOT NULL DEFAULT 'manual',-- bounce | complaint | manual | ...
    created_at     timestamptz NOT NULL DEFAULT now(),
    UNIQUE (application_id, channel, recipient)
);

CREATE INDEX idx_suppression_app ON suppression_list(application_id);

-- +goose Down

DROP TABLE suppression_list;
