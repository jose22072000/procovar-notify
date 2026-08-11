-- Bloque B (v2): revocación de sesiones admin.

-- +goose Up

-- token_version permite invalidar todos los refresh tokens de un admin de golpe
-- (logout / cambio de contraseña / robo de token): el refresh JWT lleva la
-- versión con la que se emitió y el endpoint /admin/auth/refresh la compara con
-- esta columna. Al incrementarla, todos los refresh anteriores dejan de valer.
ALTER TABLE admin_users ADD COLUMN token_version int NOT NULL DEFAULT 0;

-- +goose Down

ALTER TABLE admin_users DROP COLUMN token_version;
