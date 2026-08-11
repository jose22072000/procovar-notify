package http

import (
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"dvtech/qbn/internal/domain"
	"dvtech/qbn/internal/store/sqlc"
)

// notificationView es la representación JSON pública de una notificación, sin
// exponer tipos internos (pgtype, etc.) y con timestamps en RFC3339.
type notificationView struct {
	ID               string          `json:"id"`
	TemplateKey      string          `json:"templateKey"`
	NotificationType string          `json:"notificationType"`
	Channel          string          `json:"channel"`
	Status           string          `json:"status"`
	RecipientUserID  *string         `json:"recipientUserId,omitempty"`
	Recipient        json.RawMessage `json:"recipient,omitempty"`
	Payload          json.RawMessage `json:"payload,omitempty"`
	Priority         string          `json:"priority"`
	QueueState       string          `json:"queueState"`
	Locale           *string         `json:"locale,omitempty"`
	RetryCount       int32           `json:"retryCount"`
	CreatedAt        string          `json:"createdAt"`
	SentAt           *string         `json:"sentAt,omitempty"`
	DeliveredAt      *string         `json:"deliveredAt,omitempty"`
	ReadAt           *string         `json:"readAt,omitempty"`
	FailedAt         *string         `json:"failedAt,omitempty"`
	ArchivedAt       *string         `json:"archivedAt,omitempty"`
}

func toNotificationView(n sqlc.Notification) notificationView {
	return notificationView{
		ID:               n.ID.String(),
		TemplateKey:      n.TemplateKey,
		NotificationType: n.NotificationType,
		Channel:          string(n.Channel),
		Status:           string(n.Status),
		RecipientUserID:  n.RecipientUserID,
		Recipient:        json.RawMessage(n.Recipient),
		Payload:          json.RawMessage(n.Payload),
		Priority:         string(n.Priority),
		QueueState:       string(domain.DeriveQueueState(string(n.Status), n.ScheduledAt.Valid && n.ScheduledAt.Time.After(time.Now()))),
		Locale:           n.Locale,
		RetryCount:       n.RetryCount,
		CreatedAt:        n.CreatedAt.Time.Format(time.RFC3339),
		SentAt:           tsPtr(n.SentAt),
		DeliveredAt:      tsPtr(n.DeliveredAt),
		ReadAt:           tsPtr(n.ReadAt),
		FailedAt:         tsPtr(n.FailedAt),
		ArchivedAt:       tsPtr(n.ArchivedAt),
	}
}

func toNotificationViews(ns []sqlc.Notification) []notificationView {
	out := make([]notificationView, 0, len(ns))
	for _, n := range ns {
		out = append(out, toNotificationView(n))
	}
	return out
}

// tsPtr formatea un timestamptz nullable a *string RFC3339 (nil si NULL).
func tsPtr(ts pgtype.Timestamptz) *string {
	if !ts.Valid {
		return nil
	}
	s := ts.Time.Format(time.RFC3339)
	return &s
}
