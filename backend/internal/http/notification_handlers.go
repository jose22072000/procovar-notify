package http

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"

	"dvtech/qbn/internal/apperr"
	"dvtech/qbn/internal/auth"
	"dvtech/qbn/internal/httpx"
	"dvtech/qbn/internal/notification"
)

// NotificationHandler expone los endpoints de notificaciones de la API pública.
type NotificationHandler struct {
	svc *notification.Service
}

// NewNotificationHandler crea el handler.
func NewNotificationHandler(svc *notification.Service) *NotificationHandler {
	return &NotificationHandler{svc: svc}
}

type createNotificationRequest struct {
	Type             string              `json:"type"`
	TemplateKey      string              `json:"templateKey"`
	NotificationType string              `json:"notificationType"`
	Channel          string              `json:"channel"`
	Recipient        map[string]any      `json:"recipient"`
	RecipientUserID  *string             `json:"recipientUserId"`
	Payload          map[string]any      `json:"payload"`
	Priority         string              `json:"priority"`
	Locale           *string             `json:"locale"`
	IdempotencyKey   *string             `json:"idempotencyKey"`
	ScheduledAt      *string             `json:"scheduledAt"`
	Attachments      []attachmentRequest `json:"attachments"`
}

// attachmentRequest es un adjunto declarado: inline (base64) o por URL.
type attachmentRequest struct {
	Filename      string `json:"filename"`
	ContentType   string `json:"contentType"`
	URL           string `json:"url"`
	ContentBase64 string `json:"contentBase64"`
}

// Create: POST /v1/notifications — crea y encola una notificación.
func (h *NotificationHandler) Create(w http.ResponseWriter, r *http.Request) {
	caller, ok := auth.CallerFromContext(r.Context())
	if !ok {
		httpx.WriteProblem(w, r, apperr.Unauthorized("unauthenticated", "Authentication required"))
		return
	}

	var req createNotificationRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	// El envío se identifica por el Tipo de notificación (type = id o nombre), que
	// aporta canal, plantilla y destino. Se admite el modo legado templateKey +
	// notificationType para retrocompatibilidad.
	if req.Type == "" && (req.TemplateKey == "" || req.NotificationType == "") {
		httpx.WriteProblem(w, r, apperr.Validation("missing_fields", "type (o templateKey + notificationType) es obligatorio"))
		return
	}

	var scheduledAt *time.Time
	if req.ScheduledAt != nil && *req.ScheduledAt != "" {
		t, perr := time.Parse(time.RFC3339, *req.ScheduledAt)
		if perr != nil {
			httpx.WriteProblem(w, r, apperr.Validation("invalid_scheduled_at", "scheduledAt must be RFC3339"))
			return
		}
		scheduledAt = &t
	}

	var attachments []byte
	if len(req.Attachments) > 0 {
		attachments, _ = json.Marshal(req.Attachments)
	}

	n, err := h.svc.Create(r.Context(), notification.CreateInput{
		ApplicationID:    caller.ApplicationID,
		Type:             req.Type,
		TemplateKey:      req.TemplateKey,
		NotificationType: req.NotificationType,
		Channel:          req.Channel,
		Recipient:        req.Recipient,
		RecipientUserID:  req.RecipientUserID,
		Payload:          req.Payload,
		Priority:         req.Priority,
		Locale:           req.Locale,
		IdempotencyKey:   req.IdempotencyKey,
		ScheduledAt:      scheduledAt,
		Attachments:      attachments,
	})
	if err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}

	httpx.WriteJSON(w, http.StatusAccepted, map[string]any{
		"id":     n.ID.String(),
		"status": string(n.Status),
	})
}

type ingestEventRequest struct {
	Type           string  `json:"type"`
	Channel        string  `json:"channel"`
	Recipient      string  `json:"recipient"`
	NotificationID *string `json:"notificationId"`
	Reason         string  `json:"reason"`
}

// IngestEvent: POST /v1/events — el tenant (o su adaptador del proveedor)
// reenvía eventos normalizados (bounce/complaint/delivered) para suprimir
// destinatarios o confirmar entregas.
func (h *NotificationHandler) IngestEvent(w http.ResponseWriter, r *http.Request) {
	caller, ok := auth.CallerFromContext(r.Context())
	if !ok {
		httpx.WriteProblem(w, r, apperr.Unauthorized("unauthenticated", "Authentication required"))
		return
	}
	var req ingestEventRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	if req.Type == "" {
		httpx.WriteProblem(w, r, apperr.Validation("missing_type", "type is required"))
		return
	}
	ev := notification.ProviderEvent{
		Type: req.Type, Channel: req.Channel, Recipient: req.Recipient, Reason: req.Reason,
	}
	if req.NotificationID != nil && *req.NotificationID != "" {
		id, perr := uuid.Parse(*req.NotificationID)
		if perr != nil {
			httpx.WriteProblem(w, r, apperr.Validation("invalid_notification_id", "Invalid notificationId"))
			return
		}
		ev.NotificationID = &id
	}
	if err := h.svc.IngestEvent(r.Context(), caller.ApplicationID, ev); err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}
