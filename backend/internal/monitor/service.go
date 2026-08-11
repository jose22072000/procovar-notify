// Package monitor implementa la vista por aplicación del procesamiento (§8.3):
// resumen de estados de cola, métricas, detalle de una notificación (intentos +
// logs) y acciones de reintento/cancelación. Se alimenta de PostgreSQL (fuente
// fiable y filtrable por tenant); los estados de cola se DERIVAN de la
// notificación con domain.DeriveQueueState (no son un enum de BD).
package monitor

import (
	"context"
	"errors"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"dvtech/qbn/internal/apperr"
	"dvtech/qbn/internal/domain"
	"dvtech/qbn/internal/store"
	"dvtech/qbn/internal/store/sqlc"
)

// Retrier reencola una notificación (lo implementa queue.Client).
type Retrier interface {
	EnqueueRetry(ctx context.Context, id uuid.UUID, priority string, maxRetry int) error
}

// Service expone la vista de monitor por aplicación.
type Service struct {
	db      *store.DB
	retrier Retrier
	logger  *slog.Logger
}

// NewService crea el servicio de monitor.
func NewService(db *store.DB, retrier Retrier, logger *slog.Logger) *Service {
	return &Service{db: db, retrier: retrier, logger: logger}
}

// Summary son los contadores del monitor de cola.
type Summary struct {
	ByState    map[string]int `json:"byState"` // estados de cola derivados
	ByPriority map[string]int `json:"byPriority"`
	Total      int            `json:"total"`
}

// Summary calcula contadores por estado-de-cola y por prioridad.
func (s *Service) Summary(ctx context.Context, appID uuid.UUID) (Summary, error) {
	states, err := s.db.Q.CountNotificationStates(ctx, appID)
	if err != nil {
		return Summary{}, apperr.Internal(err)
	}
	out := Summary{ByState: map[string]int{}, ByPriority: map[string]int{}}
	for _, row := range states {
		scheduledFuture := row.ScheduledFuture != nil && *row.ScheduledFuture
		state := string(domain.DeriveQueueState(string(row.Status), scheduledFuture))
		out.ByState[state] += int(row.Count)
		out.Total += int(row.Count)
	}

	prios, err := s.db.Q.CountNotificationsByPriority(ctx, appID)
	if err != nil {
		return Summary{}, apperr.Internal(err)
	}
	for _, row := range prios {
		out.ByPriority[string(row.Priority)] = int(row.Count)
	}
	return out, nil
}

// Metrics son las métricas por estado de notificación + tasa de error.
type Metrics struct {
	ByStatus  map[string]int `json:"byStatus"`
	Total     int            `json:"total"`
	ErrorRate float64        `json:"errorRate"`
}

// Metrics agrega los conteos por estado de notificación.
func (s *Service) Metrics(ctx context.Context, appID uuid.UUID) (Metrics, error) {
	states, err := s.db.Q.CountNotificationStates(ctx, appID)
	if err != nil {
		return Metrics{}, apperr.Internal(err)
	}
	m := Metrics{ByStatus: map[string]int{}}
	for _, row := range states {
		m.ByStatus[string(row.Status)] += int(row.Count)
		m.Total += int(row.Count)
	}
	if m.Total > 0 {
		m.ErrorRate = float64(m.ByStatus[domain.StatusFailed]) / float64(m.Total)
	}
	return m, nil
}

// Detail es el detalle de una notificación con su línea de tiempo.
type Detail struct {
	Notification sqlc.Notification
	Attempts     []sqlc.DeliveryAttempt
	Logs         []sqlc.NotificationLog
}

// Detail devuelve la notificación + intentos + logs (línea de tiempo, §8.3).
func (s *Service) Detail(ctx context.Context, appID, id uuid.UUID) (Detail, error) {
	n, err := s.db.Q.GetNotification(ctx, sqlc.GetNotificationParams{ID: id, ApplicationID: appID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Detail{}, apperr.NotFound("notification_not_found", "Notification not found")
	}
	if err != nil {
		return Detail{}, apperr.Internal(err)
	}
	attempts, err := s.db.Q.ListDeliveryAttemptsByNotification(ctx, id)
	if err != nil {
		return Detail{}, apperr.Internal(err)
	}
	logs, err := s.db.Q.ListNotificationLogsByNotification(ctx, pgUUID(id))
	if err != nil {
		return Detail{}, apperr.Internal(err)
	}
	return Detail{Notification: n, Attempts: attempts, Logs: logs}, nil
}

// Retry reencola manualmente una notificación FALLIDA.
func (s *Service) Retry(ctx context.Context, appID, id uuid.UUID) error {
	n, err := s.db.Q.GetNotification(ctx, sqlc.GetNotificationParams{ID: id, ApplicationID: appID})
	if errors.Is(err, pgx.ErrNoRows) {
		return apperr.NotFound("notification_not_found", "Notification not found")
	}
	if err != nil {
		return apperr.Internal(err)
	}
	if string(n.Status) != domain.StatusFailed {
		return apperr.Conflict("not_retryable", "Only failed notifications can be retried")
	}
	if err := s.db.Q.RequeueNotification(ctx, sqlc.RequeueNotificationParams{ID: id, ApplicationID: appID}); err != nil {
		return apperr.Internal(err)
	}
	if err := s.retrier.EnqueueRetry(ctx, id, string(n.Priority), int(n.MaxRetries)); err != nil {
		return apperr.Internal(err)
	}
	warnErr(s.logger, "no se pudo escribir el log de auditoría", s.db.Q.CreateNotificationLog(ctx, sqlc.CreateNotificationLogParams{
		ApplicationID: appID, NotificationID: pgUUID(id), Event: "retry", Message: ptr("manual retry"),
	}))
	return nil
}

// Cancel cancela una notificación aún no enviada (PENDING/QUEUED).
func (s *Service) Cancel(ctx context.Context, appID, id uuid.UUID) error {
	_, err := s.db.Q.CancelNotification(ctx, sqlc.CancelNotificationParams{ID: id, ApplicationID: appID})
	if errors.Is(err, pgx.ErrNoRows) {
		if _, gerr := s.db.Q.GetNotification(ctx, sqlc.GetNotificationParams{ID: id, ApplicationID: appID}); errors.Is(gerr, pgx.ErrNoRows) {
			return apperr.NotFound("notification_not_found", "Notification not found")
		}
		return apperr.Conflict("not_cancellable", "Notification can no longer be cancelled")
	}
	if err != nil {
		return apperr.Internal(err)
	}
	warnErr(s.logger, "no se pudo escribir el log de auditoría", s.db.Q.CreateNotificationLog(ctx, sqlc.CreateNotificationLogParams{
		ApplicationID: appID, NotificationID: pgUUID(id), Event: "cancelled", Message: ptr("manual cancel"),
	}))
	return nil
}
