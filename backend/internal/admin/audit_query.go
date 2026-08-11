package admin

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"dvtech/qbn/internal/apperr"
	"dvtech/qbn/internal/store/sqlc"
)

// ListAuditLogs devuelve el registro de auditoría de administración de una
// aplicación, paginado (más recientes primero).
func (s *Service) ListAuditLogs(ctx context.Context, appID uuid.UUID, limit, offset int32) ([]sqlc.AdminAuditLog, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	logs, err := s.db.Q.ListAuditLogsByApplication(ctx, sqlc.ListAuditLogsByApplicationParams{
		ApplicationID: pgtype.UUID{Bytes: appID, Valid: true},
		Limit:         limit,
		Offset:        offset,
	})
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return logs, nil
}
