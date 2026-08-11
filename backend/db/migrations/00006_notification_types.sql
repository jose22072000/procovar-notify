-- Fase 21 (v2.2): "Tipos de notificación" autocontenidos (1:1).
-- Cada channel_route pasa a ser un "Tipo de notificación": lleva la plantilla
-- que usa (por template_key estable → siempre la última versión activa) y se
-- identifica por un nombre único por aplicación (notification_type). Se elimina
-- el modelo de fallback: un (tipo) = un canal + una plantilla + un destino.

-- +goose Up

-- Plantilla que usa este Tipo de notificación (key estable; el envío resuelve la
-- última versión activa). NULL hasta que se asigne desde la UI.
ALTER TABLE channel_routes ADD COLUMN template_key text;

-- Sin fallback: fuera prioridad y default (y su UNIQUE compuesto).
ALTER TABLE channel_routes DROP CONSTRAINT channel_routes_application_id_notification_type_channel_pri_key;
ALTER TABLE channel_routes DROP COLUMN priority;
ALTER TABLE channel_routes DROP COLUMN is_default;

-- El nombre (notification_type) identifica de forma única el Tipo dentro de la
-- app; el envío puede referirlo por id o por este nombre.
ALTER TABLE channel_routes ADD CONSTRAINT channel_routes_app_type_key UNIQUE (application_id, notification_type);

-- +goose Down

ALTER TABLE channel_routes DROP CONSTRAINT channel_routes_app_type_key;
ALTER TABLE channel_routes ADD COLUMN is_default boolean NOT NULL DEFAULT false;
ALTER TABLE channel_routes ADD COLUMN priority int NOT NULL DEFAULT 0;
ALTER TABLE channel_routes ADD CONSTRAINT channel_routes_application_id_notification_type_channel_pri_key
    UNIQUE (application_id, notification_type, channel, priority);
ALTER TABLE channel_routes DROP COLUMN template_key;
