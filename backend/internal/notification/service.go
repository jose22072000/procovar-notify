// Package notification orquesta el ciclo de vida de las notificaciones: el
// Service crea y encola (lado API) y el Processor envía (lado worker). El render
// del template ocurre en el worker (no en la API), de modo que el job solo
// transporta el notificationId.
package notification

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"dvtech/qbn/internal/apperr"
	"dvtech/qbn/internal/domain"
	"dvtech/qbn/internal/store"
	"dvtech/qbn/internal/store/sqlc"
	"dvtech/qbn/internal/template"
)

// Enqueuer encola el envío de una notificación. Lo implementa queue.Client; se
// define aquí (consumidor) para no acoplar el servicio a asynq. processAt cero
// = envío inmediato; futuro = envío programado.
type Enqueuer interface {
	EnqueueSend(ctx context.Context, id uuid.UUID, priority string, maxRetry int, processAt time.Time) error
}

// QuotaChecker cuenta el envío y decide si la aplicación superó su cuota.
// Opcional (nil = sin cuotas).
type QuotaChecker interface {
	Allow(ctx context.Context, appID uuid.UUID, daily, monthly int32) (bool, error)
}

// Service crea y encola notificaciones (lado API).
type Service struct {
	db     *store.DB
	enq    Enqueuer
	logger *slog.Logger
	quota  QuotaChecker
	// maxRetries es el tope de reintentos por notificación (QUEUE_RETRY_MAX).
	maxRetries int32
	// webhooks emite eventos al tenant (opcional; ver WithWebhooks).
	webhooks WebhookEnqueuer
}

// NewService construye el servicio.
func NewService(db *store.DB, enq Enqueuer, logger *slog.Logger) *Service {
	return &Service{db: db, enq: enq, logger: logger, maxRetries: 3}
}

// WithQuota fija el verificador de cuotas (encadenable). Sin él no se aplican.
func (s *Service) WithQuota(q QuotaChecker) *Service {
	s.quota = q
	return s
}

// WithMaxRetries fija el tope de reintentos por notificación (encadenable;
// QUEUE_RETRY_MAX). Valores < 1 conservan el default.
func (s *Service) WithMaxRetries(n int) *Service {
	if n >= 1 {
		s.maxRetries = int32(n)
	}
	return s
}

// WithWebhooks fija el encolador de webhooks (encadenable). Opcional: sin él,
// la ingesta de eventos no emite webhooks al tenant.
func (s *Service) WithWebhooks(w WebhookEnqueuer) *Service {
	s.webhooks = w
	return s
}

// enforceQuota comprueba la cuota de la aplicación (si hay verificador y límite).
func (s *Service) enforceQuota(ctx context.Context, appID uuid.UUID) error {
	if s.quota == nil {
		return nil
	}
	app, err := s.db.Q.GetApplication(ctx, appID)
	if err != nil {
		return apperr.Internal(err)
	}
	if app.DailySendQuota <= 0 && app.MonthlySendQuota <= 0 {
		return nil
	}
	allowed, err := s.quota.Allow(ctx, appID, app.DailySendQuota, app.MonthlySendQuota)
	if err != nil {
		return apperr.Internal(err)
	}
	if !allowed {
		return apperr.RateLimited("quota_exceeded", "Application send quota exceeded")
	}
	return nil
}

// CreateInput son los datos de entrada para crear una notificación (ya derivados
// del request HTTP, con el application_id del Caller autenticado).
type CreateInput struct {
	ApplicationID    uuid.UUID
	Type             string // id o nombre del "Tipo de notificación"; deriva canal, plantilla y destino
	TemplateKey      string // opcional: override de la plantilla del Tipo (retrocompat)
	NotificationType string
	Channel          string
	Recipient        map[string]any
	RecipientUserID  *string
	Payload          map[string]any
	Priority         string
	Locale           *string
	IdempotencyKey   *string
	ScheduledAt      *time.Time // futuro = envío programado
	Attachments      []byte     // JSON array [{filename, contentType, url|contentBase64}]; nil = sin adjuntos
}

// Create resuelve el template, valida el payload, comprueba opt-out, persiste la
// notificación (PENDING) y la encola. Idempotente por (application_id, idempotencyKey).
func (s *Service) Create(ctx context.Context, in CreateInput) (sqlc.Notification, error) {
	// 0) Tipo de notificación: si se indica, deriva nombre, canal y plantilla
	//    desde él (el destino SMTP/proveedor lo resuelve el worker por tipo+canal).
	//    templateKey/channel explícitos actúan como override (retrocompat).
	var routeRetentionDays *int32
	if in.Type != "" {
		route, err := s.resolveNotificationType(ctx, in.ApplicationID, in.Type)
		if err != nil {
			return sqlc.Notification{}, err
		}
		in.NotificationType = route.NotificationType
		if in.Channel == "" {
			in.Channel = string(route.Channel)
		}
		if in.TemplateKey == "" && route.TemplateKey != nil {
			in.TemplateKey = *route.TemplateKey
		}
		// Sin priority explícita se hereda la del Tipo (send_priority): la
		// urgencia la define el servidor, no cada llamada.
		if in.Priority == "" {
			in.Priority = string(route.SendPriority)
		}
		routeRetentionDays = route.RetentionDays
	}
	if in.TemplateKey == "" {
		return sqlc.Notification{}, apperr.Validation("missing_template",
			"no se pudo determinar la plantilla: indica un Tipo de notificación con plantilla asignada o un templateKey")
	}

	// 1) Idempotencia: si ya existe una notificación con esa clave, devolverla.
	if in.IdempotencyKey != nil && *in.IdempotencyKey != "" {
		existing, err := s.db.Q.GetNotificationByIdempotencyKey(ctx, sqlc.GetNotificationByIdempotencyKeyParams{
			ApplicationID:  in.ApplicationID,
			IdempotencyKey: in.IdempotencyKey,
		})
		if err == nil {
			// Rescate inmediato si quedó huérfana (commit sin encolar);
			// no-op si la tarea ya existe (TaskID idempotente).
			s.requeueIfStalled(ctx, existing)
			return existing, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return sqlc.Notification{}, apperr.Internal(err)
		}
	}

	// 1b) Cuota de envío de la aplicación (cuenta en Redis; 429 si se excede).
	if err := s.enforceQuota(ctx, in.ApplicationID); err != nil {
		return sqlc.Notification{}, err
	}

	// 2) Resolver template (versión activa más reciente para el locale pedido).
	locale := derefOr(in.Locale, "en")
	tmpl, err := s.resolveTemplate(ctx, in.ApplicationID, in.TemplateKey, locale)
	if err != nil {
		return sqlc.Notification{}, err
	}

	// El canal lo dicta el template (evita incoherencias cliente/template).
	channel := string(tmpl.Channel)
	if in.Channel != "" && in.Channel != channel {
		return sqlc.Notification{}, apperr.Validation("channel_mismatch",
			"channel does not match the template channel")
	}

	// 3) Validar payload contra el JSON Schema del template.
	if err := template.ValidatePayload(tmpl.RequiredVariables, in.Payload); err != nil {
		return sqlc.Notification{}, err
	}

	// 4) Opt-out: si el destinatario deshabilitó el canal, cancelar sin encolar.
	if cancelled, n, err := s.handleOptOut(ctx, in, channel); err != nil {
		return sqlc.Notification{}, err
	} else if cancelled {
		return n, nil
	}

	// 4b) Suppression list: rebotes/quejas previos cancelan sin encolar.
	if cancelled, n, err := s.handleSuppressed(ctx, in, channel); err != nil {
		return sqlc.Notification{}, err
	} else if cancelled {
		return n, nil
	}

	// 5) Persistir (PENDING) + log "created" en una transacción; commit ANTES de encolar.
	recipientJSON, _ := json.Marshal(in.Recipient)
	payloadJSON, _ := json.Marshal(in.Payload)
	version := tmpl.Version
	// Se fija el locale REALMENTE resuelto (puede diferir del pedido por el
	// fallback de idioma) para que el worker localice exactamente este template.
	locale = tmpl.Locale

	// expires_at fija la vida útil según el retention_days del Tipo: vencida,
	// el barrido de retención la borra físicamente (con intentos y logs).
	var expiresAt *time.Time
	if routeRetentionDays != nil {
		t := time.Now().AddDate(0, 0, int(*routeRetentionDays))
		expiresAt = &t
	}

	var created sqlc.Notification
	err = s.db.Tx(ctx, func(q *sqlc.Queries) error {
		n, err := q.CreateNotification(ctx, sqlc.CreateNotificationParams{
			ApplicationID:    in.ApplicationID,
			TemplateKey:      in.TemplateKey,
			TemplateVersion:  &version,
			NotificationType: in.NotificationType,
			Channel:          tmpl.Channel,
			RecipientUserID:  in.RecipientUserID,
			Recipient:        recipientJSON,
			Payload:          payloadJSON,
			Attachments:      in.Attachments,
			Locale:           &locale,
			Priority:         priorityOr(in.Priority),
			Status:           sqlc.NotificationStatusPENDING,
			IdempotencyKey:   in.IdempotencyKey,
			ScheduledAt:      toTimestamptz(in.ScheduledAt),
			ExpiresAt:        toTimestamptz(expiresAt),
			MaxRetries:       s.maxRetries,
		})
		if err != nil {
			return err
		}
		created = n
		return q.CreateNotificationLog(ctx, sqlc.CreateNotificationLogParams{
			ApplicationID:  in.ApplicationID,
			NotificationID: pgUUID(n.ID),
			Event:          "created",
		})
	})
	if err != nil {
		// Carrera de idempotencia: si otro request ganó, devolver el existente.
		if in.IdempotencyKey != nil {
			if existing, e2 := s.db.Q.GetNotificationByIdempotencyKey(ctx, sqlc.GetNotificationByIdempotencyKeyParams{
				ApplicationID: in.ApplicationID, IdempotencyKey: in.IdempotencyKey,
			}); e2 == nil {
				return existing, nil
			}
		}
		return sqlc.Notification{}, store.MapError(err, "", "notification_conflict")
	}

	// 6) Encolar tras el commit. Si está programada a futuro, se usa ProcessAt y
	// la notificación permanece PENDING (el monitor la muestra como "scheduled").
	scheduledFuture := in.ScheduledAt != nil && in.ScheduledAt.After(time.Now())
	var processAt time.Time
	if scheduledFuture {
		processAt = *in.ScheduledAt
	}
	if err := s.enq.EnqueueSend(ctx, created.ID, string(created.Priority), int(created.MaxRetries), processAt); err != nil {
		s.logger.Error("enqueue failed", slog.String("notification_id", created.ID.String()), slog.Any("error", err))
		return created, apperr.Internal(err)
	}

	if scheduledFuture {
		warnErr(s.logger, "no se pudo escribir el log de auditoría", s.db.Q.CreateNotificationLog(ctx, sqlc.CreateNotificationLogParams{
			ApplicationID: in.ApplicationID, NotificationID: pgUUID(created.ID), Event: "scheduled",
		}))
		return created, nil // status PENDING
	}

	_ = s.db.Q.SetNotificationStatusByID(ctx, sqlc.SetNotificationStatusByIDParams{
		ID: created.ID, Status: sqlc.NotificationStatusQUEUED,
	})
	warnErr(s.logger, "no se pudo escribir el log de auditoría", s.db.Q.CreateNotificationLog(ctx, sqlc.CreateNotificationLogParams{
		ApplicationID: in.ApplicationID, NotificationID: pgUUID(created.ID), Event: "queued",
	}))
	created.Status = sqlc.NotificationStatusQUEUED
	return created, nil
}

// resolveNotificationType busca el "Tipo de notificación" por id (UUID) o, si no
// es un UUID válido o no existe, por su nombre único (notification_type).
func (s *Service) resolveNotificationType(ctx context.Context, appID uuid.UUID, ref string) (sqlc.ChannelRoute, error) {
	if id, perr := uuid.Parse(ref); perr == nil {
		route, err := s.db.Q.GetChannelRouteByID(ctx, sqlc.GetChannelRouteByIDParams{ID: id, ApplicationID: appID})
		if err == nil {
			return route, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return sqlc.ChannelRoute{}, apperr.Internal(err)
		}
		// UUID válido pero sin coincidencia por id: se intenta por nombre.
	}
	route, err := s.db.Q.GetChannelRouteByName(ctx, sqlc.GetChannelRouteByNameParams{
		ApplicationID: appID, NotificationType: ref,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return sqlc.ChannelRoute{}, apperr.NotFound("notification_type_not_found", "Tipo de notificación no encontrado: "+ref)
		}
		return sqlc.ChannelRoute{}, apperr.Internal(err)
	}
	return route, nil
}

// resolveTemplate busca la versión activa para el locale pedido aplicando el
// fallback de idioma (§16): locale solicitado → "en" → cualquier variante activa
// con esa key. Devuelve el template realmente resuelto (con su locale/versión).
func (s *Service) resolveTemplate(ctx context.Context, appID uuid.UUID, key, locale string) (sqlc.Template, error) {
	locales := []string{locale}
	if locale != defaultLocale {
		locales = append(locales, defaultLocale)
	}
	for _, loc := range locales {
		tmpl, err := s.db.Q.GetLatestActiveTemplate(ctx, sqlc.GetLatestActiveTemplateParams{
			ApplicationID: appID, Key: key, Locale: loc,
		})
		if err == nil {
			return tmpl, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return sqlc.Template{}, apperr.Internal(err)
		}
	}
	// Último recurso: cualquier variante activa con esa key.
	all, err := s.db.Q.ListActiveTemplates(ctx, appID)
	if err != nil {
		return sqlc.Template{}, apperr.Internal(err)
	}
	for _, t := range all {
		if t.Key == key {
			return t, nil
		}
	}
	return sqlc.Template{}, apperr.NotFound("template_not_found", "Template not found: "+key)
}

// handleOptOut comprueba las preferencias del usuario destinatario. Si el canal
// está deshabilitado, crea la notificación como CANCELLED (con log) y no encola.
func (s *Service) handleOptOut(ctx context.Context, in CreateInput, channel string) (bool, sqlc.Notification, error) {
	if in.RecipientUserID == nil || *in.RecipientUserID == "" {
		return false, sqlc.Notification{}, nil
	}
	pref, err := s.db.Q.GetUserPreference(ctx, sqlc.GetUserPreferenceParams{
		ApplicationID: in.ApplicationID, UserID: *in.RecipientUserID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, sqlc.Notification{}, nil // sin preferencias => por defecto habilitado
	}
	if err != nil {
		return false, sqlc.Notification{}, apperr.Internal(err)
	}
	if channelEnabled(pref, channel) {
		return false, sqlc.Notification{}, nil
	}

	recipientJSON, _ := json.Marshal(in.Recipient)
	payloadJSON, _ := json.Marshal(in.Payload)
	locale := derefOr(in.Locale, "en")
	n, err := s.db.Q.CreateNotification(ctx, sqlc.CreateNotificationParams{
		ApplicationID:    in.ApplicationID,
		TemplateKey:      in.TemplateKey,
		NotificationType: in.NotificationType,
		Channel:          sqlc.Channel(channel),
		RecipientUserID:  in.RecipientUserID,
		Recipient:        recipientJSON,
		Payload:          payloadJSON,
		Locale:           &locale,
		Priority:         priorityOr(in.Priority),
		Status:           sqlc.NotificationStatusCANCELLED,
		IdempotencyKey:   in.IdempotencyKey,
		MaxRetries:       s.maxRetries,
	})
	if err != nil {
		return false, sqlc.Notification{}, store.MapError(err, "", "notification_conflict")
	}
	warnErr(s.logger, "no se pudo escribir el log de auditoría", s.db.Q.CreateNotificationLog(ctx, sqlc.CreateNotificationLogParams{
		ApplicationID:  in.ApplicationID,
		NotificationID: pgUUID(n.ID),
		Event:          "cancelled",
		Message:        ptr("recipient opted out of channel " + channel),
	}))
	return true, n, nil
}

func channelEnabled(p sqlc.UserNotificationPreference, channel string) bool {
	switch channel {
	case domain.ChannelEmail:
		return p.EmailEnabled
	case domain.ChannelPush:
		return p.PushEnabled
	case domain.ChannelSMS:
		return p.SmsEnabled
	case domain.ChannelInApp:
		return p.InAppEnabled
	default:
		return true
	}
}
