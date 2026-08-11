package admin

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	"dvtech/qbn/internal/apperr"
	"dvtech/qbn/internal/audit"
	"dvtech/qbn/internal/store"
	"dvtech/qbn/internal/store/sqlc"
)

// RecurringInput son los datos de un horario recurrente.
type RecurringInput struct {
	Name             string
	TemplateKey      string
	NotificationType string
	Channel          string
	Recipient        map[string]any
	RecipientUserID  *string
	Payload          map[string]any
	Locale           *string
	Priority         string
	Cron             string
}

// ListRecurring lista los horarios recurrentes de una app.
func (s *Service) ListRecurring(ctx context.Context, appID uuid.UUID) ([]sqlc.RecurringSchedule, error) {
	rs, err := s.db.Q.ListRecurringSchedulesByApp(ctx, appID)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return rs, nil
}

// CreateRecurring crea un horario recurrente (cron en UTC).
func (s *Service) CreateRecurring(ctx context.Context, actor, appID uuid.UUID, in RecurringInput) (sqlc.RecurringSchedule, error) {
	if in.Name == "" || in.TemplateKey == "" || in.Cron == "" {
		return sqlc.RecurringSchedule{}, apperr.Validation("missing_fields", "name, templateKey and cron are required")
	}
	priority := in.Priority
	if priority == "" {
		priority = "NORMAL"
	}
	recipientJSON, _ := json.Marshal(in.Recipient)
	payloadJSON, _ := json.Marshal(in.Payload)

	var sch sqlc.RecurringSchedule
	err := s.db.Tx(ctx, func(q *sqlc.Queries) error {
		var err error
		sch, err = q.CreateRecurringSchedule(ctx, sqlc.CreateRecurringScheduleParams{
			ApplicationID:    appID,
			Name:             in.Name,
			TemplateKey:      in.TemplateKey,
			NotificationType: in.NotificationType,
			Channel:          sqlc.Channel(in.Channel),
			Recipient:        recipientJSON,
			RecipientUserID:  in.RecipientUserID,
			Payload:          payloadJSON,
			Locale:           in.Locale,
			Priority:         sqlc.Priority(priority),
			Cron:             in.Cron,
		})
		if err != nil {
			return err
		}
		return s.rec(ctx, q, actor, &appID, audit.ActionRecurringCreate, "recurring_schedule", sch.ID.String())
	})
	if err != nil {
		return sqlc.RecurringSchedule{}, apperr.Internal(err)
	}
	return sch, nil
}

// DeleteRecurring elimina un horario recurrente.
func (s *Service) DeleteRecurring(ctx context.Context, actor, appID, id uuid.UUID) error {
	return s.db.Tx(ctx, func(q *sqlc.Queries) error {
		if err := q.DeleteRecurringSchedule(ctx, sqlc.DeleteRecurringScheduleParams{ID: id, ApplicationID: appID}); err != nil {
			return err
		}
		return s.rec(ctx, q, actor, &appID, audit.ActionRecurringDelete, "recurring_schedule", id.String())
	})
}

// SetRetention fija el periodo de retención PII (días) de una aplicación.
func (s *Service) SetRetention(ctx context.Context, actor, appID uuid.UUID, days int32) (sqlc.Application, error) {
	if days <= 0 {
		return sqlc.Application{}, apperr.Validation("invalid_retention", "retentionDays must be positive")
	}
	var app sqlc.Application
	err := s.db.Tx(ctx, func(q *sqlc.Queries) error {
		var err error
		app, err = q.SetApplicationRetention(ctx, sqlc.SetApplicationRetentionParams{ID: appID, PiiRetentionDays: days})
		if err != nil {
			return err
		}
		return s.rec(ctx, q, actor, &appID, audit.ActionRetentionUpdate, "application", appID.String())
	})
	if err != nil {
		return sqlc.Application{}, apperr.Internal(err)
	}
	return app, nil
}

// ForgetUser anonimiza todas las notificaciones de un usuario (derecho al olvido).
func (s *Service) ForgetUser(ctx context.Context, actor, appID uuid.UUID, userID string) (int64, error) {
	uid := userID
	var affected int64
	err := s.db.Tx(ctx, func(q *sqlc.Queries) error {
		var err error
		affected, err = q.AnonymizeNotificationsByRecipient(ctx, sqlc.AnonymizeNotificationsByRecipientParams{
			ApplicationID: appID, RecipientUserID: &uid,
		})
		if err != nil {
			return err
		}
		return s.rec(ctx, q, actor, &appID, audit.ActionUserForget, "user", userID)
	})
	if err != nil {
		return 0, store.MapError(err, "", "")
	}
	return affected, nil
}
