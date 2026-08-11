package admin

import (
	"context"

	"github.com/google/uuid"

	"dvtech/qbn/internal/apperr"
	"dvtech/qbn/internal/audit"
	"dvtech/qbn/internal/store"
	"dvtech/qbn/internal/store/sqlc"
)

// ListSuppressions lista la suppression list de una app, paginada.
func (s *Service) ListSuppressions(ctx context.Context, appID uuid.UUID, limit, offset int32) ([]sqlc.SuppressionList, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.db.Q.ListSuppressions(ctx, sqlc.ListSuppressionsParams{ApplicationID: appID, Limit: limit, Offset: offset})
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return rows, nil
}

// AddSuppression añade (o actualiza) un destinatario suprimido manualmente.
func (s *Service) AddSuppression(ctx context.Context, actor, appID uuid.UUID, channel, recipient, reason string) (sqlc.SuppressionList, error) {
	if channel == "" || recipient == "" {
		return sqlc.SuppressionList{}, apperr.Validation("missing_fields", "channel and recipient are required")
	}
	if reason == "" {
		reason = "manual"
	}
	var row sqlc.SuppressionList
	err := s.db.Tx(ctx, func(q *sqlc.Queries) error {
		var err error
		row, err = q.AddSuppression(ctx, sqlc.AddSuppressionParams{
			ApplicationID: appID, Channel: sqlc.Channel(channel), Recipient: recipient, Reason: reason,
		})
		if err != nil {
			return err
		}
		return s.rec(ctx, q, actor, &appID, audit.ActionSuppressionAdd, "suppression", row.ID.String())
	})
	if err != nil {
		return sqlc.SuppressionList{}, apperr.Internal(err)
	}
	return row, nil
}

// DeleteSuppression elimina un destinatario de la suppression list (lo rehabilita).
func (s *Service) DeleteSuppression(ctx context.Context, actor, appID, id uuid.UUID) error {
	err := s.db.Tx(ctx, func(q *sqlc.Queries) error {
		if err := q.DeleteSuppression(ctx, sqlc.DeleteSuppressionParams{ID: id, ApplicationID: appID}); err != nil {
			return err
		}
		return s.rec(ctx, q, actor, &appID, audit.ActionSuppressionRemove, "suppression", id.String())
	})
	if err != nil {
		return store.MapError(err, "", "")
	}
	return nil
}
