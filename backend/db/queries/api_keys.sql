-- name: CreateAPIKey :one
INSERT INTO api_keys (application_id, key_id, secret_enc, scopes, expires_at, name)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetAPIKey :one
SELECT * FROM api_keys WHERE id = $1 AND application_id = $2;

-- name: GetAPIKeyByKeyID :one
-- Usada por la autenticación HMAC (Fase 2): localiza la credencial por su id
-- público para recalcular la firma.
SELECT * FROM api_keys WHERE key_id = $1;

-- name: ListAPIKeysByApplication :many
SELECT * FROM api_keys WHERE application_id = $1 ORDER BY created_at DESC;

-- name: RevokeAPIKey :exec
UPDATE api_keys SET status = 'REVOKED'
WHERE id = $1 AND application_id = $2;

-- name: DeleteAPIKey :exec
-- Borrado real (la revocación solo la desactiva). Sirve para limpiar la lista de
-- keys revocadas, que si no se acumulan sin poder quitarlas. El handler exige que
-- la key esté ya REVOKED: así no se puede borrar por error una key en uso.
DELETE FROM api_keys WHERE id = $1 AND application_id = $2;

-- name: TouchAPIKeyLastUsed :exec
UPDATE api_keys SET last_used_at = now() WHERE id = $1;
