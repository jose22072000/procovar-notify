package store

import (
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"dvtech/qbn/internal/apperr"
)

// Códigos de error de PostgreSQL relevantes.
const (
	pgUniqueViolation     = "23505"
	pgForeignKeyViolation = "23503"
)

// MapError traduce un error de pgx a un *apperr.Error con el Kind adecuado, sin
// filtrar el error del driver al cliente. Pensado para las capas CRUD:
//   - sin filas        → NotFound
//   - violación unique → Conflict (p. ej. idempotencia, slug duplicado)
//   - violación FK     → Validation
//   - resto            → Internal (con la causa envuelta para logs)
//
// notFoundCode/conflictCode permiten dar un code de máquina específico del
// recurso (p. ej. "notification_not_found"); si se omiten se usan genéricos.
func MapError(err error, notFoundCode, conflictCode string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return apperr.NotFound(orDefault(notFoundCode, "not_found"), "Resource not found")
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case pgUniqueViolation:
			return apperr.Conflict(orDefault(conflictCode, "conflict"), "Resource already exists")
		case pgForeignKeyViolation:
			return apperr.Validation("foreign_key_violation", "Referenced resource does not exist")
		}
	}
	return apperr.Internal(err)
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
