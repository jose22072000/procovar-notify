package queue

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

// TaskWebhookDeliver entrega los webhooks de estado al tenant.
const TaskWebhookDeliver = "webhook:deliver"

// WebhookPayload identifica la notificación y el evento a notificar al tenant.
type WebhookPayload struct {
	ApplicationID  uuid.UUID `json:"applicationId"`
	NotificationID uuid.UUID `json:"notificationId"`
	Event          string    `json:"event"`
}

// EnqueueWebhook encola la entrega de un webhook (cola propia, con reintentos).
func (c *Client) EnqueueWebhook(ctx context.Context, applicationID, notificationID uuid.UUID, event string) error {
	payload, err := json.Marshal(WebhookPayload{ApplicationID: applicationID, NotificationID: notificationID, Event: event})
	if err != nil {
		return err
	}
	task := asynq.NewTask(TaskWebhookDeliver, payload)
	_, err = c.client.EnqueueContext(ctx, task,
		asynq.Queue(QueueWebhooks),
		asynq.MaxRetry(5),
	)
	return err
}

// RegisterWebhook registra el handler de entrega de webhooks sobre el mux.
func RegisterWebhook(mux *asynq.ServeMux, deliver func(ctx context.Context, p WebhookPayload) error) {
	mux.HandleFunc(TaskWebhookDeliver, func(ctx context.Context, t *asynq.Task) error {
		var p WebhookPayload
		if err := json.Unmarshal(t.Payload(), &p); err != nil {
			return asynq.SkipRetry
		}
		return deliver(ctx, p)
	})
}
