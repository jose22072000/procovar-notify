// Package domain contiene tipos y lógica de negocio puros, sin dependencias de
// infraestructura (ni store, ni http, ni colas).
package domain

// Estados de la notificación (espejo del enum notification_status en BD).
const (
	StatusPending    = "PENDING"
	StatusQueued     = "QUEUED"
	StatusProcessing = "PROCESSING"
	StatusRetry      = "RETRY"
	StatusSent       = "SENT"
	StatusDelivered  = "DELIVERED"
	StatusRead       = "READ"
	StatusFailed     = "FAILED"
	StatusCancelled  = "CANCELLED"
	StatusBounced    = "BOUNCED"
)

// Canales (espejo del enum channel en BD).
const (
	ChannelEmail = "EMAIL"
	ChannelPush  = "PUSH"
	ChannelSMS   = "SMS"
	ChannelInApp = "IN_APP"
)

// Scopes de la API pública.
const (
	ScopeNotificationsSend = "notifications:send"
	ScopeNotificationsRead = "notifications:read"
)

// QueueState es el estado de cola que muestra el monitor (§8.3). NO es un enum
// de BD: se DERIVA de (status, scheduled_at) con DeriveQueueState, de modo que
// el monitor (Fase 6) y la API usan exactamente la misma lógica.
type QueueState string

const (
	QueueScheduled QueueState = "scheduled"
	QueuePending   QueueState = "pending"
	QueueActive    QueueState = "active"
	QueueRetry     QueueState = "retry"
	QueueCompleted QueueState = "completed"
	QueueFailed    QueueState = "failed"
	QueueCancelled QueueState = "cancelled"
)

// DeriveQueueState traduce el estado persistido al estado de cola. scheduledInFuture
// indica si scheduled_at es posterior a "ahora".
func DeriveQueueState(status string, scheduledInFuture bool) QueueState {
	switch status {
	case StatusPending:
		if scheduledInFuture {
			return QueueScheduled
		}
		return QueuePending
	case StatusQueued:
		return QueuePending
	case StatusProcessing:
		return QueueActive
	case StatusRetry:
		return QueueRetry
	case StatusSent, StatusDelivered, StatusRead:
		return QueueCompleted
	case StatusFailed, StatusBounced:
		return QueueFailed
	case StatusCancelled:
		return QueueCancelled
	default:
		return QueuePending
	}
}
