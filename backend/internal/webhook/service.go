// Package webhook entrega notificaciones de estado (sent/failed/...) a los
// endpoints que cada tenant configura, con cuerpo JSON firmado por HMAC.
package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"dvtech/qbn/internal/crypto"
	"dvtech/qbn/internal/queue"
	"dvtech/qbn/internal/safedial"
	"dvtech/qbn/internal/store"
	"dvtech/qbn/internal/store/sqlc"
)

// Service entrega los webhooks. Entrega "al menos una vez": ante un fallo
// parcial, asynq reintenta y puede re-entregar a endpoints ya notificados, por
// lo que el consumidor debe ser idempotente (cabecera X-QBN-Event + id).
type Service struct {
	db     *store.DB
	enc    *crypto.Encryptor
	http   *http.Client
	logger *slog.Logger
	now    func() time.Time
}

// NewService crea el servicio de webhooks.
func NewService(db *store.DB, enc *crypto.Encryptor, logger *slog.Logger) *Service {
	return &Service{
		db:  db,
		enc: enc,
		// Guard anti-SSRF: el endpoint lo elige el tenant; bloquea destinos
		// internos (loopback/privados/link-local), incluidos los redirects.
		http:   safedial.NewClient(15 * time.Second),
		logger: logger,
		now:    time.Now,
	}
}

type eventBody struct {
	Event        string `json:"event"`
	Notification struct {
		ID               string `json:"id"`
		Status           string `json:"status"`
		Channel          string `json:"channel"`
		NotificationType string `json:"notificationType"`
		RecipientUserID  string `json:"recipientUserId,omitempty"`
		CreatedAt        string `json:"createdAt"`
	} `json:"notification"`
	Timestamp int64 `json:"timestamp"`
}

// Deliver entrega el evento de una notificación a todos los endpoints activos de
// la aplicación suscritos a ese evento. Devuelve error si algún POST falla, para
// que asynq reintente.
func (s *Service) Deliver(ctx context.Context, p queue.WebhookPayload) error {
	endpoints, err := s.db.Q.ListActiveWebhookEndpoints(ctx, p.ApplicationID)
	if err != nil {
		return err
	}
	if len(endpoints) == 0 {
		return nil
	}
	n, err := s.db.Q.GetNotificationByID(ctx, p.NotificationID)
	if err != nil {
		return err
	}

	body := eventBody{Event: p.Event, Timestamp: s.now().Unix()}
	body.Notification.ID = n.ID.String()
	body.Notification.Status = string(n.Status)
	body.Notification.Channel = string(n.Channel)
	body.Notification.NotificationType = n.NotificationType
	if n.RecipientUserID != nil {
		body.Notification.RecipientUserID = *n.RecipientUserID
	}
	body.Notification.CreatedAt = n.CreatedAt.Time.Format(time.RFC3339)
	raw, _ := json.Marshal(body)

	var firstErr error
	for _, ep := range endpoints {
		if !subscribed(ep.Events, p.Event) {
			continue
		}
		if err := s.post(ctx, ep, raw, body.Timestamp); err != nil {
			s.logger.Warn("webhook delivery failed",
				slog.String("endpoint", ep.ID.String()), slog.String("event", p.Event), slog.Any("error", err))
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// post firma y envía el cuerpo a un endpoint. La firma es
// HMAC-SHA256(secret, "<timestamp>.<body>") en hex.
func (s *Service) post(ctx context.Context, ep sqlc.WebhookEndpoint, body []byte, ts int64) error {
	secret, err := s.enc.Decrypt(ep.SecretEnc)
	if err != nil {
		return fmt.Errorf("descifrando secreto: %w", err)
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(strconv.FormatInt(ts, 10)))
	mac.Write([]byte("."))
	mac.Write(body)
	sig := hex.EncodeToString(mac.Sum(nil))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ep.Url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-QBN-Event", string(extractEvent(body)))
	req.Header.Set("X-QBN-Timestamp", strconv.FormatInt(ts, 10))
	req.Header.Set("X-QBN-Signature", sig)

	res, err := s.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("respuesta %d del endpoint", res.StatusCode)
	}
	return nil
}

// subscribed indica si el endpoint debe recibir el evento: lista vacía = todos.
func subscribed(events []string, event string) bool {
	if len(events) == 0 {
		return true
	}
	for _, e := range events {
		if e == event {
			return true
		}
	}
	return false
}

func extractEvent(body []byte) string {
	var b struct {
		Event string `json:"event"`
	}
	_ = json.Unmarshal(body, &b)
	return b.Event
}

// Eventos de webhook conocidos (taxonomía recurso.estado).
const (
	EventSent   = "notification.sent"
	EventFailed = "notification.failed"
)
