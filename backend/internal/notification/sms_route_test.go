package notification_test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/google/uuid"

	"dvtech/qbn/internal/channels"
	"dvtech/qbn/internal/crypto"
	"dvtech/qbn/internal/notification"
	"dvtech/qbn/internal/store/sqlc"
	"dvtech/qbn/internal/storetest"
)

// capturingSender registra la ResolvedRoute con la que se le invoca, para
// comprobar que el processor resolvió y descifró la config del proveedor.
type capturingSender struct {
	channel  string
	gotRoute channels.ResolvedRoute
	gotMsg   channels.RenderedMessage
	sent     int
}

func (s *capturingSender) Channel() string { return s.channel }
func (s *capturingSender) Send(_ context.Context, msg channels.RenderedMessage, route channels.ResolvedRoute) (channels.ProviderRef, error) {
	s.sent++
	s.gotRoute = route
	s.gotMsg = msg
	return "sms-ref", nil
}

// TestSendFlowSMS_ResolvesProviderConfig verifica el camino de la Fase 10: una
// ruta SMS hacia un ChannelProvider Twilio cuyo config_enc cifrado se descifra y
// llega al Sender en route.Provider.
func TestSendFlowSMS_ResolvesProviderConfig(t *testing.T) {
	db := storetest.NewPostgres(t)
	ctx := context.Background()
	enc, _ := crypto.NewEncryptor(make([]byte, 32))

	app, err := db.Q.CreateApplication(ctx, sqlc.CreateApplicationParams{Name: "Acme", Slug: "acme"})
	must(t, err)

	// Template SMS.
	_, err = db.Pool.Exec(ctx, `
		INSERT INTO templates (application_id, key, channel, name, subject, structure, body, required_variables, locale, version)
		VALUES ($1,'sms-code','SMS','Código','Código','{}'::jsonb,'Tu código es {{code}}','{}'::jsonb,'en',1)`, app.ID)
	must(t, err)

	// ChannelProvider TWILIO con config cifrada.
	cfgEnc, err := enc.Encrypt([]byte(`{"accountSid":"AC123","authToken":"tok","from":"+15550001111"}`))
	must(t, err)
	var provID uuid.UUID
	must(t, db.Pool.QueryRow(ctx, `
		INSERT INTO channel_providers (application_id, channel, provider, name, config_enc)
		VALUES ($1,'SMS','TWILIO','twilio-main',$2) RETURNING id`, app.ID, cfgEnc).Scan(&provID))

	// Ruta SMS por defecto hacia el proveedor.
	_, err = db.Pool.Exec(ctx, `
		INSERT INTO channel_routes (application_id, notification_type, channel, template_key, channel_provider_id)
		VALUES ($1,'transactional','SMS','sms-code',$2)`, app.ID, provID)
	must(t, err)

	enq := &fakeEnqueuer{}
	svc := notification.NewService(db, enq, slog.Default())
	n, err := svc.Create(ctx, notification.CreateInput{
		ApplicationID:    app.ID,
		TemplateKey:      "sms-code",
		NotificationType: "transactional",
		Channel:          "SMS",
		Recipient:        map[string]any{"phone": "+15557778888"},
		Payload:          map[string]any{"code": "4321"},
	})
	must(t, err)
	if string(n.Status) != "QUEUED" {
		t.Fatalf("estado esperado QUEUED, got %s", n.Status)
	}

	sender := &capturingSender{channel: "SMS"}
	d := channels.NewDispatcher(sender, channels.NewInAppSender())
	proc := notification.NewProcessor(db, d, enc, slog.Default())
	must(t, proc.ProcessSend(ctx, n.ID))

	if sender.sent != 1 {
		t.Fatalf("el sender SMS debería invocarse 1 vez, got %d", sender.sent)
	}
	if sender.gotRoute.Provider == nil {
		t.Fatal("route.Provider no debería ser nil para SMS")
	}
	if sender.gotRoute.Provider.Provider != "TWILIO" {
		t.Errorf("Provider = %q, quería TWILIO", sender.gotRoute.Provider.Provider)
	}
	if got := sender.gotRoute.Provider.String("accountSid"); got != "AC123" {
		t.Errorf("config accountSid descifrado = %q, quería AC123", got)
	}
	if got := sender.gotRoute.Provider.String("from"); got != "+15550001111" {
		t.Errorf("config from descifrado = %q", got)
	}

	got, err := db.Q.GetNotificationByID(ctx, n.ID)
	must(t, err)
	if string(got.Status) != "SENT" {
		t.Fatalf("estado esperado SENT, got %s", got.Status)
	}
}
