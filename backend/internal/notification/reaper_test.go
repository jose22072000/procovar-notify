package notification_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"dvtech/qbn/internal/notification"
)

// insertNotifRow inserta una notificación directamente en SQL para controlar
// updated_at (el trigger set_updated_at pisa cualquier UPDATE con now(), así
// que la antigüedad solo puede fijarse en el INSERT).
func insertNotifRow(t *testing.T, f fixture, status string, age time.Duration, scheduledAt *time.Time) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := f.db.Pool.QueryRow(context.Background(), `
		INSERT INTO notifications (application_id, template_key, notification_type, channel, recipient,
		                           payload, priority, status, max_retries, scheduled_at, created_at, updated_at)
		VALUES ($1, 'welcome', 'transactional', 'EMAIL', '{"email":"user@acme.test"}'::jsonb, '{}'::jsonb,
		        'NORMAL', $2, 3, $3, now() - $4::interval, now() - $4::interval)
		RETURNING id`, f.appID, status, scheduledAt, age.String()).Scan(&id)
	must(t, err)
	return id
}

// TestReaperRequeuesOrphans: una PENDING antigua sin tarea (el hueco
// commit-luego-encolar) se reencola; las frescas, programadas a futuro o
// terminales se quedan como están.
func TestReaperRequeuesOrphans(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	enq := &fakeEnqueuer{}
	svc := f.service(enq)

	future := time.Now().Add(time.Hour)
	orphan := insertNotifRow(t, f, "PENDING", 10*time.Minute, nil)        // huérfana real
	stuckQueued := insertNotifRow(t, f, "QUEUED", 10*time.Minute, nil)    // atascada tras retry
	fresh := insertNotifRow(t, f, "PENDING", 0, nil)                      // aún en flujo normal
	scheduled := insertNotifRow(t, f, "PENDING", 10*time.Minute, &future) // programada: legítimamente PENDING
	sent := insertNotifRow(t, f, "SENT", 10*time.Minute, nil)             // terminal

	requeued, err := svc.RequeueStale(ctx, 2*time.Minute, 100)
	must(t, err)
	if requeued != 2 {
		t.Fatalf("esperaba 2 reencoladas (orphan+stuckQueued), got %d", requeued)
	}
	got := map[uuid.UUID]bool{}
	for _, id := range enq.ids {
		got[id] = true
	}
	if !got[orphan] || !got[stuckQueued] {
		t.Fatalf("faltan ids esperados en la cola: %v", enq.ids)
	}
	if got[fresh] || got[scheduled] || got[sent] {
		t.Fatalf("se reencoló algo que no debía: %v", enq.ids)
	}

	// El rescate queda auditado en notification_logs.
	var logs int
	must(t, f.db.Pool.QueryRow(ctx,
		`SELECT count(*) FROM notification_logs WHERE notification_id = $1 AND event = 'requeued'`,
		orphan).Scan(&logs))
	if logs != 1 {
		t.Fatalf("esperaba 1 log 'requeued' para la huérfana, got %d", logs)
	}
}

// TestReaperIdempotentSweep: dos pasadas seguidas no duplican nada raro a nivel
// del servicio (el no-duplicado real lo garantiza asynq por TaskID; aquí se
// verifica que el barrido en sí es estable y reencola lo mismo).
func TestReaperIdempotentSweep(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	enq := &fakeEnqueuer{}
	svc := f.service(enq)

	insertNotifRow(t, f, "PENDING", 10*time.Minute, nil)

	n1, err := svc.RequeueStale(ctx, 2*time.Minute, 100)
	must(t, err)
	n2, err := svc.RequeueStale(ctx, 2*time.Minute, 100)
	must(t, err)
	if n1 != 1 || n2 != 1 {
		t.Fatalf("cada pasada debería ver 1 candidata (asynq dedupe por TaskID), got %d y %d", n1, n2)
	}

	// notification.DefaultReap* son la config que usa el worker; el umbral debe
	// superar con margen la latencia normal de cola.
	if notification.DefaultReapThreshold < time.Minute {
		t.Fatalf("umbral por defecto sospechosamente corto: %v", notification.DefaultReapThreshold)
	}
}

// failingEnqueuer falla el encolado: simula el crash/failover de Redis justo
// después del commit (la fila queda persistida pero sin tarea).
type failingEnqueuer struct{ fakeEnqueuer }

func (f *failingEnqueuer) EnqueueSend(context.Context, uuid.UUID, string, int, time.Time) error {
	return errors.New("redis caído")
}

// TestIdempotencyHitRequeuesStalled: si el primer Create commitea pero no logra
// encolar, el reintento del cliente con la misma IdempotencyKey debe devolver
// la fila existente Y reencolarla (mitad B del hueco commit-luego-encolar).
func TestIdempotencyHitRequeuesStalled(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	key := "otp-123"

	in := baseInput(f.appID)
	in.IdempotencyKey = &key

	// 1) Primer intento: commit OK, enqueue falla → notificación huérfana.
	first, err := f.service(&failingEnqueuer{}).Create(ctx, in)
	if err == nil {
		t.Fatal("esperaba error de encolado en el primer intento")
	}
	if first.ID == uuid.Nil {
		t.Fatal("la fila debería haberse persistido pese al fallo de encolado")
	}

	// 2) Reintento del cliente (misma key), ya con la cola sana: devuelve la
	// existente y la reencola.
	enq := &fakeEnqueuer{}
	got, err := f.service(enq).Create(ctx, in)
	must(t, err)
	if got.ID != first.ID {
		t.Fatalf("la idempotencia debería devolver la misma notificación (%s != %s)", got.ID, first.ID)
	}
	if len(enq.ids) != 1 || enq.ids[0] != first.ID {
		t.Fatalf("el hit de idempotencia debería reencolar la huérfana, encolados: %v", enq.ids)
	}
}

// TestIdempotencyHitDoesNotRequeueTerminal: una notificación ya SENT no se
// reencola en el hit de idempotencia (nada de reenvíos por reintentos tardíos).
func TestIdempotencyHitDoesNotRequeueTerminal(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	key := "otp-456"

	in := baseInput(f.appID)
	in.IdempotencyKey = &key

	created, err := f.service(&fakeEnqueuer{}).Create(ctx, in)
	must(t, err)
	_, err = f.db.Pool.Exec(ctx, `UPDATE notifications SET status='SENT', sent_at=now() WHERE id=$1`, created.ID)
	must(t, err)

	enq := &fakeEnqueuer{}
	got, err := f.service(enq).Create(ctx, in)
	must(t, err)
	if got.ID != created.ID {
		t.Fatalf("debería devolver la existente")
	}
	if len(enq.ids) != 0 {
		t.Fatalf("una SENT no debe reencolarse, encolados: %v", enq.ids)
	}
}
