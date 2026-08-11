package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/google/uuid"

	"dvtech/qbn/internal/crypto"
	"dvtech/qbn/internal/queue"
	"dvtech/qbn/internal/safedial"
	"dvtech/qbn/internal/store/sqlc"
	"dvtech/qbn/internal/storetest"
)

func TestSubscribed(t *testing.T) {
	if !subscribed(nil, "notification.sent") {
		t.Error("lista vacía debería recibir todos los eventos")
	}
	if !subscribed([]string{"notification.sent"}, "notification.sent") {
		t.Error("debería estar suscrito")
	}
	if subscribed([]string{"notification.failed"}, "notification.sent") {
		t.Error("no debería estar suscrito a otro evento")
	}
}

// TestDeliver_SignsAndPosts verifica que Deliver entrega un POST firmado al
// endpoint suscrito, con firma HMAC válida y cabeceras correctas.
func TestDeliver_SignsAndPosts(t *testing.T) {
	db := storetest.NewPostgres(t)
	ctx := context.Background()
	enc, _ := crypto.NewEncryptor(make([]byte, 32))

	app, err := db.Q.CreateApplication(ctx, sqlc.CreateApplicationParams{Name: "Acme", Slug: "acme"})
	if err != nil {
		t.Fatalf("CreateApplication: %v", err)
	}

	var notifID uuid.UUID
	if err := db.Pool.QueryRow(ctx, `
		INSERT INTO notifications (application_id, template_key, notification_type, channel, recipient, payload, status)
		VALUES ($1,'welcome','transactional','EMAIL','{}'::jsonb,'{}'::jsonb,'SENT') RETURNING id`, app.ID).Scan(&notifID); err != nil {
		t.Fatalf("insert notification: %v", err)
	}

	const secret = "super-secret-firma"
	secretEnc, _ := enc.Encrypt([]byte(secret))

	var gotBody []byte
	var gotSig, gotEvent, gotTS string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		gotSig = r.Header.Get("X-QBN-Signature")
		gotEvent = r.Header.Get("X-QBN-Event")
		gotTS = r.Header.Get("X-QBN-Timestamp")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO webhook_endpoints (application_id, url, secret_enc, events)
		VALUES ($1,$2,$3,$4)`, app.ID, srv.URL, secretEnc, []string{"notification.sent"}); err != nil {
		t.Fatalf("insert webhook: %v", err)
	}

	// El destino es loopback (httptest); habilitamos el opt-in que en producción
	// permite webhooks a la red privada, para que el guard anti-SSRF no lo bloquee.
	safedial.SetAllowPrivate(true)
	t.Cleanup(func() { safedial.SetAllowPrivate(false) })

	svc := NewService(db, enc, slog.Default())
	if err := svc.Deliver(ctx, queue.WebhookPayload{ApplicationID: app.ID, NotificationID: notifID, Event: "notification.sent"}); err != nil {
		t.Fatalf("Deliver: %v", err)
	}

	if gotEvent != "notification.sent" {
		t.Errorf("X-QBN-Event = %q", gotEvent)
	}
	// La firma debe ser HMAC-SHA256(secret, "<ts>.<body>").
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(gotTS))
	mac.Write([]byte("."))
	mac.Write(gotBody)
	want := hex.EncodeToString(mac.Sum(nil))
	if gotSig != want {
		t.Errorf("firma inválida:\n got %s\nwant %s", gotSig, want)
	}
	if _, err := strconv.ParseInt(gotTS, 10, 64); err != nil {
		t.Errorf("timestamp no numérico: %q", gotTS)
	}
}

// TestDeliver_FailingEndpointReturnsError (M7): si el endpoint responde 5xx,
// Deliver devuelve error para que asynq reintente la entrega.
func TestDeliver_FailingEndpointReturnsError(t *testing.T) {
	db := storetest.NewPostgres(t)
	ctx := context.Background()
	enc, _ := crypto.NewEncryptor(make([]byte, 32))

	app, _ := db.Q.CreateApplication(ctx, sqlc.CreateApplicationParams{Name: "Acme", Slug: "acme"})
	var notifID uuid.UUID
	if err := db.Pool.QueryRow(ctx, `
		INSERT INTO notifications (application_id, template_key, notification_type, channel, recipient, status)
		VALUES ($1,'welcome','transactional','EMAIL','{}'::jsonb,'SENT') RETURNING id`, app.ID).Scan(&notifID); err != nil {
		t.Fatalf("insert notification: %v", err)
	}

	secretEnc, _ := enc.Encrypt([]byte("s"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO webhook_endpoints (application_id, url, secret_enc, events)
		VALUES ($1,$2,$3,$4)`, app.ID, srv.URL, secretEnc, []string{"notification.sent"}); err != nil {
		t.Fatalf("insert webhook: %v", err)
	}

	safedial.SetAllowPrivate(true)
	t.Cleanup(func() { safedial.SetAllowPrivate(false) })

	svc := NewService(db, enc, slog.Default())
	if err := svc.Deliver(ctx, queue.WebhookPayload{ApplicationID: app.ID, NotificationID: notifID, Event: "notification.sent"}); err == nil {
		t.Fatal("un 5xx del endpoint debería devolver error (para que asynq reintente)")
	}
}

// TestDeliver_BlocksInternalSSRF (H6): con el guard activo (por defecto, sin
// opt-in), entregar a un endpoint loopback falla y el destino no recibe nada.
func TestDeliver_BlocksInternalSSRF(t *testing.T) {
	db := storetest.NewPostgres(t)
	ctx := context.Background()
	enc, _ := crypto.NewEncryptor(make([]byte, 32))

	app, _ := db.Q.CreateApplication(ctx, sqlc.CreateApplicationParams{Name: "Acme", Slug: "acme"})
	var notifID uuid.UUID
	if err := db.Pool.QueryRow(ctx, `
		INSERT INTO notifications (application_id, template_key, notification_type, channel, recipient, status)
		VALUES ($1,'welcome','transactional','EMAIL','{}'::jsonb,'SENT') RETURNING id`, app.ID).Scan(&notifID); err != nil {
		t.Fatalf("insert notification: %v", err)
	}

	secretEnc, _ := enc.Encrypt([]byte("s"))
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO webhook_endpoints (application_id, url, secret_enc, events)
		VALUES ($1,$2,$3,$4)`, app.ID, srv.URL, secretEnc, []string{"notification.sent"}); err != nil {
		t.Fatalf("insert webhook: %v", err)
	}

	// Sin SetAllowPrivate: el guard anti-SSRF (A2) debe bloquear el POST a loopback.
	svc := NewService(db, enc, slog.Default())
	if err := svc.Deliver(ctx, queue.WebhookPayload{ApplicationID: app.ID, NotificationID: notifID, Event: "notification.sent"}); err == nil {
		t.Fatal("Deliver a un endpoint loopback debería fallar por el guard anti-SSRF")
	}
	if called {
		t.Error("el endpoint interno NO debería recibir la petición")
	}
}

// TestDeliver_SkipsUnsubscribed: un endpoint no suscrito al evento no recibe nada.
func TestDeliver_SkipsUnsubscribed(t *testing.T) {
	db := storetest.NewPostgres(t)
	ctx := context.Background()
	enc, _ := crypto.NewEncryptor(make([]byte, 32))

	app, _ := db.Q.CreateApplication(ctx, sqlc.CreateApplicationParams{Name: "Acme", Slug: "acme"})
	var notifID uuid.UUID
	_ = db.Pool.QueryRow(ctx, `
		INSERT INTO notifications (application_id, template_key, notification_type, channel, recipient, status)
		VALUES ($1,'welcome','transactional','EMAIL','{}'::jsonb,'FAILED') RETURNING id`, app.ID).Scan(&notifID)

	secretEnc, _ := enc.Encrypt([]byte("s"))
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { called = true }))
	defer srv.Close()
	_, _ = db.Pool.Exec(ctx, `
		INSERT INTO webhook_endpoints (application_id, url, secret_enc, events)
		VALUES ($1,$2,$3,$4)`, app.ID, srv.URL, secretEnc, []string{"notification.sent"})

	svc := NewService(db, enc, slog.Default())
	if err := svc.Deliver(ctx, queue.WebhookPayload{ApplicationID: app.ID, NotificationID: notifID, Event: "notification.failed"}); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if called {
		t.Error("el endpoint no suscrito no debería recibir el evento")
	}
}
