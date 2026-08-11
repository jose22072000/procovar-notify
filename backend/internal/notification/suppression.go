package notification

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	"dvtech/qbn/internal/apperr"
	"dvtech/qbn/internal/domain"
	"dvtech/qbn/internal/store"
	"dvtech/qbn/internal/store/sqlc"
)

// ProviderEvent es un evento de proveedor normalizado que el tenant (o su
// adaptador del proveedor) reenvía a /v1/events.
type ProviderEvent struct {
	Type           string // bounce | complaint | delivered | opened
	Channel        string // EMAIL | SMS | ...
	Recipient      string // email/teléfono (para bounce/complaint)
	NotificationID *uuid.UUID
	Reason         string
}

// IngestEvent procesa un evento de proveedor: bounce/complaint añaden el
// destinatario a la suppression list; delivered confirma la entrega. Eventos
// desconocidos se ignoran (no es un error).
func (s *Service) IngestEvent(ctx context.Context, appID uuid.UUID, ev ProviderEvent) error {
	switch ev.Type {
	case "bounce", "complaint":
		if ev.Channel == "" || ev.Recipient == "" {
			return apperr.Validation("missing_fields", "bounce/complaint require channel and recipient")
		}
		reason := ev.Reason
		if reason == "" {
			reason = ev.Type
		}
		_, err := s.db.Q.AddSuppression(ctx, sqlc.AddSuppressionParams{
			ApplicationID: appID, Channel: sqlc.Channel(ev.Channel), Recipient: ev.Recipient, Reason: reason,
		})
		if err != nil {
			return apperr.Internal(err)
		}
		// Con notificationId, el evento también actualiza la notificación:
		// antes un bounce duro la dejaba SENT para siempre y el tenant no se
		// enteraba. Solo avanza desde SENT/DELIVERED (scope por appID).
		if ev.NotificationID != nil {
			rows, err := s.db.Q.MarkNotificationBounced(ctx, sqlc.MarkNotificationBouncedParams{
				ID: *ev.NotificationID, ApplicationID: appID,
			})
			if err != nil {
				return apperr.Internal(err)
			}
			if rows > 0 {
				event := "bounced"
				if ev.Type == "complaint" {
					event = "complained"
				}
				warnErr(s.logger, "no se pudo escribir el log de "+event, s.db.Q.CreateNotificationLog(ctx, sqlc.CreateNotificationLogParams{
					ApplicationID: appID, NotificationID: pgUUID(*ev.NotificationID), Event: event, Message: &reason,
				}))
				if s.webhooks != nil {
					warnErr(s.logger, "no se pudo encolar el webhook", s.webhooks.EnqueueWebhook(ctx, appID, *ev.NotificationID, "notification."+event))
				}
			}
		}
		return nil
	case "delivered":
		if ev.NotificationID == nil {
			return apperr.Validation("missing_notification_id", "delivered requires notificationId")
		}
		if err := s.db.Q.MarkNotificationDelivered(ctx, sqlc.MarkNotificationDeliveredParams{
			ID: *ev.NotificationID, ApplicationID: appID,
		}); err != nil {
			return apperr.Internal(err)
		}
		return nil
	default:
		return nil // opened u otros: aceptados sin efecto por ahora
	}
}

// recipientAddress devuelve la dirección suprimible del destinatario según el
// canal (email para EMAIL, teléfono para SMS). Vacío si el canal no aplica.
func recipientAddress(channel string, recipient map[string]any) string {
	switch channel {
	case domain.ChannelEmail:
		if v, ok := recipient["email"].(string); ok {
			return v
		}
	case domain.ChannelSMS:
		if v, ok := recipient["phone"].(string); ok {
			return v
		}
	}
	return ""
}

// handleSuppressed cancela el envío si el destinatario está en la suppression
// list de la aplicación (rebotes/quejas). Crea la notificación como CANCELLED
// (con log) y no la encola. Devuelve (true, n) si estaba suprimido.
func (s *Service) handleSuppressed(ctx context.Context, in CreateInput, channel string) (bool, sqlc.Notification, error) {
	addr := recipientAddress(channel, in.Recipient)
	if addr == "" {
		return false, sqlc.Notification{}, nil
	}
	suppressed, err := s.db.Q.IsSuppressed(ctx, sqlc.IsSuppressedParams{
		ApplicationID: in.ApplicationID, Channel: sqlc.Channel(channel), Recipient: addr,
	})
	if err != nil {
		return false, sqlc.Notification{}, apperr.Internal(err)
	}
	if !suppressed {
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
		Message:        ptr("recipient is on the suppression list"),
	}))
	return true, n, nil
}
