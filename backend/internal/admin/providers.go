package admin

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"dvtech/qbn/internal/apperr"
	"dvtech/qbn/internal/audit"
	"dvtech/qbn/internal/store"
	"dvtech/qbn/internal/store/sqlc"
)

// ProviderInput son los datos de un proveedor de canal (config en claro; cifra).
type ProviderInput struct {
	Channel  string // PUSH | SMS | IN_APP
	Provider string // FCM | APNS | TWILIO | ...
	Name     string
	Config   map[string]any
	Status   string // ACTIVE | DISABLED (update)
}

// ListProviders lista los proveedores de una app.
func (s *Service) ListProviders(ctx context.Context, appID uuid.UUID) ([]sqlc.ChannelProvider, error) {
	ps, err := s.db.Q.ListChannelProviders(ctx, appID)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return ps, nil
}

// CreateProvider crea un proveedor (cifra la config). El envío real por Push/SMS
// llega en la Fase 10; aquí se gestiona la credencial.
func (s *Service) CreateProvider(ctx context.Context, actor, appID uuid.UUID, in ProviderInput) (sqlc.ChannelProvider, error) {
	cfgEnc, err := s.encryptConfig(in.Config)
	if err != nil {
		return sqlc.ChannelProvider{}, err
	}
	var p sqlc.ChannelProvider
	err = s.db.Tx(ctx, func(q *sqlc.Queries) error {
		var err error
		p, err = q.CreateChannelProvider(ctx, sqlc.CreateChannelProviderParams{
			ApplicationID: appID, Channel: sqlc.Channel(in.Channel), Provider: in.Provider,
			Name: in.Name, ConfigEnc: cfgEnc,
		})
		if err != nil {
			return err
		}
		return s.rec(ctx, q, actor, &appID, audit.ActionProviderCreate, "channel_provider", p.ID.String())
	})
	if err != nil {
		return sqlc.ChannelProvider{}, store.MapError(err, "", "provider_name_exists")
	}
	return p, nil
}

// UpdateProvider actualiza un proveedor (re-cifra la config si se provee).
func (s *Service) UpdateProvider(ctx context.Context, actor, appID, id uuid.UUID, in ProviderInput) (sqlc.ChannelProvider, error) {
	current, err := s.getProvider(ctx, appID, id)
	if err != nil {
		return sqlc.ChannelProvider{}, err
	}
	cfgEnc := current.ConfigEnc
	if in.Config != nil {
		if cfgEnc, err = s.encryptConfig(in.Config); err != nil {
			return sqlc.ChannelProvider{}, err
		}
	}
	status := sqlc.StatusActiveDisabled(in.Status)
	if in.Status == "" {
		status = current.Status
	}

	var p sqlc.ChannelProvider
	err = s.db.Tx(ctx, func(q *sqlc.Queries) error {
		var err error
		p, err = q.UpdateChannelProvider(ctx, sqlc.UpdateChannelProviderParams{
			ID: id, ApplicationID: appID, Channel: sqlc.Channel(in.Channel), Provider: in.Provider,
			Name: in.Name, ConfigEnc: cfgEnc, Status: status,
		})
		if err != nil {
			return err
		}
		return s.rec(ctx, q, actor, &appID, audit.ActionProviderUpdate, "channel_provider", id.String())
	})
	if err != nil {
		return sqlc.ChannelProvider{}, apperr.Internal(err)
	}
	return p, nil
}

// DeleteProvider elimina un proveedor.
func (s *Service) DeleteProvider(ctx context.Context, actor, appID, id uuid.UUID) error {
	if _, err := s.getProvider(ctx, appID, id); err != nil {
		return err
	}
	return s.db.Tx(ctx, func(q *sqlc.Queries) error {
		if err := q.DeleteChannelProvider(ctx, sqlc.DeleteChannelProviderParams{ID: id, ApplicationID: appID}); err != nil {
			return err
		}
		return s.rec(ctx, q, actor, &appID, audit.ActionProviderDelete, "channel_provider", id.String())
	})
}

func (s *Service) encryptConfig(cfg map[string]any) ([]byte, error) {
	raw, err := json.Marshal(cfg)
	if err != nil {
		return nil, apperr.Validation("invalid_config", "config must be a JSON object")
	}
	enc, err := s.enc.Encrypt(raw)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return enc, nil
}

func (s *Service) getProvider(ctx context.Context, appID, id uuid.UUID) (sqlc.ChannelProvider, error) {
	p, err := s.db.Q.GetChannelProvider(ctx, sqlc.GetChannelProviderParams{ID: id, ApplicationID: appID})
	if errors.Is(err, pgx.ErrNoRows) {
		return sqlc.ChannelProvider{}, apperr.NotFound("provider_not_found", "Provider not found")
	}
	if err != nil {
		return sqlc.ChannelProvider{}, apperr.Internal(err)
	}
	return p, nil
}
