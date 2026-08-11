package queue

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

// TaskSendNotification es el tipo de tarea de envío. El payload solo lleva el
// id: el worker carga el resto desde Postgres (fuente de verdad).
const TaskSendNotification = "notification:send"

// Nombres de las colas ponderadas (ver Config en server.go).
const (
	QueueCritical = "critical"
	QueueDefault  = "default"
	// QueueWebhooks aísla la entrega de webhooks: un endpoint de tenant lento
	// no compite por workers con los envíos de notificaciones.
	QueueWebhooks = "webhooks"
	QueueLow      = "low"
)

// SendNotificationPayload es el contrato de payload compartido entre api y
// worker. RequestID correlaciona los logs del worker con la petición HTTP que
// originó el envío (antes el rastro se cortaba al cruzar la cola).
type SendNotificationPayload struct {
	NotificationID uuid.UUID `json:"notification_id"`
	RequestID      string    `json:"request_id,omitempty"`
}

// newSendTask serializa la tarea de envío para una notificación.
func newSendTask(id uuid.UUID, requestID string) (*asynq.Task, error) {
	payload, err := json.Marshal(SendNotificationPayload{NotificationID: id, RequestID: requestID})
	if err != nil {
		return nil, fmt.Errorf("serializando payload: %w", err)
	}
	return asynq.NewTask(TaskSendNotification, payload), nil
}

// queueForPriority enruta cada prioridad a su cola. HIGH comparte critical con
// URGENT (antes caía a default y no surtía efecto); la distinción se conserva
// en la fila para orden y métricas.
func queueForPriority(priority string) string {
	switch priority {
	case "URGENT", "HIGH":
		return QueueCritical
	case "LOW":
		return QueueLow
	default: // NORMAL
		return QueueDefault
	}
}

// parseSendPayload deserializa el payload de una tarea de envío.
func parseSendPayload(t *asynq.Task) (SendNotificationPayload, error) {
	var p SendNotificationPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return p, fmt.Errorf("payload inválido: %w", err)
	}
	return p, nil
}
