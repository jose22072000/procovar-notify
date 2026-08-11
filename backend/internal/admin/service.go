// Package admin implementa la lógica de la API de administración: gestión de
// aplicaciones, admins, API keys, conexiones SMTP, proveedores y rutas de canal.
// Cifra los secretos en reposo y audita cada acción mutante (AdminAuditLog).
package admin

import (
	"context"
	"errors"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"dvtech/qbn/internal/apperr"
	"dvtech/qbn/internal/audit"
	"dvtech/qbn/internal/crypto"
	"dvtech/qbn/internal/store"
	"dvtech/qbn/internal/store/sqlc"
)

// Service agrupa las operaciones de administración.
type Service struct {
	db     *store.DB
	enc    *crypto.Encryptor
	logger *slog.Logger
}

// NewService crea el servicio de administración.
func NewService(db *store.DB, enc *crypto.Encryptor, logger *slog.Logger) *Service {
	return &Service{db: db, enc: enc, logger: logger}
}

// rec registra una entrada de auditoría con el q (transacción) dado.
func (s *Service) rec(ctx context.Context, q *sqlc.Queries, actor uuid.UUID, appID *uuid.UUID, action audit.Action, targetType, targetID string) error {
	return audit.Record(ctx, q, audit.Entry{
		ActorAdminID:  actor,
		ApplicationID: appID,
		Action:        action,
		TargetType:    targetType,
		TargetID:      targetID,
	})
}

// --- Aplicaciones (super-admin) ---

// CreateApplication crea un tenant.
func (s *Service) CreateApplication(ctx context.Context, actor uuid.UUID, name, slug string) (sqlc.Application, error) {
	var app sqlc.Application
	err := s.db.Tx(ctx, func(q *sqlc.Queries) error {
		var err error
		app, err = q.CreateApplication(ctx, sqlc.CreateApplicationParams{Name: name, Slug: slug})
		if err != nil {
			return err
		}
		return s.rec(ctx, q, actor, &app.ID, audit.ActionApplicationCreate, "application", app.ID.String())
	})
	if err != nil {
		return sqlc.Application{}, store.MapError(err, "", "application_slug_exists")
	}
	return app, nil
}

// ListApplications lista todos los tenants.
func (s *Service) ListApplications(ctx context.Context) ([]sqlc.Application, error) {
	apps, err := s.db.Q.ListApplications(ctx)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return apps, nil
}

// GetApplication obtiene un tenant.
func (s *Service) GetApplication(ctx context.Context, id uuid.UUID) (sqlc.Application, error) {
	app, err := s.db.Q.GetApplication(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return sqlc.Application{}, apperr.NotFound("application_not_found", "Application not found")
	}
	if err != nil {
		return sqlc.Application{}, apperr.Internal(err)
	}
	return app, nil
}

// UpdateApplication cambia el nombre y/o el estado (suspend/activar) del tenant.
func (s *Service) UpdateApplication(ctx context.Context, actor, id uuid.UUID, name *string, slug *string, status *string) (sqlc.Application, error) {
	// Partimos de la app actual: si no se cambia ningún campo, se devuelve tal cual
	// (antes quedaba un sqlc.Application{} en blanco).
	app, err := s.GetApplication(ctx, id)
	if err != nil {
		return sqlc.Application{}, err
	}
	err = s.db.Tx(ctx, func(q *sqlc.Queries) error {
		var e error
		if name != nil && *name != "" {
			if app, e = q.UpdateApplication(ctx, sqlc.UpdateApplicationParams{ID: id, Name: *name}); e != nil {
				return e
			}
		}
		if slug != nil && *slug != "" {
			if app, e = q.UpdateApplicationSlug(ctx, sqlc.UpdateApplicationSlugParams{ID: id, Slug: *slug}); e != nil {
				return e
			}
		}
		if status != nil && *status != "" {
			if app, e = q.SetApplicationStatus(ctx, sqlc.SetApplicationStatusParams{
				ID: id, Status: sqlc.StatusActiveSuspended(*status),
			}); e != nil {
				return e
			}
		}
		return s.rec(ctx, q, actor, &id, audit.ActionApplicationUpdate, "application", id.String())
	})
	if err != nil {
		// El slug es UNIQUE: un duplicado debe salir como conflicto, no como 500.
		return sqlc.Application{}, store.MapError(err, "", "application_slug_exists")
	}
	return app, nil
}

// --- Admin users (super-admin) ---

// CreateAdminUser crea un usuario del panel (con password Argon2id).
func (s *Service) CreateAdminUser(ctx context.Context, actor uuid.UUID, email, password, role string, appID *uuid.UUID) (sqlc.AdminUser, error) {
	hash, err := crypto.HashPassword(password)
	if err != nil {
		return sqlc.AdminUser{}, apperr.Internal(err)
	}
	var pgApp pgtype.UUID
	if appID != nil {
		pgApp = pgtype.UUID{Bytes: *appID, Valid: true}
	}

	var u sqlc.AdminUser
	err = s.db.Tx(ctx, func(q *sqlc.Queries) error {
		var err error
		u, err = q.CreateAdminUser(ctx, sqlc.CreateAdminUserParams{
			Email: email, PasswordHash: hash, Role: sqlc.AdminRole(role), ApplicationID: pgApp,
		})
		if err != nil {
			return err
		}
		return s.rec(ctx, q, actor, appID, audit.ActionAdminUserCreate, "admin_user", u.ID.String())
	})
	if err != nil {
		return sqlc.AdminUser{}, store.MapError(err, "", "admin_email_exists")
	}
	return u, nil
}

// ListAdminUsers lista los usuarios del panel.
func (s *Service) ListAdminUsers(ctx context.Context) ([]sqlc.AdminUser, error) {
	us, err := s.db.Q.ListAdminUsers(ctx)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return us, nil
}
