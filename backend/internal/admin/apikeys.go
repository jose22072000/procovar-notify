package admin

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"dvtech/qbn/internal/apperr"
	"dvtech/qbn/internal/audit"
	"dvtech/qbn/internal/store"
	"dvtech/qbn/internal/store/sqlc"
)

// APIKeyCreated es el resultado de crear una API key. Secret se devuelve EN
// CLARO una única vez (en BD solo queda cifrado).
type APIKeyCreated struct {
	ID     uuid.UUID
	KeyID  string
	Secret string
	Scopes []string
}

// CreateAPIKey genera key_id + secret, cifra el secret y lo persiste. El secret
// en claro solo se devuelve aquí (no se puede recuperar después).
func (s *Service) CreateAPIKey(ctx context.Context, actor, appID uuid.UUID, name string, scopes []string) (APIKeyCreated, error) {
	keyID, err := randomToken(8)
	if err != nil {
		return APIKeyCreated{}, apperr.Internal(err)
	}
	keyID = "key_" + keyID
	secret, err := randomSecret(32)
	if err != nil {
		return APIKeyCreated{}, apperr.Internal(err)
	}
	secretEnc, err := s.enc.Encrypt([]byte(secret))
	if err != nil {
		return APIKeyCreated{}, apperr.Internal(err)
	}
	if len(scopes) == 0 {
		scopes = []string{"notifications:send", "notifications:read"}
	}

	var created sqlc.ApiKey
	err = s.db.Tx(ctx, func(q *sqlc.Queries) error {
		var err error
		created, err = q.CreateAPIKey(ctx, sqlc.CreateAPIKeyParams{
			ApplicationID: appID, KeyID: keyID, SecretEnc: secretEnc, Scopes: scopes, Name: name,
		})
		if err != nil {
			return err
		}
		return s.rec(ctx, q, actor, &appID, audit.ActionAPIKeyCreate, "api_key", created.ID.String())
	})
	if err != nil {
		return APIKeyCreated{}, store.MapError(err, "", "api_key_exists")
	}
	return APIKeyCreated{ID: created.ID, KeyID: keyID, Secret: secret, Scopes: scopes}, nil
}

// ListAPIKeys lista las API keys de una app (sin exponer el secret).
func (s *Service) ListAPIKeys(ctx context.Context, appID uuid.UUID) ([]sqlc.ApiKey, error) {
	keys, err := s.db.Q.ListAPIKeysByApplication(ctx, appID)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return keys, nil
}

// RevokeAPIKey revoca una API key.
func (s *Service) RevokeAPIKey(ctx context.Context, actor, appID, id uuid.UUID) error {
	if _, err := s.db.Q.GetAPIKey(ctx, sqlc.GetAPIKeyParams{ID: id, ApplicationID: appID}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.NotFound("api_key_not_found", "API key not found")
		}
		return apperr.Internal(err)
	}
	return s.db.Tx(ctx, func(q *sqlc.Queries) error {
		if err := q.RevokeAPIKey(ctx, sqlc.RevokeAPIKeyParams{ID: id, ApplicationID: appID}); err != nil {
			return err
		}
		return s.rec(ctx, q, actor, &appID, audit.ActionAPIKeyRevoke, "api_key", id.String())
	})
}

// DeleteAPIKey borra la key de verdad. La revocación solo la desactiva, así que
// las revocadas se acumulaban en la lista sin forma de limpiarlas.
//
// Solo se permite borrar una key YA REVOCADA: así no se puede tumbar por error
// una credencial en uso (revocar primero obliga a un paso consciente, y deja el
// rastro en auditoría antes del borrado).
func (s *Service) DeleteAPIKey(ctx context.Context, actor, appID, id uuid.UUID) error {
	key, err := s.db.Q.GetAPIKey(ctx, sqlc.GetAPIKeyParams{ID: id, ApplicationID: appID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.NotFound("api_key_not_found", "API key not found")
		}
		return apperr.Internal(err)
	}
	if key.Status != sqlc.StatusActiveRevokedREVOKED {
		return apperr.Validation("api_key_not_revoked", "Revoke the API key before deleting it")
	}
	return s.db.Tx(ctx, func(q *sqlc.Queries) error {
		if err := q.DeleteAPIKey(ctx, sqlc.DeleteAPIKeyParams{ID: id, ApplicationID: appID}); err != nil {
			return err
		}
		return s.rec(ctx, q, actor, &appID, audit.ActionAPIKeyDelete, "api_key", id.String())
	})
}

func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func randomSecret(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
