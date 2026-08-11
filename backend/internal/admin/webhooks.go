package admin

import (
	"context"

	"github.com/google/uuid"

	"dvtech/qbn/internal/apperr"
	"dvtech/qbn/internal/audit"
	"dvtech/qbn/internal/store"
	"dvtech/qbn/internal/store/sqlc"
)

// WebhookCreated devuelve el endpoint creado junto al secreto de firma, que se
// muestra una sola vez (en BD solo va cifrado).
type WebhookCreated struct {
	Endpoint sqlc.WebhookEndpoint
	Secret   string
}

// CreateWebhook crea un endpoint de webhook: genera el secreto de firma, lo
// cifra y lo persiste. El secreto se devuelve en claro una única vez.
func (s *Service) CreateWebhook(ctx context.Context, actor, appID uuid.UUID, url string, events []string) (WebhookCreated, error) {
	if url == "" {
		return WebhookCreated{}, apperr.Validation("missing_url", "url is required")
	}
	secret, err := randomSecret(32)
	if err != nil {
		return WebhookCreated{}, apperr.Internal(err)
	}
	secretEnc, err := s.enc.Encrypt([]byte(secret))
	if err != nil {
		return WebhookCreated{}, apperr.Internal(err)
	}
	if events == nil {
		events = []string{}
	}

	var ep sqlc.WebhookEndpoint
	err = s.db.Tx(ctx, func(q *sqlc.Queries) error {
		var err error
		ep, err = q.CreateWebhookEndpoint(ctx, sqlc.CreateWebhookEndpointParams{
			ApplicationID: appID, Url: url, SecretEnc: secretEnc, Events: events,
		})
		if err != nil {
			return err
		}
		return s.rec(ctx, q, actor, &appID, audit.ActionWebhookCreate, "webhook_endpoint", ep.ID.String())
	})
	if err != nil {
		return WebhookCreated{}, apperr.Internal(err)
	}
	return WebhookCreated{Endpoint: ep, Secret: secret}, nil
}

// ListWebhooks lista los endpoints de webhook de una app (sin exponer secreto).
func (s *Service) ListWebhooks(ctx context.Context, appID uuid.UUID) ([]sqlc.WebhookEndpoint, error) {
	eps, err := s.db.Q.ListWebhookEndpoints(ctx, appID)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return eps, nil
}

// DeleteWebhook elimina un endpoint de webhook.
func (s *Service) DeleteWebhook(ctx context.Context, actor, appID, id uuid.UUID) error {
	err := s.db.Tx(ctx, func(q *sqlc.Queries) error {
		if err := q.DeleteWebhookEndpoint(ctx, sqlc.DeleteWebhookEndpointParams{ID: id, ApplicationID: appID}); err != nil {
			return err
		}
		return s.rec(ctx, q, actor, &appID, audit.ActionWebhookDelete, "webhook_endpoint", id.String())
	})
	if err != nil {
		return store.MapError(err, "", "")
	}
	return nil
}
