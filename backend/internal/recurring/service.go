// Package recurring implementa las notificaciones recurrentes (§6.5): un
// PeriodicTaskManager (en queue) registra las entradas cron de los horarios
// habilitados y, en cada disparo, este servicio crea una notificación reusando
// el flujo de envío estándar. Cron en UTC; sin catch-up (asynq no rellena
// disparos perdidos); el solapamiento no aplica (cada disparo crea una notif).
package recurring

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"

	"dvtech/qbn/internal/notification"
	"dvtech/qbn/internal/queue"
	"dvtech/qbn/internal/store"
)

// Service crea notificaciones a partir de horarios recurrentes.
type Service struct {
	db     *store.DB
	notif  *notification.Service
	logger *slog.Logger
}

// NewService crea el servicio de recurrentes.
func NewService(db *store.DB, notif *notification.Service, logger *slog.Logger) *Service {
	return &Service{db: db, notif: notif, logger: logger}
}

// Process se invoca en cada disparo del cron: carga el horario y crea una
// notificación (que a su vez se encola y envía por el flujo normal).
func (s *Service) Process(ctx context.Context, scheduleID uuid.UUID) error {
	sch, err := s.db.Q.GetRecurringSchedule(ctx, scheduleID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !sch.Enabled) {
		return nil // horario borrado o deshabilitado: nada que hacer
	}
	if err != nil {
		return err
	}

	var recipient, payload map[string]any
	if err := json.Unmarshal(sch.Recipient, &recipient); err != nil {
		s.logger.Warn("recipient JSON inválido en el horario recurrente", "schedule_id", sch.ID, "error", err)
	}
	if len(sch.Payload) > 0 {
		if err := json.Unmarshal(sch.Payload, &payload); err != nil {
			s.logger.Warn("payload JSON inválido en el horario recurrente", "schedule_id", sch.ID, "error", err)
		}
	}

	// Clave determinista por (horario, tick al minuto): si asynq reentrega el
	// disparo (crash a mitad de Process), la idempotencia de Create devuelve la
	// misma notificación en vez de crear un duplicado. El cron dispara con
	// precisión de minuto, así que el truncado identifica el tick.
	idemKey := "recurring:" + sch.ID.String() + ":" + time.Now().UTC().Truncate(time.Minute).Format(time.RFC3339)

	_, err = s.notif.Create(ctx, notification.CreateInput{
		ApplicationID:    sch.ApplicationID,
		TemplateKey:      sch.TemplateKey,
		NotificationType: sch.NotificationType,
		Channel:          string(sch.Channel),
		Recipient:        recipient,
		RecipientUserID:  sch.RecipientUserID,
		Payload:          payload,
		Priority:         string(sch.Priority),
		Locale:           sch.Locale,
		IdempotencyKey:   &idemKey,
	})
	if err != nil {
		s.logger.Error("recurring create failed", slog.String("schedule_id", scheduleID.String()), slog.Any("error", err))
		return err
	}
	return nil
}

// ConfigProvider implementa asynq.PeriodicTaskConfigProvider leyendo los
// horarios habilitados de la BD (el manager lo consulta periódicamente).
type ConfigProvider struct {
	db *store.DB
}

// NewConfigProvider crea el provider de configuración periódica.
func NewConfigProvider(db *store.DB) *ConfigProvider { return &ConfigProvider{db: db} }

// GetConfigs devuelve una entrada cron por horario habilitado.
func (c *ConfigProvider) GetConfigs() ([]*asynq.PeriodicTaskConfig, error) {
	schedules, err := c.db.Q.ListEnabledRecurringSchedules(context.Background())
	if err != nil {
		return nil, err
	}
	configs := make([]*asynq.PeriodicTaskConfig, 0, len(schedules))
	for _, sch := range schedules {
		task, err := queue.NewRecurringTask(sch.ID)
		if err != nil {
			continue
		}
		configs = append(configs, &asynq.PeriodicTaskConfig{Cronspec: sch.Cron, Task: task})
	}
	return configs, nil
}

var _ asynq.PeriodicTaskConfigProvider = (*ConfigProvider)(nil)
