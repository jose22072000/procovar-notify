package admin

import (
	"context"

	"github.com/google/uuid"

	"dvtech/qbn/internal/apperr"
	"dvtech/qbn/internal/audit"
	"dvtech/qbn/internal/store/sqlc"
)

// SetQuota fija las cuotas de envío (diaria/mensual; 0 = ilimitada) de una app.
func (s *Service) SetQuota(ctx context.Context, actor, appID uuid.UUID, daily, monthly int32) (sqlc.Application, error) {
	if daily < 0 || monthly < 0 {
		return sqlc.Application{}, apperr.Validation("invalid_quota", "quotas must be >= 0")
	}
	var app sqlc.Application
	err := s.db.Tx(ctx, func(q *sqlc.Queries) error {
		var err error
		app, err = q.SetApplicationQuota(ctx, sqlc.SetApplicationQuotaParams{
			ID: appID, DailySendQuota: daily, MonthlySendQuota: monthly,
		})
		if err != nil {
			return err
		}
		return s.rec(ctx, q, actor, &appID, audit.ActionApplicationUpdate, "application", appID.String())
	})
	if err != nil {
		return sqlc.Application{}, apperr.Internal(err)
	}
	return app, nil
}
