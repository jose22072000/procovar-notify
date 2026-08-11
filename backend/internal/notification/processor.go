package notification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/sony/gobreaker"

	"dvtech/qbn/internal/apperr"
	"dvtech/qbn/internal/channels"
	"dvtech/qbn/internal/crypto"
	"dvtech/qbn/internal/domain"
	"dvtech/qbn/internal/metrics"
	"dvtech/qbn/internal/observability"
	"dvtech/qbn/internal/store"
	"dvtech/qbn/internal/store/sqlc"
	"dvtech/qbn/internal/template"
)

// WebhookEnqueuer encola la entrega de un webhook de estado al tenant. Lo
// implementa el cliente de cola; opcional (nil = sin webhooks).
type WebhookEnqueuer interface {
	EnqueueWebhook(ctx context.Context, applicationID, notificationID uuid.UUID, event string) error
}

// Processor ejecuta el envío de una notificación (lado worker).
type Processor struct {
	db         *store.DB
	dispatcher *channels.Dispatcher
	enc        *crypto.Encryptor
	logger     *slog.Logger
	breakers   *breakerRegistry
	webhooks   WebhookEnqueuer
	// attempt obtiene (nº de reintento, máximo, taskID) del contexto. Por defecto
	// lee el contexto de asynq; es un seam inyectable (el contexto interno de
	// asynq no es construible fuera de su paquete, así que los tests lo sustituyen
	// para ejercitar las ramas RETRY/breaker/failPermanent).
	attempt func(context.Context) (retry, max int, taskID string)
	// Cachés de la ruta caliente (ver cache.go): la mayoría de envíos repiten
	// exactamente la misma ruta/credencial/plantilla.
	routeCache *ttlCache[[]resolvedTarget]
	tmplCache  *ttlCache[sqlc.Template]
}

// NewProcessor construye el procesador.
func NewProcessor(db *store.DB, dispatcher *channels.Dispatcher, enc *crypto.Encryptor, logger *slog.Logger) *Processor {
	return &Processor{
		db: db, dispatcher: dispatcher, enc: enc, logger: logger,
		breakers:   newBreakerRegistry(),
		attempt:    attemptInfo,
		routeCache: newTTLCache[[]resolvedTarget](),
		tmplCache:  newTTLCache[sqlc.Template](),
	}
}

// WithAttemptInfo sustituye la fuente de metadatos de intento (reintento/máximo/
// taskID). Encadenable; pensado para inyectar valores en tests.
func (p *Processor) WithAttemptInfo(fn func(context.Context) (retry, max int, taskID string)) *Processor {
	p.attempt = fn
	return p
}

// WithWebhooks fija el encolador de webhooks (encadenable). Si no se llama, el
// processor no emite webhooks.
func (p *Processor) WithWebhooks(w WebhookEnqueuer) *Processor {
	p.webhooks = w
	return p
}

// emitWebhook encola (best-effort) un evento de webhook; los fallos solo se
// registran (la entrega y sus reintentos viven en la tarea de webhook).
func (p *Processor) emitWebhook(ctx context.Context, appID, notifID uuid.UUID, event string) {
	if p.webhooks == nil {
		return
	}
	if err := p.webhooks.EnqueueWebhook(ctx, appID, notifID, event); err != nil {
		p.logger.Warn("enqueue webhook failed", slog.String("event", event), slog.Any("error", err))
	}
}

// resolvedTarget es una ruta resuelta lista para enviar, con la clave de su
// circuit breaker (conexión/proveedor) y el tipo de proveedor para métricas
// (enum de baja cardinalidad: smtp, twilio, fcm, apns, inapp, inbox).
type resolvedTarget struct {
	route    channels.ResolvedRoute
	key      string
	provider string
}

// transientError marca un fallo recuperable (infra) para que el envío se
// reintente en vez de fallar permanentemente (L2).
type transientError struct{ err error }

func (e *transientError) Error() string { return e.err.Error() }
func (e *transientError) Unwrap() error { return e.err }

func isTransient(err error) bool {
	var t *transientError
	return errors.As(err, &t)
}

// ProcessSend implementa queue.Processor: carga la notificación, resuelve la
// ruta, renderiza y envía, y actualiza estado + intentos + logs.
func (p *Processor) ProcessSend(ctx context.Context, id uuid.UUID) error {
	n, err := p.db.Q.GetNotificationByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		p.logger.Warn("notification not found, skipping", slog.String("notification_id", id.String()))
		return asynq.SkipRetry
	}
	if err != nil {
		return apperr.Internal(err) // transitorio: que asynq reintente
	}

	// Estados terminales o cancelados: nada que hacer (idempotencia del worker).
	switch string(n.Status) {
	case domain.StatusSent, domain.StatusDelivered, domain.StatusRead, domain.StatusCancelled:
		return nil
	}

	// Guardia anti-duplicado: con at-least-once, una reentrega tras crash
	// post-envío (PROCESSING no terminal) reenviaría de verdad. Si ya hay un
	// intento SUCCESS, el mensaje llegó: solo falta marcar SENT.
	if sentAlready, err := p.db.Q.HasSuccessfulDeliveryAttempt(ctx, id); err == nil && sentAlready {
		p.warnIf("mark_sent_reconciled", p.db.Q.MarkNotificationSent(ctx, id))
		p.log(ctx, n.ApplicationID, id, "sent", "reconciliada: intento SUCCESS previo, no se reenvía")
		p.logger.Info("redelivery reconciled without resend", slog.String("notification_id", id.String()))
		return nil
	}

	start := time.Now()
	retryCount, maxRetry, taskID := p.attempt(ctx)
	attemptNumber := int32(retryCount + 1)

	logger := p.logger.With(
		slog.String("notification_id", id.String()),
		slog.Int("attempt", int(attemptNumber)),
	)
	// Correlación con la petición HTTP de origen (viaja en el payload de la tarea).
	if rid := observability.RequestIDFromContext(ctx); rid != "" {
		logger = logger.With(slog.String("request_id", rid))
	}

	p.warnIf("set_processing", p.db.Q.SetNotificationStatusByID(ctx, sqlc.SetNotificationStatusByIDParams{
		ID: id, Status: sqlc.NotificationStatusPROCESSING,
	}))
	p.log(ctx, n.ApplicationID, id, "processing", "")

	attempt, err := p.db.Q.CreateDeliveryAttempt(ctx, sqlc.CreateDeliveryAttemptParams{
		NotificationID: id,
		AttemptNumber:  attemptNumber,
		AsynqTaskID:    strOrNil(taskID),
	})
	if err != nil {
		return apperr.Internal(err)
	}

	// Resolución de rutas: un error de configuración no se reintenta, pero uno
	// transitorio (DB/descifrado caídos) sí, para no fallar permanentemente por
	// un bache de infra (L2).
	targets, err := p.resolveRoutes(ctx, n)
	if err != nil {
		if isTransient(err) {
			logger.Warn("route resolution transient error, will retry", slog.Any("error", err))
			return err // asynq reintenta con backoff
		}
		return p.failPermanent(ctx, n, attempt.ID, "route_error", err, attemptNumber, logger)
	}
	msg, err := p.renderMessage(ctx, n)
	if err != nil {
		return p.failPermanent(ctx, n, attempt.ID, "render_error", err, attemptNumber, logger)
	}

	// Envío por la ruta del Tipo (modelo 1:1, sin fallback) a través de su
	// circuit breaker. Con el breaker abierto el error se clasifica como
	// Unavailable: se reintenta con retardo sin consumir intentos (ver
	// handleSendFailure), porque un destino caído no debe perder mensajes.
	tgt := targets[0]
	br := p.breakers.get(tgt.key)
	res, sendErr := br.Execute(func() (any, error) {
		return p.dispatcher.Send(ctx, string(n.Channel), msg, tgt.route)
	})
	if errors.Is(sendErr, gobreaker.ErrOpenState) || errors.Is(sendErr, gobreaker.ErrTooManyRequests) {
		sendErr = channels.Unavailable(sendErr)
	}
	if sendErr != nil {
		return p.handleSendFailure(ctx, n, attempt.ID, sendErr, tgt.provider, attemptNumber, retryCount, maxRetry, logger)
	}
	providerRef, _ := res.(channels.ProviderRef)

	// Éxito.
	ref := string(providerRef)
	p.warnIf("finish_attempt_success", p.db.Q.FinishDeliveryAttempt(ctx, sqlc.FinishDeliveryAttemptParams{
		ID: attempt.ID, Status: sqlc.DeliveryStatusSUCCESS, ProviderRef: &ref,
	}))
	p.warnIf("mark_sent", p.db.Q.MarkNotificationSent(ctx, id))
	p.log(ctx, n.ApplicationID, id, "sent", "provider_ref="+ref)
	// Despacho + extremo a extremo (incluida la espera en cola: la latencia
	// que sufre el usuario) con su prioridad — el SLA real de los urgentes.
	metrics.RecordSent(string(n.Channel), tgt.provider, time.Since(start), time.Since(n.CreatedAt.Time), string(n.Priority))
	p.emitWebhook(ctx, n.ApplicationID, id, "notification.sent")
	logger.Info("notification sent", slog.String("provider_ref", ref))
	return nil
}

// handleSendFailure registra el fallo y decide reintento vs fallo definitivo.
func (p *Processor) handleSendFailure(ctx context.Context, n sqlc.Notification, attemptID uuid.UUID, sendErr error, provider string, attemptNumber int32, retryCount, maxRetry int, logger *slog.Logger) error {
	msg := sendErr.Error()

	// Destino caído (breaker abierto): la notificación queda QUEUED esperando
	// la recuperación. El error Unavailable no cuenta como fallo para asynq
	// (IsFailure) y se reintenta con retardo (RetryDelayFunc) — sin consumir
	// reintentos ni acabar en FAILED. La edad delata retenciones largas.
	if channels.IsUnavailable(sendErr) {
		p.warnIf("finish_attempt_unavailable", p.db.Q.FinishDeliveryAttempt(ctx, sqlc.FinishDeliveryAttemptParams{
			ID: attemptID, Status: sqlc.DeliveryStatusRETRY,
			ErrorCode: strOrNil("provider_unavailable"), ErrorMessage: &msg,
		}))
		p.warnIf("mark_retry", p.db.Q.MarkNotificationRetry(ctx, sqlc.MarkNotificationRetryParams{ID: n.ID, RetryCount: int32(retryCount)}))
		p.log(ctx, n.ApplicationID, n.ID, "retry", "provider_unavailable: "+msg)
		metrics.RecordRetry("provider_unavailable")
		age := time.Since(n.CreatedAt.Time)
		if age > time.Hour {
			logger.Error("provider unavailable, notification held", slog.Duration("age", age), slog.Any("error", sendErr))
		} else {
			logger.Warn("provider unavailable, will retry after delay", slog.Duration("age", age), slog.Any("error", sendErr))
		}
		return sendErr
	}

	willRetry := retryCount < maxRetry

	// El intento se marca RETRY si habrá otro, o FAILED si es el último.
	attemptStatus := sqlc.DeliveryStatusFAILED
	if willRetry {
		attemptStatus = sqlc.DeliveryStatusRETRY
	}
	p.warnIf("finish_attempt", p.db.Q.FinishDeliveryAttempt(ctx, sqlc.FinishDeliveryAttemptParams{
		ID: attemptID, Status: attemptStatus,
		ErrorCode: strOrNil("send_error"), ErrorMessage: &msg,
	}))

	if willRetry {
		p.warnIf("mark_retry", p.db.Q.MarkNotificationRetry(ctx, sqlc.MarkNotificationRetryParams{ID: n.ID, RetryCount: attemptNumber}))
		p.log(ctx, n.ApplicationID, n.ID, "retry", msg)
		metrics.RecordRetry("send_error")
		logger.Warn("send failed, will retry", slog.Any("error", sendErr))
		return sendErr // asynq reintenta con backoff
	}

	p.warnIf("mark_failed", p.db.Q.MarkNotificationFailed(ctx, sqlc.MarkNotificationFailedParams{ID: n.ID, RetryCount: attemptNumber}))
	p.log(ctx, n.ApplicationID, n.ID, "failed", msg)
	metrics.RecordFailed(string(n.Channel), provider, "send_error")
	p.emitWebhook(ctx, n.ApplicationID, n.ID, "notification.failed")
	logger.Error("send failed permanently", slog.Any("error", sendErr))
	return sendErr // asynq archiva
}

// failPermanent marca un fallo no recuperable (config) sin reintentos.
func (p *Processor) failPermanent(ctx context.Context, n sqlc.Notification, attemptID uuid.UUID, code string, cause error, attemptNumber int32, logger *slog.Logger) error {
	msg := cause.Error()
	p.warnIf("finish_attempt_failed", p.db.Q.FinishDeliveryAttempt(ctx, sqlc.FinishDeliveryAttemptParams{
		ID: attemptID, Status: sqlc.DeliveryStatusFAILED,
		ErrorCode: &code, ErrorMessage: &msg,
	}))
	p.warnIf("mark_failed", p.db.Q.MarkNotificationFailed(ctx, sqlc.MarkNotificationFailedParams{ID: n.ID, RetryCount: attemptNumber}))
	p.log(ctx, n.ApplicationID, n.ID, "failed", code+": "+msg)
	metrics.RecordFailed(string(n.Channel), "none", code)
	p.emitWebhook(ctx, n.ApplicationID, n.ID, "notification.failed")
	logger.Error("permanent failure", slog.String("code", code), slog.Any("error", cause))
	return asynq.SkipRetry
}

// resolveRoutes obtiene TODAS las rutas para (tipo, canal) ordenadas para
// fallback. Para EMAIL resuelve y descifra cada conexión SMTP; salta rutas que
// no se puedan resolver. Devuelve cada ruta con la clave de su circuit breaker.
func (p *Processor) resolveRoutes(ctx context.Context, n sqlc.Notification) ([]resolvedTarget, error) {
	// Ruta + credencial descifrada cacheadas: son idénticas para todos los
	// envíos de un (app, tipo, canal) y costaban 2+ queries y un descifrado
	// AES por notificación. Solo se cachean resoluciones con éxito.
	cacheKey := n.ApplicationID.String() + "|" + n.NotificationType + "|" + string(n.Channel)
	if targets, ok := p.routeCache.get(cacheKey); ok {
		return targets, nil
	}
	targets, err := p.resolveRoutesUncached(ctx, n)
	if err != nil {
		return nil, err
	}
	p.routeCache.set(cacheKey, targets, routeCacheTTL)
	return targets, nil
}

func (p *Processor) resolveRoutesUncached(ctx context.Context, n sqlc.Notification) ([]resolvedTarget, error) {
	rows, err := p.db.Q.ListRoutesForResolution(ctx, sqlc.ListRoutesForResolutionParams{
		ApplicationID:    n.ApplicationID,
		NotificationType: n.NotificationType,
		Channel:          n.Channel,
	})
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, errors.New("no hay ruta configurada para el tipo/canal")
	}

	var targets []resolvedTarget
	var lastTransient error // un fallo de infra al resolver: clasifica el global como retryable
	for _, route := range rows {
		rr := channels.ResolvedRoute{Channel: string(n.Channel)}
		var key, provider string

		if n.Channel == sqlc.ChannelEMAIL {
			if !route.SmtpConnectionID.Valid {
				p.logger.Warn("ruta EMAIL sin conexión SMTP, se salta", slog.String("route_id", route.ID.String()))
				continue
			}
			smtpID := uuid.UUID(route.SmtpConnectionID.Bytes)
			smtp, err := p.db.Q.GetSmtpConnection(ctx, sqlc.GetSmtpConnectionParams{ID: smtpID, ApplicationID: n.ApplicationID})
			if err != nil {
				if !errors.Is(err, pgx.ErrNoRows) {
					lastTransient = err // error de infra (no "no existe"): reintentar
				}
				p.logger.Warn("no se pudo cargar la conexión SMTP, se salta", slog.String("smtp_id", smtpID.String()), slog.Any("error", err))
				continue
			}
			password, err := p.enc.Decrypt(smtp.PasswordEnc)
			if err != nil {
				// Descifrado fallido = dato/clave corruptos: permanente, no reintentar.
				p.logger.Warn("no se pudo descifrar la contraseña SMTP, se salta", slog.String("smtp_id", smtpID.String()), slog.Any("error", err))
				continue
			}
			rr.SMTP = &channels.SMTPConfig{
				Host: smtp.Host, Port: int(smtp.Port),
				Username: smtp.Username, Password: string(password),
				FromEmail: smtp.FromEmail, FromName: smtp.FromName, Secure: smtp.Secure,
			}
			key = "smtp:" + smtpID.String()
			provider = "smtp"
		} else if n.Channel == sqlc.ChannelPUSH || n.Channel == sqlc.ChannelSMS {
			if !route.ChannelProviderID.Valid {
				// PUSH admite no llevar proveedor: se entrega como bandeja interna
				// (no-op) hasta que se configure FCM/APNS; la app la lee mientras
				// está abierta. SMS sí exige proveedor.
				if n.Channel != sqlc.ChannelPUSH {
					p.logger.Warn("ruta SMS sin proveedor, se salta", slog.String("route_id", route.ID.String()))
					continue
				}
				key = "channel:PUSH"
				provider = "inbox" // push sin proveedor: entrega como bandeja interna
			} else {
				// Se carga y descifra la config del proveedor (Push con FCM/APNS o SMS).
				provID := uuid.UUID(route.ChannelProviderID.Bytes)
				prov, err := p.db.Q.GetChannelProvider(ctx, sqlc.GetChannelProviderParams{ID: provID, ApplicationID: n.ApplicationID})
				if err != nil {
					if !errors.Is(err, pgx.ErrNoRows) {
						lastTransient = err // error de infra: reintentar
					}
					p.logger.Warn("no se pudo cargar el proveedor, se salta", slog.String("provider_id", provID.String()), slog.Any("error", err))
					continue
				}
				cfgRaw, err := p.enc.Decrypt(prov.ConfigEnc)
				if err != nil {
					// Descifrado fallido = permanente, no reintentar.
					p.logger.Warn("no se pudo descifrar la config del proveedor, se salta", slog.String("provider_id", provID.String()), slog.Any("error", err))
					continue
				}
				var cfg map[string]any
				if err := json.Unmarshal(cfgRaw, &cfg); err != nil {
					p.logger.Warn("config del proveedor no es JSON, se salta", slog.String("provider_id", provID.String()), slog.Any("error", err))
					continue
				}
				rr.Provider = &channels.ProviderConfig{Provider: prov.Provider, Config: cfg}
				key = "provider:" + provID.String()
				provider = prov.Provider
			}
		} else {
			key = "channel:" + string(n.Channel)
			provider = "inapp"
		}

		targets = append(targets, resolvedTarget{route: rr, key: key, provider: provider})
	}
	if len(targets) == 0 {
		// Si todas las rutas se saltaron por un fallo de infra (no por mala
		// configuración), clasifica el error como transitorio para que se reintente.
		if lastTransient != nil {
			return nil, &transientError{err: fmt.Errorf("rutas no resueltas por error transitorio: %w", lastTransient)}
		}
		return nil, errors.New("ninguna ruta utilizable para el tipo/canal")
	}
	return targets, nil
}

// defaultLocale es el idioma de respaldo cuando no existe la variante solicitada.
const defaultLocale = "en"

// renderMessage resuelve el template (versión fijada en la notificación) y
// renderiza subject y body con el payload.
func (p *Processor) renderMessage(ctx context.Context, n sqlc.Notification) (channels.RenderedMessage, error) {
	tmpl, err := p.resolveTemplate(ctx, n)
	if err != nil {
		return channels.RenderedMessage{}, err
	}

	var payload map[string]any
	if len(n.Payload) > 0 {
		if err := json.Unmarshal(n.Payload, &payload); err != nil {
			p.logger.Warn("payload JSON inválido; se renderiza con payload vacío", "notification_id", n.ID, "error", err)
		}
	}

	subject, err := template.Render(derefOr(tmpl.Subject, ""), payload)
	if err != nil {
		return channels.RenderedMessage{}, err
	}
	body, err := template.Render(tmpl.Body, payload)
	if err != nil {
		return channels.RenderedMessage{}, err
	}

	var rcpt struct {
		Email     string `json:"email"`
		Name      string `json:"name"`
		Phone     string `json:"phone"`
		PushToken string `json:"push_token"`
	}
	if err := json.Unmarshal(n.Recipient, &rcpt); err != nil {
		p.logger.Warn("recipient JSON inválido; destinatario incompleto", "notification_id", n.ID, "error", err)
	}

	var attachments []channels.Attachment
	if len(n.Attachments) > 0 {
		var specs []attachmentSpec
		if err := json.Unmarshal(n.Attachments, &specs); err != nil {
			return channels.RenderedMessage{}, apperr.Validation("invalid_attachments", "attachments inválidos")
		}
		if attachments, err = resolveAttachments(ctx, specs); err != nil {
			return channels.RenderedMessage{}, err
		}
	}

	return channels.RenderedMessage{
		Subject:  subject,
		HTMLBody: body,
		Recipient: channels.Recipient{
			Email: rcpt.Email, Name: rcpt.Name, Phone: rcpt.Phone, PushToken: rcpt.PushToken,
		},
		Attachments: attachments,
	}, nil
}

// resolveTemplate localiza el template a usar aplicando fallback de idioma
// (§16 i18n). Si la versión está fijada, se respeta el locale exacto. Para la
// última versión activa, la cadena es: locale solicitado → defaultLocale →
// cualquier variante activa con esa key.
func (p *Processor) resolveTemplate(ctx context.Context, n sqlc.Notification) (sqlc.Template, error) {
	locale := derefOr(n.Locale, defaultLocale)

	// Versión fijada: el locale forma parte de la identidad, no se hace fallback.
	// Una versión concreta es inmutable, así que se cachea con TTL largo.
	if n.TemplateVersion != nil {
		key := "v|" + n.ApplicationID.String() + "|" + n.TemplateKey + "|" + locale + "|" + strconv.Itoa(int(*n.TemplateVersion))
		if t, ok := p.tmplCache.get(key); ok {
			return t, nil
		}
		t, err := p.db.Q.GetTemplateByVersion(ctx, sqlc.GetTemplateByVersionParams{
			ApplicationID: n.ApplicationID, Key: n.TemplateKey, Locale: locale, Version: *n.TemplateVersion,
		})
		if err != nil {
			return sqlc.Template{}, err
		}
		p.tmplCache.set(key, t, templatePinnedTTL)
		return t, nil
	}

	// "Última versión activa" cambia al editar: TTL corto.
	latestKey := "l|" + n.ApplicationID.String() + "|" + n.TemplateKey + "|" + locale
	if t, ok := p.tmplCache.get(latestKey); ok {
		return t, nil
	}
	t, err := p.resolveLatestTemplate(ctx, n, locale)
	if err != nil {
		return sqlc.Template{}, err
	}
	p.tmplCache.set(latestKey, t, templateLatestTTL)
	return t, nil
}

// resolveLatestTemplate aplica la cadena de fallback de idioma (§16) sobre las
// versiones activas: locale pedido → default → cualquier variante activa.
func (p *Processor) resolveLatestTemplate(ctx context.Context, n sqlc.Notification, locale string) (sqlc.Template, error) {

	// 1) Locale solicitado y 2) locale por defecto.
	locales := []string{locale}
	if locale != defaultLocale {
		locales = append(locales, defaultLocale)
	}
	for _, loc := range locales {
		tmpl, err := p.db.Q.GetLatestActiveTemplate(ctx, sqlc.GetLatestActiveTemplateParams{
			ApplicationID: n.ApplicationID, Key: n.TemplateKey, Locale: loc,
		})
		if err == nil {
			return tmpl, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return sqlc.Template{}, err
		}
	}

	// 3) Cualquier variante activa con esa key (último recurso).
	all, err := p.db.Q.ListActiveTemplates(ctx, n.ApplicationID)
	if err != nil {
		return sqlc.Template{}, err
	}
	for _, t := range all {
		if t.Key == n.TemplateKey {
			return t, nil
		}
	}
	return sqlc.Template{}, pgx.ErrNoRows
}

// warnIf registra un aviso si una escritura de estado best-effort falla. Estas
// escrituras no abortan el envío, pero un fallo silencioso dejaría la fila en un
// estado intermedio (p. ej. PROCESSING) sin rastro; al menos queda el aviso (L3).
func (p *Processor) warnIf(op string, err error) {
	if err != nil {
		p.logger.Warn("best-effort state write failed", slog.String("op", op), slog.Any("error", err))
	}
}

// log escribe un evento de auditoría (best-effort).
func (p *Processor) log(ctx context.Context, appID, notifID uuid.UUID, event, message string) {
	warnErr(p.logger, "no se pudo escribir el log de auditoría", p.db.Q.CreateNotificationLog(ctx, sqlc.CreateNotificationLogParams{
		ApplicationID:  appID,
		NotificationID: pgUUID(notifID),
		Event:          event,
		Message:        strOrNil(message),
	}))
}

// attemptInfo extrae del contexto de asynq el nº de reintento, el máximo y el id.
func attemptInfo(ctx context.Context) (retry, max int, taskID string) {
	retry, _ = asynq.GetRetryCount(ctx)
	max, _ = asynq.GetMaxRetry(ctx)
	taskID, _ = asynq.GetTaskID(ctx)
	return retry, max, taskID
}

func strOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
