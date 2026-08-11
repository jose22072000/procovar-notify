package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"dvtech/qbn/internal/config"
)

// TaskRecurring dispara la creación de una notificación a partir de un horario
// recurrente. El PeriodicTaskManager lo encola según el cron de cada schedule.
const TaskRecurring = "notification:recurring"

// RecurringPayload identifica el horario recurrente que disparó la tarea.
type RecurringPayload struct {
	ScheduleID uuid.UUID `json:"schedule_id"`
}

// NewRecurringTask construye la tarea recurrente para un schedule.
func NewRecurringTask(id uuid.UUID) (*asynq.Task, error) {
	payload, err := json.Marshal(RecurringPayload{ScheduleID: id})
	if err != nil {
		return nil, fmt.Errorf("serializando payload recurrente: %w", err)
	}
	return asynq.NewTask(TaskRecurring, payload), nil
}

// RegisterRecurring registra el handler de la tarea recurrente sobre el mux.
func RegisterRecurring(mux *asynq.ServeMux, process func(ctx context.Context, scheduleID uuid.UUID) error) {
	mux.HandleFunc(TaskRecurring, func(ctx context.Context, t *asynq.Task) error {
		var p RecurringPayload
		if err := json.Unmarshal(t.Payload(), &p); err != nil {
			return asynq.SkipRetry
		}
		return process(ctx, p.ScheduleID)
	})
}

// NewPeriodicManager crea el PeriodicTaskManager que registra las entradas cron
// del provider dado (cron en UTC) y las sincroniza periódicamente desde la BD.
func NewPeriodicManager(cfg config.RedisConfig, provider asynq.PeriodicTaskConfigProvider) (*asynq.PeriodicTaskManager, error) {
	return asynq.NewPeriodicTaskManager(asynq.PeriodicTaskManagerOpts{
		RedisUniversalClient:       newUniversalClient(cfg, cfg.DBAsynq),
		PeriodicTaskConfigProvider: provider,
		SyncInterval:               1 * time.Minute,
	})
}
