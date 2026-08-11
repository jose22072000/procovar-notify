package monitor_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"

	"dvtech/qbn/internal/channels"
	"dvtech/qbn/internal/crypto"
	"dvtech/qbn/internal/monitor"
	"dvtech/qbn/internal/notification"
	"dvtech/qbn/internal/store"
	"dvtech/qbn/internal/store/sqlc"
	"dvtech/qbn/internal/storetest"
)

type fakeEnqueuer struct{ ids []uuid.UUID }

func (f *fakeEnqueuer) EnqueueSend(_ context.Context, id uuid.UUID, _ string, _ int, _ time.Time) error {
	f.ids = append(f.ids, id)
	return nil
}

type fakeRetrier struct{ retried []uuid.UUID }

func (f *fakeRetrier) EnqueueRetry(_ context.Context, id uuid.UUID, _ string, _ int) error {
	f.retried = append(f.retried, id)
	return nil
}

type fakeSender struct {
	channel string
	err     error
}

func (s *fakeSender) Channel() string { return s.channel }
func (s *fakeSender) Send(_ context.Context, _ channels.RenderedMessage, _ channels.ResolvedRoute) (channels.ProviderRef, error) {
	if s.err != nil {
		return "", s.err
	}
	return "ref", nil
}

type harness struct {
	db    *store.DB
	enc   *crypto.Encryptor
	appID uuid.UUID
	notif *notification.Service
	mon   *monitor.Service
	retr  *fakeRetrier
}

func setup(t *testing.T) harness {
	t.Helper()
	db := storetest.NewPostgres(t)
	ctx := context.Background()
	enc, _ := crypto.NewEncryptor(make([]byte, 32))
	app, err := db.Q.CreateApplication(ctx, sqlc.CreateApplicationParams{Name: "Acme", Slug: "acme"})
	must(t, err)

	_, err = db.Pool.Exec(ctx, `
		INSERT INTO templates (application_id, key, channel, name, subject, structure, body, required_variables, locale, version)
		VALUES ($1,'welcome','EMAIL','B','S','{}'::jsonb,'<p>{{firstName}}</p>',
		        '{"type":"object","required":["firstName"],"properties":{"firstName":{"type":"string"}}}'::jsonb,'en',1)`, app.ID)
	must(t, err)
	pwd, _ := enc.Encrypt([]byte(""))
	var smtpID uuid.UUID
	must(t, db.Pool.QueryRow(ctx, `
		INSERT INTO smtp_connections (application_id,name,host,port,username,password_enc,from_email,from_name,secure)
		VALUES ($1,'m','localhost',1025,'',$2,'n@a.t','A',false) RETURNING id`, app.ID, pwd).Scan(&smtpID))
	_, err = db.Pool.Exec(ctx, `
		INSERT INTO channel_routes (application_id,notification_type,channel,smtp_connection_id)
		VALUES ($1,'transactional','EMAIL',$2)`, app.ID, smtpID)
	must(t, err)

	retr := &fakeRetrier{}
	return harness{
		db: db, enc: enc, appID: app.ID,
		notif: notification.NewService(db, &fakeEnqueuer{}, slog.Default()),
		mon:   monitor.NewService(db, retr, slog.Default()),
		retr:  retr,
	}
}

func (h harness) create(t *testing.T) sqlc.Notification {
	t.Helper()
	n, err := h.notif.Create(context.Background(), notification.CreateInput{
		ApplicationID: h.appID, TemplateKey: "welcome", NotificationType: "transactional",
		Channel: "EMAIL", Recipient: map[string]any{"email": "u@a.test"}, Payload: map[string]any{"firstName": "U"},
	})
	must(t, err)
	return n
}

func TestSummary(t *testing.T) {
	h := setup(t)
	ctx := context.Background()
	h.create(t)
	h.create(t)

	s, err := h.mon.Summary(ctx, h.appID)
	must(t, err)
	if s.Total != 2 || s.ByState["pending"] != 2 { // QUEUED → pending
		t.Fatalf("summary inesperado: %+v", s)
	}
	if s.ByPriority["NORMAL"] != 2 {
		t.Fatalf("byPriority inesperado: %+v", s.ByPriority)
	}
}

func TestDetailHasAttemptsAndLogs(t *testing.T) {
	h := setup(t)
	ctx := context.Background()
	n := h.create(t)

	proc := notification.NewProcessor(h.db, channels.NewDispatcher(&fakeSender{channel: "EMAIL"}), h.enc, slog.Default())
	must(t, proc.ProcessSend(ctx, n.ID))

	d, err := h.mon.Detail(ctx, h.appID, n.ID)
	must(t, err)
	if string(d.Notification.Status) != "SENT" {
		t.Fatalf("estado esperado SENT, got %s", d.Notification.Status)
	}
	if len(d.Attempts) != 1 || string(d.Attempts[0].Status) != "SUCCESS" {
		t.Fatalf("debería haber 1 intento SUCCESS, got %+v", d.Attempts)
	}
	if len(d.Logs) == 0 {
		t.Fatal("debería haber logs")
	}
}

func TestRetryFailed(t *testing.T) {
	h := setup(t)
	ctx := context.Background()
	n := h.create(t)

	// Forzar fallo → FAILED.
	proc := notification.NewProcessor(h.db, channels.NewDispatcher(&fakeSender{channel: "EMAIL", err: errors.New("boom")}), h.enc, slog.Default())
	_ = proc.ProcessSend(ctx, n.ID)

	if err := h.mon.Retry(ctx, h.appID, n.ID); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if len(h.retr.retried) != 1 {
		t.Fatal("debería haberse reencolado")
	}
	got, _ := h.db.Q.GetNotificationByID(ctx, n.ID)
	if string(got.Status) != "QUEUED" {
		t.Fatalf("tras retry debería estar QUEUED, got %s", got.Status)
	}

	// Reintentar una no-fallida → conflicto.
	if err := h.mon.Retry(ctx, h.appID, n.ID); err == nil {
		t.Fatal("retry de una QUEUED debería fallar")
	}
}

func TestCancel(t *testing.T) {
	h := setup(t)
	ctx := context.Background()
	n := h.create(t) // QUEUED

	if err := h.mon.Cancel(ctx, h.appID, n.ID); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	got, _ := h.db.Q.GetNotificationByID(ctx, n.ID)
	if string(got.Status) != "CANCELLED" {
		t.Fatalf("debería estar CANCELLED, got %s", got.Status)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
}
