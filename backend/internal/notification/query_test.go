package notification_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"dvtech/qbn/internal/notification"
)

// TestNotificationScopingIsolation (M10): las consultas están aisladas por
// tenant; otro application_id no puede leer/cancelar ni listar las notificaciones
// ajenas.
func TestNotificationScopingIsolation(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	svc := f.service(&fakeEnqueuer{})

	n, err := svc.Create(ctx, baseInput(f.appID))
	must(t, err)

	other := uuid.New() // otro tenant

	if _, err := svc.Get(ctx, other, n.ID); err == nil {
		t.Error("Get de otro tenant debería dar NotFound")
	}
	if _, err := svc.Cancel(ctx, other, n.ID); err == nil {
		t.Error("Cancel de otro tenant debería dar NotFound")
	}
	res, err := svc.List(ctx, notification.ListParams{ApplicationID: other})
	must(t, err)
	if len(res.Items) != 0 {
		t.Fatalf("otro tenant no debería ver notificaciones, got %d", len(res.Items))
	}
	// El propietario sí la ve.
	if _, err := svc.Get(ctx, f.appID, n.ID); err != nil {
		t.Fatalf("el tenant propietario debería verla: %v", err)
	}
}

func TestListAndPagination(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	svc := f.service(&fakeEnqueuer{})

	for i := 0; i < 3; i++ {
		_, err := svc.Create(ctx, baseInput(f.appID))
		must(t, err)
	}

	// Página de 2 → 2 items + cursor.
	page1, err := svc.List(ctx, notification.ListParams{ApplicationID: f.appID, Limit: 2})
	must(t, err)
	if len(page1.Items) != 2 || page1.NextCursor == "" {
		t.Fatalf("esperaba 2 items y cursor, got %d items cursor=%q", len(page1.Items), page1.NextCursor)
	}

	// Siguiente página con el cursor → el item restante, sin cursor.
	page2, err := svc.List(ctx, notification.ListParams{ApplicationID: f.appID, Limit: 2, Cursor: page1.NextCursor})
	must(t, err)
	if len(page2.Items) != 1 {
		t.Fatalf("esperaba 1 item en la 2ª página, got %d", len(page2.Items))
	}
	if page2.Items[0].ID == page1.Items[0].ID {
		t.Fatal("la 2ª página no debería repetir items")
	}
}

func TestListFilterByStatus(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	svc := f.service(&fakeEnqueuer{})
	_, err := svc.Create(ctx, baseInput(f.appID))
	must(t, err)

	sent := "SENT"
	res, err := svc.List(ctx, notification.ListParams{ApplicationID: f.appID, Status: &sent})
	must(t, err)
	if len(res.Items) != 0 {
		t.Fatalf("no debería haber notificaciones SENT todavía, got %d", len(res.Items))
	}
}

func TestCancel(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	svc := f.service(&fakeEnqueuer{})

	n, err := svc.Create(ctx, baseInput(f.appID)) // QUEUED
	must(t, err)

	cancelled, err := svc.Cancel(ctx, f.appID, n.ID)
	must(t, err)
	if string(cancelled.Status) != "CANCELLED" {
		t.Fatalf("esperaba CANCELLED, got %s", cancelled.Status)
	}

	// Cancelar una ya enviada → conflicto.
	n2, err := svc.Create(ctx, baseInput(f.appID))
	must(t, err)
	must(t, f.processor(&fakeSender{channel: "EMAIL"}).ProcessSend(ctx, n2.ID)) // → SENT
	if _, err := svc.Cancel(ctx, f.appID, n2.ID); err == nil {
		t.Fatal("cancelar una SENT debería fallar (conflicto)")
	}
}

func TestMarkRead(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	svc := f.service(&fakeEnqueuer{})
	n, err := svc.Create(ctx, baseInput(f.appID))
	must(t, err)

	read, err := svc.MarkRead(ctx, f.appID, n.ID)
	must(t, err)
	if string(read.Status) != "READ" || !read.ReadAt.Valid {
		t.Fatalf("esperaba READ con read_at, got %s valid=%v", read.Status, read.ReadAt.Valid)
	}
}

func TestBatchPartialSuccess(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	svc := f.service(&fakeEnqueuer{})

	res, err := svc.CreateBatch(ctx, notification.BatchInput{
		ApplicationID:    f.appID,
		TemplateKey:      "welcome",
		NotificationType: "transactional",
		Channel:          "EMAIL",
		Items: []notification.BatchItem{
			{Recipient: map[string]any{"email": "a@x.test"}, Payload: map[string]any{"firstName": "A"}}, // ok
			{Recipient: map[string]any{"email": "b@x.test"}, Payload: map[string]any{}},                 // falta firstName
		},
	})
	must(t, err)
	if len(res.Created) != 1 || len(res.Failed) != 1 {
		t.Fatalf("esperaba 1 creado y 1 fallido, got created=%d failed=%d", len(res.Created), len(res.Failed))
	}
	if res.Failed[0].Index != 1 {
		t.Fatalf("el item fallido debería ser el índice 1, got %d", res.Failed[0].Index)
	}
}

func TestPreferences(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	svc := f.service(&fakeEnqueuer{})

	// Por defecto: email/push/inApp on, sms off.
	def, err := svc.GetPreferences(ctx, f.appID, "u1")
	must(t, err)
	if !def.Email || def.SMS {
		t.Fatalf("preferencias por defecto inesperadas: %+v", def)
	}

	// Fijar y releer.
	_, err = svc.SetPreferences(ctx, f.appID, "u1", notification.Preferences{Email: false, Push: true, SMS: true, InApp: false})
	must(t, err)
	got, err := svc.GetPreferences(ctx, f.appID, "u1")
	must(t, err)
	if got.Email || !got.SMS || got.InApp {
		t.Fatalf("preferencias no persistidas correctamente: %+v", got)
	}
}
