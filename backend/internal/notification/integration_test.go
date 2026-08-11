package notification_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"

	"dvtech/qbn/internal/channels"
	"dvtech/qbn/internal/crypto"
	"dvtech/qbn/internal/notification"
	"dvtech/qbn/internal/store"
	"dvtech/qbn/internal/store/sqlc"
	"dvtech/qbn/internal/storetest"
)

// fakeEnqueuer captura los ids y los ProcessAt encolados sin tocar Redis.
type fakeEnqueuer struct {
	ids        []uuid.UUID
	processAts []time.Time
}

func (f *fakeEnqueuer) EnqueueSend(_ context.Context, id uuid.UUID, _ string, _ int, processAt time.Time) error {
	f.ids = append(f.ids, id)
	f.processAts = append(f.processAts, processAt)
	return nil
}

// fakeSender simula un canal: cuenta envíos y puede fallar a demanda.
type fakeSender struct {
	channel string
	err     error
	sent    int
}

func (s *fakeSender) Channel() string { return s.channel }
func (s *fakeSender) Send(_ context.Context, _ channels.RenderedMessage, _ channels.ResolvedRoute) (channels.ProviderRef, error) {
	s.sent++
	if s.err != nil {
		return "", s.err
	}
	return "fake-ref", nil
}

type fixture struct {
	db    *store.DB
	enc   *crypto.Encryptor
	appID uuid.UUID
}

// setup crea app + template EMAIL + conexión SMTP + ruta por defecto.
func setup(t *testing.T) fixture {
	t.Helper()
	db := storetest.NewPostgres(t)
	ctx := context.Background()
	enc, _ := crypto.NewEncryptor(make([]byte, 32))

	app, err := db.Q.CreateApplication(ctx, sqlc.CreateApplicationParams{Name: "Acme", Slug: "acme"})
	must(t, err)

	// Template EMAIL "welcome" con required_variables (firstName requerido).
	_, err = db.Pool.Exec(ctx, `
		INSERT INTO templates (application_id, key, channel, name, subject, structure, body, required_variables, locale, version)
		VALUES ($1,'welcome','EMAIL','Bienvenida','Hola {{firstName}}','{}'::jsonb,
		        '<p>Hola {{firstName}}</p>',
		        '{"type":"object","required":["firstName"],"properties":{"firstName":{"type":"string"}}}'::jsonb,
		        'en',1)`, app.ID)
	must(t, err)

	// Conexión SMTP (password cifrado) + ruta transactional/EMAIL por defecto.
	pwd, _ := enc.Encrypt([]byte(""))
	var smtpID uuid.UUID
	must(t, db.Pool.QueryRow(ctx, `
		INSERT INTO smtp_connections (application_id, name, host, port, username, password_enc, from_email, from_name, secure)
		VALUES ($1,'mailhog','localhost',1025,'',$2,'noreply@acme.test','Acme',false) RETURNING id`,
		app.ID, pwd).Scan(&smtpID))
	_, err = db.Pool.Exec(ctx, `
		INSERT INTO channel_routes (application_id, notification_type, channel, template_key, smtp_connection_id)
		VALUES ($1,'transactional','EMAIL','welcome',$2)`, app.ID, smtpID)
	must(t, err)

	return fixture{db: db, enc: enc, appID: app.ID}
}

func (f fixture) service(enq notification.Enqueuer) *notification.Service {
	return notification.NewService(f.db, enq, slog.Default())
}

func (f fixture) processor(sender channels.Sender) *notification.Processor {
	d := channels.NewDispatcher(sender, channels.NewInAppSender())
	return notification.NewProcessor(f.db, d, f.enc, slog.Default())
}

func baseInput(appID uuid.UUID) notification.CreateInput {
	return notification.CreateInput{
		ApplicationID:    appID,
		TemplateKey:      "welcome",
		NotificationType: "transactional",
		Channel:          "EMAIL",
		Recipient:        map[string]any{"email": "user@acme.test", "name": "Jane"},
		Payload:          map[string]any{"firstName": "Jane"},
	}
}

// TestSendFlowSuccess: crear → QUEUED → procesar → SENT.
func TestSendFlowSuccess(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	enq := &fakeEnqueuer{}

	n, err := f.service(enq).Create(ctx, baseInput(f.appID))
	must(t, err)
	if string(n.Status) != "QUEUED" {
		t.Fatalf("estado esperado QUEUED, got %s", n.Status)
	}
	if len(enq.ids) != 1 || enq.ids[0] != n.ID {
		t.Fatalf("debería haberse encolado la notificación")
	}

	sender := &fakeSender{channel: "EMAIL"}
	must(t, f.processor(sender).ProcessSend(ctx, n.ID))
	if sender.sent != 1 {
		t.Fatalf("el sender debería haberse invocado 1 vez, got %d", sender.sent)
	}

	got, err := f.db.Q.GetNotificationByID(ctx, n.ID)
	must(t, err)
	if string(got.Status) != "SENT" {
		t.Fatalf("estado esperado SENT, got %s", got.Status)
	}
}

// TestOptOutCancels: si el usuario deshabilitó el canal → CANCELLED sin encolar.
func TestOptOutCancels(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	_, err := f.db.Pool.Exec(ctx, `
		INSERT INTO user_notification_preferences (application_id, user_id, email_enabled)
		VALUES ($1,'u1',false)`, f.appID)
	must(t, err)

	enq := &fakeEnqueuer{}
	in := baseInput(f.appID)
	uid := "u1"
	in.RecipientUserID = &uid

	n, err := f.service(enq).Create(ctx, in)
	must(t, err)
	if string(n.Status) != "CANCELLED" {
		t.Fatalf("estado esperado CANCELLED, got %s", n.Status)
	}
	if len(enq.ids) != 0 {
		t.Fatalf("no debería haberse encolado")
	}
}

// TestIdempotency: misma idempotencyKey → misma notificación, un solo encolado.
func TestIdempotency(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	enq := &fakeEnqueuer{}
	svc := f.service(enq)

	key := "order-123"
	in := baseInput(f.appID)
	in.IdempotencyKey = &key

	n1, err := svc.Create(ctx, in)
	must(t, err)
	n2, err := svc.Create(ctx, in)
	must(t, err)

	if n1.ID != n2.ID {
		t.Fatalf("la misma clave debería devolver la misma notificación")
	}
	if len(enq.ids) != 1 {
		t.Fatalf("solo debería encolarse una vez, got %d", len(enq.ids))
	}
}

// TestValidationFails: payload sin la variable requerida → error de validación.
func TestValidationFails(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	in := baseInput(f.appID)
	in.Payload = map[string]any{} // falta firstName

	_, err := f.service(&fakeEnqueuer{}).Create(ctx, in)
	if err == nil {
		t.Fatal("debería fallar por validación de payload")
	}
}

// TestSendFailureMarksFailed: si el sender falla (sin reintentos asynq) → FAILED.
func TestSendFailureMarksFailed(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	enq := &fakeEnqueuer{}

	n, err := f.service(enq).Create(ctx, baseInput(f.appID))
	must(t, err)

	sender := &fakeSender{channel: "EMAIL", err: errors.New("smtp caído")}
	// ProcessSend devuelve el error de envío; el estado debe quedar FAILED.
	if err := f.processor(sender).ProcessSend(ctx, n.ID); err == nil {
		t.Fatal("ProcessSend debería devolver el error de envío")
	}

	got, _ := f.db.Q.GetNotificationByID(ctx, n.ID)
	if string(got.Status) != "FAILED" {
		t.Fatalf("estado esperado FAILED, got %s", got.Status)
	}
}

// TestScheduledStaysPending: con scheduledAt futuro, la notificación queda
// PENDING (programada) pero igualmente se encola (con ProcessAt).
func TestScheduledStaysPending(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	enq := &fakeEnqueuer{}

	in := baseInput(f.appID)
	future := time.Now().Add(time.Hour)
	in.ScheduledAt = &future

	n, err := f.service(enq).Create(ctx, in)
	must(t, err)
	if string(n.Status) != "PENDING" {
		t.Fatalf("una notificación programada debería quedar PENDING, got %s", n.Status)
	}
	if len(enq.ids) != 1 {
		t.Fatalf("debería haberse encolado (con ProcessAt), got %d", len(enq.ids))
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
}
