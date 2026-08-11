package notification

import (
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"dvtech/qbn/internal/store/sqlc"
)

// warnErr deja rastro (Warn) de un error de una operación best-effort que no debe
// abortar el flujo, p. ej. escribir en el NotificationLog de auditoría. Evita que
// esos fallos se traguen en silencio.
func warnErr(logger *slog.Logger, msg string, err error) {
	if err != nil && logger != nil {
		logger.Warn(msg, "error", err)
	}
}

// derefOr devuelve *s o def si s es nil/vacío.
func derefOr(s *string, def string) string {
	if s == nil || *s == "" {
		return def
	}
	return *s
}

// ptr devuelve un puntero a s.
func ptr(s string) *string { return &s }

// priorityOr normaliza la prioridad de entrada (default NORMAL).
func priorityOr(p string) sqlc.Priority {
	switch p {
	case "LOW", "HIGH", "URGENT":
		return sqlc.Priority(p)
	default:
		return sqlc.PriorityNORMAL
	}
}

// pgUUID envuelve un uuid.UUID en el pgtype.UUID (nullable) que espera sqlc.
func pgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}
