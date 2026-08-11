package queue

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"dvtech/qbn/internal/channels"
)

func TestQueueForPriority(t *testing.T) {
	cases := map[string]string{
		"URGENT":      QueueCritical,
		"HIGH":        QueueCritical, // antes caía a default: promesa incumplida de la API
		"LOW":         QueueLow,
		"NORMAL":      QueueDefault,
		"":            QueueDefault,
		"desconocida": QueueDefault,
	}
	for in, want := range cases {
		if got := queueForPriority(in); got != want {
			t.Errorf("queueForPriority(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSendPayloadRoundtrip(t *testing.T) {
	id := uuid.New()
	task, err := newSendTask(id, "req-123")
	if err != nil {
		t.Fatalf("newSendTask: %v", err)
	}
	if task.Type() != TaskSendNotification {
		t.Fatalf("tipo de tarea = %q, want %q", task.Type(), TaskSendNotification)
	}
	p, err := parseSendPayload(task)
	if err != nil {
		t.Fatalf("parseSendPayload: %v", err)
	}
	if p.RequestID != "req-123" {
		t.Fatalf("request_id debería sobrevivir el roundtrip, got %q", p.RequestID)
	}
	if p.NotificationID != id {
		t.Fatalf("id = %s, want %s", p.NotificationID, id)
	}
}

func TestParseSendPayloadInvalid(t *testing.T) {
	bad := asynq.NewTask(TaskSendNotification, []byte("no es json"))
	if _, err := parseSendPayload(bad); err == nil {
		t.Fatal("un payload inválido debería dar error")
	}
}

// TestUnavailableRetrySemantics: la indisponibilidad del destino no consume
// reintentos (IsFailure=false) y se reintenta con retardo fijo >= timeout del
// breaker; el resto de errores siguen el backoff por defecto.
func TestUnavailableRetrySemantics(t *testing.T) {
	unavailable := channels.Unavailable(errors.New("breaker abierto"))
	if isFailure(unavailable) {
		t.Fatal("Unavailable no debe contar como fallo (no consume reintentos)")
	}
	if !isFailure(errors.New("smtp 550")) {
		t.Fatal("un error normal sí debe contar como fallo")
	}
	task := asynq.NewTask(TaskSendNotification, nil)
	if d := retryDelay(0, unavailable, task); d != providerRetryDelay {
		t.Fatalf("retardo con destino caído: esperado %v, got %v", providerRetryDelay, d)
	}
	if d := retryDelay(0, errors.New("otro"), task); d == providerRetryDelay {
		t.Fatal("un error normal no debe usar el retardo fijo de indisponibilidad")
	}
}
