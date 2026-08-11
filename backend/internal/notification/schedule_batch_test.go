package notification_test

import (
	"context"
	"testing"
	"time"

	"dvtech/qbn/internal/notification"
)

// TestScheduledEnqueuesAtFutureTime verifica que scheduledAt se propaga como
// ProcessAt al encolar (la tarea no se ejecuta antes de su hora).
func TestScheduledEnqueuesAtFutureTime(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	enq := &fakeEnqueuer{}

	in := baseInput(f.appID)
	future := time.Now().Add(2 * time.Hour).Truncate(time.Second)
	in.ScheduledAt = &future

	_, err := f.service(enq).Create(ctx, in)
	must(t, err)

	if len(enq.processAts) != 1 {
		t.Fatalf("debería haberse encolado una vez, got %d", len(enq.processAts))
	}
	got := enq.processAts[0]
	if diff := got.Sub(future); diff < -2*time.Second || diff > 2*time.Second {
		t.Fatalf("ProcessAt = %v, se esperaba ~%v (scheduledAt)", got, future)
	}
}

// TestImmediateEnqueuesNow: sin scheduledAt, el ProcessAt es el cero
// (convenio "encolar ahora"), no una hora futura.
func TestImmediateEnqueuesNow(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	enq := &fakeEnqueuer{}

	_, err := f.service(enq).Create(ctx, baseInput(f.appID))
	must(t, err)

	if len(enq.processAts) != 1 {
		t.Fatalf("debería haberse encolado una vez, got %d", len(enq.processAts))
	}
	if !enq.processAts[0].IsZero() {
		t.Fatalf("ProcessAt = %v, se esperaba cero (ahora)", enq.processAts[0])
	}
}

// TestBatchRejectsOverLimit: más de MaxBatchRecipients destinatarios → error
// antes de crear nada.
func TestBatchRejectsOverLimit(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	items := make([]notification.BatchItem, notification.MaxBatchRecipients+1)
	for i := range items {
		items[i] = notification.BatchItem{Recipient: map[string]any{"email": "a@b.c"}, Payload: map[string]any{"firstName": "X"}}
	}
	_, err := f.service(&fakeEnqueuer{}).CreateBatch(ctx, notification.BatchInput{
		ApplicationID:    f.appID,
		TemplateKey:      "welcome",
		NotificationType: "transactional",
		Channel:          "EMAIL",
		Items:            items,
	})
	if err == nil {
		t.Fatal("se esperaba error por exceder el tope de batch")
	}
}

// TestBatchRejectsUrgentPriority: los masivos no pueden usar la cola critical
// (URGENT/HIGH se rechazan explícitamente; #3 del análisis de robustez).
func TestBatchRejectsUrgentPriority(t *testing.T) {
	f := setup(t)
	for _, p := range []string{"URGENT", "HIGH"} {
		_, err := f.service(&fakeEnqueuer{}).CreateBatch(context.Background(), notification.BatchInput{
			ApplicationID:    f.appID,
			TemplateKey:      "welcome",
			NotificationType: "transactional",
			Channel:          "EMAIL",
			Priority:         p,
			Items:            []notification.BatchItem{{Recipient: map[string]any{"email": "a@b.c"}, Payload: map[string]any{"firstName": "X"}}},
		})
		if err == nil {
			t.Fatalf("se esperaba rechazo del batch con priority %s", p)
		}
	}
}

// TestBatchRejectsEmpty: batch sin destinatarios → error.
func TestBatchRejectsEmpty(t *testing.T) {
	f := setup(t)
	_, err := f.service(&fakeEnqueuer{}).CreateBatch(context.Background(), notification.BatchInput{
		ApplicationID:    f.appID,
		TemplateKey:      "welcome",
		NotificationType: "transactional",
		Channel:          "EMAIL",
		Items:            nil,
	})
	if err == nil {
		t.Fatal("se esperaba error por batch vacío")
	}
}
