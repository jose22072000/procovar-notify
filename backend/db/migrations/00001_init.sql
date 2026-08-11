-- Esquema inicial de QB Notify v2 (multi-tenant). Implementa el modelo de
-- dominio de new-version.md §3. Postgres 16 (gen_random_uuid() está en el core).
--
-- Convenciones:
--  * Toda entidad de tenant cuelga de applications.id vía application_id.
--  * Los secretos (SMTP/proveedor/API key) se guardan cifrados en columnas bytea
--    (*_enc); el cifrado lo hace internal/crypto, nunca la BD.
--  * updated_at se mantiene con un trigger común (set_updated_at).

-- +goose Up
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION set_updated_at() RETURNS trigger AS $$
BEGIN
  NEW.updated_at = now();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- Enums (conjuntos cerrados). El campo "provider" de channel_providers se deja
-- como text (lista abierta: FCM, APNS, TWILIO, ...) para no requerir ALTER TYPE
-- al añadir proveedores nuevos.
CREATE TYPE status_active_suspended AS ENUM ('ACTIVE', 'SUSPENDED');
CREATE TYPE status_active_revoked   AS ENUM ('ACTIVE', 'REVOKED');
CREATE TYPE status_active_disabled  AS ENUM ('ACTIVE', 'DISABLED');
CREATE TYPE channel                 AS ENUM ('EMAIL', 'PUSH', 'SMS', 'IN_APP');
CREATE TYPE priority                AS ENUM ('LOW', 'NORMAL', 'HIGH', 'URGENT');
CREATE TYPE notification_status     AS ENUM ('PENDING', 'QUEUED', 'PROCESSING', 'SENT', 'DELIVERED', 'READ', 'FAILED', 'CANCELLED');
CREATE TYPE delivery_status         AS ENUM ('PROCESSING', 'SUCCESS', 'FAILED', 'RETRY');
CREATE TYPE admin_role              AS ENUM ('SUPER_ADMIN', 'APP_ADMIN');

-- applications: la raíz del tenant / centro de autenticación.
CREATE TABLE applications (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name        text NOT NULL,
    slug        text NOT NULL UNIQUE,
    status      status_active_suspended NOT NULL DEFAULT 'ACTIVE',
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER trg_applications_updated BEFORE UPDATE ON applications
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- api_keys: credencial de un backend para consumir la API pública (HMAC).
CREATE TABLE api_keys (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    application_id  uuid NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    key_id          text NOT NULL UNIQUE,           -- identificador público (viaja en cabecera)
    secret_enc      bytea NOT NULL,                 -- secret cifrado AES-256-GCM
    scopes          text[] NOT NULL DEFAULT '{}',
    status          status_active_revoked NOT NULL DEFAULT 'ACTIVE',
    last_used_at    timestamptz,
    expires_at      timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_api_keys_application ON api_keys(application_id);

-- smtp_connections: conexiones SMTP de una aplicación.
CREATE TABLE smtp_connections (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    application_id  uuid NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    name            text NOT NULL,                  -- alias legible ("transactional", "marketing")
    host            text NOT NULL,
    port            int  NOT NULL,
    username        text NOT NULL,
    password_enc    bytea NOT NULL,
    from_email      text NOT NULL,
    from_name       text NOT NULL,
    secure          boolean NOT NULL DEFAULT true,
    status          status_active_disabled NOT NULL DEFAULT 'ACTIVE',
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (application_id, name)
);
CREATE TRIGGER trg_smtp_connections_updated BEFORE UPDATE ON smtp_connections
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- channel_providers: proveedor para canales no-email (Push/SMS).
CREATE TABLE channel_providers (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    application_id  uuid NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    channel         channel NOT NULL,
    provider        text NOT NULL,                  -- FCM | APNS | TWILIO | ... (lista abierta)
    name            text NOT NULL,
    config_enc      bytea NOT NULL,
    status          status_active_disabled NOT NULL DEFAULT 'ACTIVE',
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (application_id, name)
);
CREATE TRIGGER trg_channel_providers_updated BEFORE UPDATE ON channel_providers
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- channel_routes: resuelve qué proveedor/SMTP usa cada (tipo, canal). priority
-- ordena el fallback (menor = primario).
CREATE TABLE channel_routes (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    application_id      uuid NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    notification_type   text NOT NULL,              -- categoría libre del tenant
    channel             channel NOT NULL,
    smtp_connection_id  uuid REFERENCES smtp_connections(id) ON DELETE RESTRICT,
    channel_provider_id uuid REFERENCES channel_providers(id) ON DELETE RESTRICT,
    priority            int  NOT NULL DEFAULT 0,
    is_default          boolean NOT NULL DEFAULT false,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    UNIQUE (application_id, notification_type, channel, priority)
);
CREATE INDEX idx_channel_routes_lookup ON channel_routes(application_id, notification_type, channel);
CREATE TRIGGER trg_channel_routes_updated BEFORE UPDATE ON channel_routes
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- base_templates: librería GLOBAL (no pertenece a un tenant) de plantillas
-- predefinidas que el tenant clona como punto de partida.
CREATE TABLE base_templates (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    key                 text NOT NULL UNIQUE,       -- "welcome", "password-reset", ...
    channel             channel NOT NULL,
    category            text NOT NULL,
    name                text NOT NULL,
    description         text,
    structure           jsonb NOT NULL,
    suggested_variables jsonb,
    is_active           boolean NOT NULL DEFAULT true,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER trg_base_templates_updated BEFORE UPDATE ON base_templates
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- templates: plantilla por aplicación y canal, construida con el builder.
CREATE TABLE templates (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    application_id      uuid NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    key                 text NOT NULL,              -- identificador estable usado por el backend
    channel             channel NOT NULL,
    base_template_id    uuid REFERENCES base_templates(id) ON DELETE SET NULL,
    name                text NOT NULL,
    description         text,
    subject             text,                       -- EMAIL / título PUSH (admite variables)
    structure           jsonb NOT NULL,             -- definición del builder (secciones)
    body                text NOT NULL,              -- HTML Handlebars compilado desde structure
    required_variables  jsonb,                      -- JSON Schema derivado de las secciones
    locale              text NOT NULL DEFAULT 'en',
    version             int  NOT NULL DEFAULT 1,
    is_active           boolean NOT NULL DEFAULT true,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    UNIQUE (application_id, key, locale, version)
);
CREATE INDEX idx_templates_lookup ON templates(application_id, key, locale, is_active);
CREATE TRIGGER trg_templates_updated BEFORE UPDATE ON templates
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- notifications: solicitud de notificación. recipient_user_id es opcional
-- (destinatarios anónimos: formularios de contacto).
CREATE TABLE notifications (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    application_id      uuid NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    template_key        text NOT NULL,
    template_version    int,                        -- null = última activa
    notification_type   text NOT NULL,              -- resuelve ChannelRoute
    channel             channel NOT NULL,
    recipient_user_id   text,                       -- userId DEL TENANT (opcional)
    recipient           jsonb NOT NULL,             -- {email?, phone?, push_token?, name?}
    payload             jsonb NOT NULL DEFAULT '{}',
    attachments         jsonb,
    locale              text,
    priority            priority NOT NULL DEFAULT 'NORMAL',
    status              notification_status NOT NULL DEFAULT 'PENDING',
    idempotency_key     text,
    scheduled_at        timestamptz,
    expires_at          timestamptz,
    max_retries         int NOT NULL DEFAULT 3,
    retry_count         int NOT NULL DEFAULT 0,
    sent_at             timestamptz,
    delivered_at        timestamptz,
    read_at             timestamptz,
    failed_at           timestamptz,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_notifications_status    ON notifications(application_id, status);
CREATE INDEX idx_notifications_recipient ON notifications(application_id, recipient_user_id);
CREATE INDEX idx_notifications_scheduled ON notifications(scheduled_at) WHERE scheduled_at IS NOT NULL;
-- Idempotencia por tenant (solo cuando se provee la clave).
CREATE UNIQUE INDEX uq_notifications_idempotency ON notifications(application_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;
CREATE TRIGGER trg_notifications_updated BEFORE UPDATE ON notifications
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- delivery_attempts: cada intento de envío (el job vive en asynq).
CREATE TABLE delivery_attempts (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    notification_id uuid NOT NULL REFERENCES notifications(id) ON DELETE CASCADE,
    attempt_number  int  NOT NULL,
    asynq_task_id   text,
    status          delivery_status NOT NULL DEFAULT 'PROCESSING',
    provider_ref    text,                           -- id del mensaje en el proveedor
    error_code      text,
    error_message   text,
    started_at      timestamptz NOT NULL DEFAULT now(),
    finished_at     timestamptz
);
CREATE INDEX idx_delivery_attempts_notification ON delivery_attempts(notification_id);

-- notification_logs: eventos de auditoría de notificaciones (append-only).
CREATE TABLE notification_logs (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    application_id  uuid NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    notification_id uuid REFERENCES notifications(id) ON DELETE CASCADE,
    event           text NOT NULL,                  -- created | queued | sent | delivered | read | failed | retry
    message         text,
    details         jsonb,
    created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_notification_logs_notification ON notification_logs(notification_id);
CREATE INDEX idx_notification_logs_app_created  ON notification_logs(application_id, created_at);

-- admin_users: usuarios de la SPA de administración.
CREATE TABLE admin_users (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email           text NOT NULL UNIQUE,
    password_hash   text NOT NULL,                  -- Argon2id
    role            admin_role NOT NULL,
    application_id  uuid REFERENCES applications(id) ON DELETE CASCADE,  -- null para SUPER_ADMIN
    status          status_active_disabled NOT NULL DEFAULT 'ACTIVE',
    last_login_at   timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    -- APP_ADMIN debe tener application_id; SUPER_ADMIN no.
    CONSTRAINT chk_admin_app CHECK (
        (role = 'SUPER_ADMIN' AND application_id IS NULL) OR
        (role = 'APP_ADMIN'   AND application_id IS NOT NULL)
    )
);
CREATE INDEX idx_admin_users_application ON admin_users(application_id);
CREATE TRIGGER trg_admin_users_updated BEFORE UPDATE ON admin_users
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- user_notification_preferences: opt-out por usuario del tenant.
CREATE TABLE user_notification_preferences (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    application_id  uuid NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    user_id         text NOT NULL,                  -- userId del tenant
    email_enabled   boolean NOT NULL DEFAULT true,
    push_enabled    boolean NOT NULL DEFAULT true,
    sms_enabled     boolean NOT NULL DEFAULT false,
    in_app_enabled  boolean NOT NULL DEFAULT true,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (application_id, user_id)
);
CREATE TRIGGER trg_user_prefs_updated BEFORE UPDATE ON user_notification_preferences
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- admin_audit_logs: auditoría de acciones del panel de administración.
CREATE TABLE admin_audit_logs (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_admin_id  uuid NOT NULL REFERENCES admin_users(id) ON DELETE RESTRICT,
    application_id  uuid REFERENCES applications(id) ON DELETE SET NULL,  -- recurso afectado (null = global)
    action          text NOT NULL,                  -- "api_key.create", "smtp.update", ...
    target_type     text NOT NULL,
    target_id       text,
    details         jsonb,
    ip              text,
    user_agent      text,
    created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_admin_audit_actor ON admin_audit_logs(actor_admin_id);
CREATE INDEX idx_admin_audit_app   ON admin_audit_logs(application_id, created_at);

-- +goose Down
DROP TABLE IF EXISTS admin_audit_logs;
DROP TABLE IF EXISTS user_notification_preferences;
DROP TABLE IF EXISTS admin_users;
DROP TABLE IF EXISTS notification_logs;
DROP TABLE IF EXISTS delivery_attempts;
DROP TABLE IF EXISTS notifications;
DROP TABLE IF EXISTS templates;
DROP TABLE IF EXISTS base_templates;
DROP TABLE IF EXISTS channel_routes;
DROP TABLE IF EXISTS channel_providers;
DROP TABLE IF EXISTS smtp_connections;
DROP TABLE IF EXISTS api_keys;
DROP TABLE IF EXISTS applications;

DROP TYPE IF EXISTS admin_role;
DROP TYPE IF EXISTS delivery_status;
DROP TYPE IF EXISTS notification_status;
DROP TYPE IF EXISTS priority;
DROP TYPE IF EXISTS channel;
DROP TYPE IF EXISTS status_active_disabled;
DROP TYPE IF EXISTS status_active_revoked;
DROP TYPE IF EXISTS status_active_suspended;

DROP FUNCTION IF EXISTS set_updated_at();
