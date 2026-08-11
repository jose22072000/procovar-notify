package notification_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"dvtech/qbn/internal/notification"
)

// TestSuppressedRecipientCancelled: un destinatario en la suppression list no se
// encola; la notificación queda CANCELLED.
func TestSuppressedRecipientCancelled(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	// La dirección de baseInput es user@acme.test (canal EMAIL).
	_, err := f.db.Pool.Exec(ctx, `
		INSERT INTO suppression_list (application_id, channel, recipient, reason)
		VALUES ($1,'EMAIL','user@acme.test','bounce')`, f.appID)
	must(t, err)

	enq := &fakeEnqueuer{}
	n, err := f.service(enq).Create(ctx, baseInput(f.appID))
	must(t, err)

	if string(n.Status) != "CANCELLED" {
		t.Fatalf("estado esperado CANCELLED, got %s", n.Status)
	}
	if len(enq.ids) != 0 {
		t.Fatalf("una notificación suprimida no debería encolarse, got %d", len(enq.ids))
	}
}

// TestIngestBounceThenSuppressed: un bounce ingerido añade el destinatario a la
// suppression list, y el siguiente envío a esa dirección se cancela.
func TestIngestBounceThenSuppressed(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	svc := f.service(&fakeEnqueuer{})

	must(t, svc.IngestEvent(ctx, f.appID, notification.ProviderEvent{
		Type: "bounce", Channel: "EMAIL", Recipient: "user@acme.test",
	}))

	n, err := svc.Create(ctx, baseInput(f.appID))
	must(t, err)
	if string(n.Status) != "CANCELLED" {
		t.Fatalf("tras el bounce, el envío debería cancelarse; got %s", n.Status)
	}
}

// TestIngestDeliveredUpdatesStatus: delivered confirma la entrega de una
// notificación SENT.
func TestIngestDeliveredUpdatesStatus(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	var id uuid.UUID
	must(t, f.db.Pool.QueryRow(ctx, `
		INSERT INTO notifications (application_id, template_key, notification_type, channel, recipient, status)
		VALUES ($1,'welcome','transactional','EMAIL','{}'::jsonb,'SENT') RETURNING id`, f.appID).Scan(&id))

	must(t, f.service(&fakeEnqueuer{}).IngestEvent(ctx, f.appID, notification.ProviderEvent{
		Type: "delivered", NotificationID: &id,
	}))

	got, err := f.db.Q.GetNotificationByID(ctx, id)
	must(t, err)
	if string(got.Status) != "DELIVERED" {
		t.Fatalf("estado esperado DELIVERED, got %s", got.Status)
	}
}

// fakeWebhooks captura los webhooks encolados.
type fakeWebhooks struct{ events []string }

func (f *fakeWebhooks) EnqueueWebhook(_ context.Context, _, _ uuid.UUID, event string) error {
	f.events = append(f.events, event)
	return nil
}

// TestIngestBounceMarksNotification (#9): un bounce con notificationId marca la
// notificación BOUNCED, deja log y emite webhook (antes solo suprimía).
func TestIngestBounceMarksNotification(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	var id uuid.UUID
	must(t, f.db.Pool.QueryRow(ctx, `
		INSERT INTO notifications (application_id, template_key, notification_type, channel, recipient, status)
		VALUES ($1,'welcome','transactional','EMAIL','{}'::jsonb,'SENT') RETURNING id`, f.appID).Scan(&id))

	wh := &fakeWebhooks{}
	must(t, f.service(&fakeEnqueuer{}).WithWebhooks(wh).IngestEvent(ctx, f.appID, notification.ProviderEvent{
		Type: "bounce", Channel: "EMAIL", Recipient: "user@acme.test", NotificationID: &id, Reason: "550 mailbox not found",
	}))

	got, err := f.db.Q.GetNotificationByID(ctx, id)
	must(t, err)
	if string(got.Status) != "BOUNCED" {
		t.Fatalf("esperaba BOUNCED, got %s", got.Status)
	}
	if len(wh.events) != 1 || wh.events[0] != "notification.bounced" {
		t.Fatalf("esperaba webhook notification.bounced, got %v", wh.events)
	}
	var logs int
	must(t, f.db.Pool.QueryRow(ctx, `SELECT count(*) FROM notification_logs WHERE notification_id=$1 AND event='bounced'`, id).Scan(&logs))
	if logs != 1 {
		t.Fatalf("esperaba 1 log 'bounced', got %d", logs)
	}

	// Un id de otra app (o inexistente) no marca nada ni emite webhook.
	other := uuid.New()
	must(t, f.service(&fakeEnqueuer{}).WithWebhooks(wh).IngestEvent(ctx, f.appID, notification.ProviderEvent{
		Type: "complaint", Channel: "EMAIL", Recipient: "x@acme.test", NotificationID: &other,
	}))
	if len(wh.events) != 1 {
		t.Fatalf("un id ajeno no debe emitir webhook, got %v", wh.events)
	}
}
