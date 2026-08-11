package queue

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"

	"dvtech/qbn/internal/channels"
	"dvtech/qbn/internal/config"
	"dvtech/qbn/internal/observability"
)

// Client encola tareas. Implementa la interfaz que consume el servicio de
// notificaciones (EnqueueSend), de modo que el servicio no depende de asynq.
// asynq comparte la conexión Redis endurecida (keepalive + reciclado de ociosas,
// ver newUniversalClient) sobre la DB dedicada REDIS_DB_ASYNQ.
type Client struct {
	client *asynq.Client
	redis  redis.UniversalClient
}

// NewClient crea el cliente de encolado.
func NewClient(cfg config.RedisConfig) *Client {
	rc := newUniversalClient(cfg, cfg.DBAsynq)
	return &Client{client: asynq.NewClientFromRedisClient(rc), redis: rc}
}

// EnqueueSend encola el envío de una notificación, eligiendo cola por prioridad
// y fijando el número máximo de reintentos. Si processAt no es cero, la tarea se
// programa para esa hora (envío programado).
func (c *Client) EnqueueSend(ctx context.Context, id uuid.UUID, priority string, maxRetry int, processAt time.Time) error {
	task, err := newSendTask(id, observability.RequestIDFromContext(ctx))
	if err != nil {
		return err
	}
	opts := []asynq.Option{
		asynq.Queue(queueForPriority(priority)),
		asynq.MaxRetry(maxRetry),
		asynq.TaskID(id.String()), // idempotencia de encolado: 1 tarea activa por notificación
	}
	if !processAt.IsZero() {
		opts = append(opts, asynq.ProcessAt(processAt))
	}
	_, err = c.client.EnqueueContext(ctx, task, opts...)
	// Si ya existe una tarea con ese ID (reintento de encolado), no es un error.
	// asynq envuelve el sentinel con %w, así que hay que usar errors.Is.
	if errors.Is(err, asynq.ErrTaskIDConflict) {
		return nil
	}
	return err
}

// EnqueueRetry vuelve a encolar una notificación (reintento manual desde el
// panel). A diferencia de EnqueueSend, NO fija TaskID: genera una tarea nueva,
// evitando conflictos con la tarea original ya archivada/completada.
func (c *Client) EnqueueRetry(ctx context.Context, id uuid.UUID, priority string, maxRetry int) error {
	task, err := newSendTask(id, observability.RequestIDFromContext(ctx))
	if err != nil {
		return err
	}
	_, err = c.client.EnqueueContext(ctx, task,
		asynq.Queue(queueForPriority(priority)),
		asynq.MaxRetry(maxRetry),
	)
	return err
}

// Close cierra la conexión Redis subyacente (al ser compartida, asynq no la
// cierra por sí mismo).
func (c *Client) Close() error { return c.redis.Close() }

// providerRetryDelay es el retardo entre reintentos cuando el destino está
// caído (breaker abierto, timeout 30 s): mayor que el timeout para que el
// breaker llegue a half-open y pruebe de verdad en el siguiente intento.
const providerRetryDelay = 45 * time.Second

// isFailure decide si el error de un handler consume un reintento. La
// indisponibilidad del destino no es un fallo del mensaje: se reintenta sin
// tocar el contador — con proveedor caído nada acaba FAILED por agotamiento.
func isFailure(err error) bool { return !channels.IsUnavailable(err) }

// retryDelay fija el retardo del próximo reintento: fijo si el destino está
// caído; backoff exponencial por defecto para el resto.
func retryDelay(n int, err error, t *asynq.Task) time.Duration {
	if channels.IsUnavailable(err) {
		return providerRetryDelay
	}
	return asynq.DefaultRetryDelayFunc(n, err, t)
}

// NewServer crea el servidor asynq (worker) con colas ponderadas. El ping
// periódico del health-check (cada 15 s por defecto) mantiene el pool activo y
// deja rastro en el log si el worker pierde Redis — antes una conexión muerta
// por inactividad fallaba en silencio y retrasaba los envíos.
func NewServer(cfg *config.Config, logger *slog.Logger) *asynq.Server {
	return asynq.NewServerFromRedisClient(newUniversalClient(cfg.Redis, cfg.Redis.DBAsynq), asynq.Config{
		Concurrency: cfg.WorkerConcurrency,
		Queues: map[string]int{
			QueueCritical: 6,
			QueueDefault:  3,
			QueueWebhooks: 2,
			QueueLow:      1,
		},
		// Orden estricto (default): critical se atiende siempre antes que
		// default/low — los pesos solos no impiden que envíos lentos ocupen
		// todos los workers. QUEUE_STRICT_PRIORITY=false vuelve al ponderado.
		StrictPriority: cfg.QueueStrictPriority,
		// Indisponibilidad del destino: reintento con retardo sin consumir
		// intentos (los mensajes esperan la recuperación, no se pierden).
		IsFailure:      isFailure,
		RetryDelayFunc: asynq.RetryDelayFunc(retryDelay),
		// El drenado en el apagado debe superar al canal más lento (SMTP 15 s);
		// el default de asynq (8 s) cortaba envíos en vuelo en cada deploy.
		ShutdownTimeout: time.Duration(cfg.QueueShutdownTimeoutSecs) * time.Second,
		HealthCheckFunc: func(err error) {
			if err != nil {
				logger.Error("redis healthcheck failed", "error", err)
			}
		},
	})
}

// Processor procesa el envío de una notificación dado su id.
type Processor interface {
	ProcessSend(ctx context.Context, notificationID uuid.UUID) error
}

// NewMux registra el handler de la tarea de envío sobre el Processor dado.
func NewMux(p Processor) *asynq.ServeMux {
	mux := asynq.NewServeMux()
	mux.HandleFunc(TaskSendNotification, func(ctx context.Context, t *asynq.Task) error {
		payload, err := parseSendPayload(t)
		if err != nil {
			// Payload corrupto: no tiene sentido reintentar.
			return asynq.SkipRetry
		}
		// Correlación api→worker: el request_id del origen viaja en el payload
		// y se reinyecta al contexto para que los logs del envío lo lleven.
		if payload.RequestID != "" {
			ctx = observability.ContextWithRequestID(ctx, payload.RequestID)
		}
		return p.ProcessSend(ctx, payload.NotificationID)
	})
	return mux
}

// PollQueueDepth consulta el tamaño de cada cola (por estado) vía el Inspector
// de asynq y lo publica con report. Pensado para un ticker en el worker: es lo
// que permite alertar "critical crece" o "hay tareas muertas" sin entrar a Redis.
func PollQueueDepth(insp *asynq.Inspector, report func(queue, state string, n int)) {
	for _, q := range []string{QueueCritical, QueueDefault, QueueWebhooks, QueueLow} {
		info, err := insp.GetQueueInfo(q)
		if err != nil {
			continue // cola aún sin crear (sin tráfico): no es un error
		}
		report(q, "pending", info.Pending)
		report(q, "active", info.Active)
		report(q, "scheduled", info.Scheduled)
		report(q, "retry", info.Retry)
		report(q, "archived", info.Archived)
	}
}

// NewInspector crea el inspector de colas (misma conexión endurecida que el resto).
func NewInspector(cfg config.RedisConfig) *asynq.Inspector {
	return asynq.NewInspectorFromRedisClient(newUniversalClient(cfg, cfg.DBAsynq))
}
