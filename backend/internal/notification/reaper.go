package notification

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"dvtech/qbn/internal/store/sqlc"
)

// Reaper: cierra el hueco commit-luego-encolar. Create commitea la fila
// (PENDING) antes de EnqueueSend; si el proceso muere entre ambos pasos la
// notificación queda huérfana (persistida, sin tarea). RequeueStale la
// reencola; es seguro porque EnqueueSend es idempotente por TaskID.

// Valores por defecto del barrido (ver RequeueStale).
const (
	// DefaultReapThreshold: antigüedad mínima para considerar huérfana una
	// notificación (muy por encima de la latencia normal de cola).
	DefaultReapThreshold = 2 * time.Minute
	// DefaultReapBatch acota cada pasada; lo pendiente sale en la siguiente.
	DefaultReapBatch = 100
)

// RequeueStale reencola las notificaciones huérfanas (PENDING/QUEUED sin
// transición reciente y sin programación futura). Devuelve cuántas reencoló.
func (s *Service) RequeueStale(ctx context.Context, threshold time.Duration, batch int32) (int, error) {
	cutoff := time.Now().Add(-threshold)
	rows, err := s.db.Q.ListStaleNotificationsForRequeue(ctx, sqlc.ListStaleNotificationsForRequeueParams{
		UpdatedAt: pgtype.Timestamptz{Time: cutoff, Valid: true},
		Limit:     batch,
	})
	if err != nil {
		return 0, err
	}

	requeued := 0
	for _, n := range rows {
		// Si la tarea sigue viva en asynq, EnqueueSend es un no-op (TaskID).
		if err := s.enq.EnqueueSend(ctx, n.ID, string(n.Priority), int(n.MaxRetries), time.Time{}); err != nil {
			// No aborta la pasada: el resto de huérfanas aún puede rescatarse.
			s.logger.Error("reaper: reenqueue failed",
				slog.String("notification_id", n.ID.String()), slog.Any("error", err))
			continue
		}
		requeued++
		msg := "reencolada por el reaper (huérfana tras commit sin encolar)"
		warnErr(s.logger, "reaper: no se pudo escribir el log", s.db.Q.CreateNotificationLog(ctx, sqlc.CreateNotificationLogParams{
			ApplicationID:  n.ApplicationID,
			NotificationID: pgUUID(n.ID),
			Event:          "requeued",
			Message:        &msg,
		}))
	}
	if requeued > 0 {
		s.logger.Info("reaper: notificaciones huérfanas reencoladas", slog.Int("count", requeued))
	}
	return requeued, nil
}

// ReconcileStuckProcessing resuelve las PROCESSING atascadas (crash a mitad de
// envío o mark-SENT fallido) contra su último intento: SUCCESS => el mensaje ya
// llegó, se marca SENT sin reenviar (evita el duplicado); si no, se reencola.
// Devuelve (marcadas SENT, reencoladas).
func (s *Service) ReconcileStuckProcessing(ctx context.Context, threshold time.Duration, batch int32) (sent, requeued int, err error) {
	cutoff := time.Now().Add(-threshold)
	rows, err := s.db.Q.ListStaleProcessingForReconcile(ctx, sqlc.ListStaleProcessingForReconcileParams{
		UpdatedAt: pgtype.Timestamptz{Time: cutoff, Valid: true},
		Limit:     batch,
	})
	if err != nil {
		return 0, 0, err
	}
	for _, n := range rows {
		if n.LastAttemptStatus == string(sqlc.DeliveryStatusSUCCESS) {
			if err := s.db.Q.MarkNotificationSent(ctx, n.ID); err != nil {
				s.logger.Error("reaper: mark sent (reconcile) failed", slog.String("notification_id", n.ID.String()), slog.Any("error", err))
				continue
			}
			msg := "reconciliada por el reaper: intento SUCCESS previo, no se reenvía"
			warnErr(s.logger, "reaper: no se pudo escribir el log", s.db.Q.CreateNotificationLog(ctx, sqlc.CreateNotificationLogParams{
				ApplicationID: n.ApplicationID, NotificationID: pgUUID(n.ID), Event: "sent", Message: &msg,
			}))
			sent++
			continue
		}
		if err := s.enq.EnqueueSend(ctx, n.ID, string(n.Priority), int(n.MaxRetries), time.Time{}); err != nil {
			s.logger.Error("reaper: reenqueue processing failed", slog.String("notification_id", n.ID.String()), slog.Any("error", err))
			continue
		}
		msg := "reencolada por el reaper (PROCESSING atascada sin intento exitoso)"
		warnErr(s.logger, "reaper: no se pudo escribir el log", s.db.Q.CreateNotificationLog(ctx, sqlc.CreateNotificationLogParams{
			ApplicationID: n.ApplicationID, NotificationID: pgUUID(n.ID), Event: "requeued", Message: &msg,
		}))
		requeued++
	}
	if sent > 0 || requeued > 0 {
		s.logger.Info("reaper: PROCESSING atascadas reconciliadas", slog.Int("sent", sent), slog.Int("requeued", requeued))
	}
	return sent, requeued, nil
}

// requeueIfStalled reencola (best-effort) una huérfana al devolverla por
// idempotencia, rescatándola sin esperar al reaper. Solo PENDING: Create marca
// QUEUED tras un encolado exitoso, así que PENDING delata que nunca se encoló
// (las QUEUED atascadas son del reaper). Los fallos no se propagan: el
// contrato es devolver la existente y el reaper garantiza el rescate.
func (s *Service) requeueIfStalled(ctx context.Context, n sqlc.Notification) {
	if n.Status != sqlc.NotificationStatusPENDING {
		return
	}
	// Programada a futuro: se reencola respetando su hora (ProcessAt).
	var processAt time.Time
	if n.ScheduledAt.Valid && n.ScheduledAt.Time.After(time.Now()) {
		processAt = n.ScheduledAt.Time
	}
	if err := s.enq.EnqueueSend(ctx, n.ID, string(n.Priority), int(n.MaxRetries), processAt); err != nil {
		s.logger.Error("idempotency hit: reenqueue failed (el reaper la rescatará)",
			slog.String("notification_id", n.ID.String()), slog.Any("error", err))
	}
}
