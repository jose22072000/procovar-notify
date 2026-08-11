package monitor

import (
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func pgUUID(id uuid.UUID) pgtype.UUID { return pgtype.UUID{Bytes: id, Valid: true} }

func ptr(s string) *string { return &s }

// warnErr deja rastro (Warn) de un fallo best-effort (p. ej. log de auditoría).
func warnErr(logger *slog.Logger, msg string, err error) {
	if err != nil && logger != nil {
		logger.Warn(msg, "error", err)
	}
}
