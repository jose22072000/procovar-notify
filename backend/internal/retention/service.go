// Package retention implementa la política de privacidad de PII (§11.1):
// anonimización del contenido de notificaciones tras el periodo de retención de
// cada aplicación. (El "derecho al olvido" por usuario vive en internal/admin.)
package retention

import (
	"context"
	"log/slog"

	"dvtech/qbn/internal/store"
)

// Service ejecuta las tareas de retención/anonimización.
type Service struct {
	db     *store.DB
	logger *slog.Logger
}

// NewService crea el servicio de retención.
func NewService(db *store.DB, logger *slog.Logger) *Service {
	return &Service{db: db, logger: logger}
}

// PurgeExpired anonimiza (vacía payload/recipient) las notificaciones que han
// superado el periodo de retención (el del Tipo, o el de la aplicación como
// fallback) y borra físicamente las vencidas por retention_days del Tipo
// (expires_at), con sus intentos y logs vía ON DELETE CASCADE. Devuelve
// cuántas se anonimizaron.
func (s *Service) PurgeExpired(ctx context.Context) (int64, error) {
	n, err := s.db.Q.AnonymizeExpiredNotifications(ctx)
	if err != nil {
		return 0, err
	}
	if n > 0 {
		s.logger.Info("pii anonymized", slog.Int64("notifications", n))
	}
	deleted, err := s.db.Q.DeleteExpiredNotifications(ctx)
	if err != nil {
		return n, err
	}
	if deleted > 0 {
		s.logger.Info("expired notifications deleted", slog.Int64("notifications", deleted))
	}
	return n, nil
}
