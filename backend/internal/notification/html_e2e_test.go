package notification_test

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/google/uuid"

	"dvtech/qbn/internal/channels"
	"dvtech/qbn/internal/crypto"
	"dvtech/qbn/internal/notification"
	"dvtech/qbn/internal/store/sqlc"
	"dvtech/qbn/internal/storetest"
	"dvtech/qbn/internal/template"
)

// TestHTMLTemplateSendE2E cubre el flujo completo de una plantilla en MODO HTML:
// crear con template.Service (el HTML se sanea) → notificación → procesar → el
// sender recibe el HTML saneado y renderizado con el payload.
func TestHTMLTemplateSendE2E(t *testing.T) {
	ctx := context.Background()
	db := storetest.NewPostgres(t)
	enc, _ := crypto.NewEncryptor(make([]byte, 32))

	app, err := db.Q.CreateApplication(ctx, sqlc.CreateApplicationParams{Name: "Acme", Slug: "acme"})
	must(t, err)
	hash, _ := crypto.HashPassword("x")
	admin, err := db.Q.CreateAdminUser(ctx, sqlc.CreateAdminUserParams{Email: "a@a.com", PasswordHash: hash, Role: sqlc.AdminRoleSUPERADMIN})
	must(t, err)

	// 1) Plantilla EMAIL en modo HTML (con <script> que debe sanearse).
	_, err = template.NewService(db, slog.Default()).Create(ctx, admin.ID, app.ID, template.Input{
		Key:     "welcome",
		Name:    "Bienvenida",
		Channel: "EMAIL",
		Kind:    template.KindHTML,
		Subject: "Hola {{firstName}}",
		Body:    "<p>Hola {{firstName}}</p><script>alert(1)</script>",
	})
	must(t, err)

	// 2) SMTP + ruta (transactional/EMAIL → plantilla "welcome").
	pwd, _ := enc.Encrypt([]byte(""))
	var smtpID uuid.UUID
	must(t, db.Pool.QueryRow(ctx, `
		INSERT INTO smtp_connections (application_id, name, host, port, username, password_enc, from_email, from_name, secure)
		VALUES ($1,'mailhog','localhost',1025,'',$2,'noreply@acme.test','Acme',false) RETURNING id`, app.ID, pwd).Scan(&smtpID))
	_, err = db.Pool.Exec(ctx, `
		INSERT INTO channel_routes (application_id, notification_type, channel, template_key, smtp_connection_id)
		VALUES ($1,'transactional','EMAIL','welcome',$2)`, app.ID, smtpID)
	must(t, err)

	// 3) Crear la notificación (payload satisface la variable requerida firstName).
	n, err := notification.NewService(db, &fakeEnqueuer{}, slog.Default()).Create(ctx, notification.CreateInput{
		ApplicationID:    app.ID,
		TemplateKey:      "welcome",
		NotificationType: "transactional",
		Channel:          "EMAIL",
		Recipient:        map[string]any{"email": "user@acme.test", "name": "Jane"},
		Payload:          map[string]any{"firstName": "Jane"},
	})
	must(t, err)

	// 4) Procesar → el sender recibe el mensaje renderizado.
	sender := &capturingSender{channel: "EMAIL"}
	d := channels.NewDispatcher(sender, channels.NewInAppSender())
	must(t, notification.NewProcessor(db, d, enc, slog.Default()).ProcessSend(ctx, n.ID))

	if sender.sent != 1 {
		t.Fatalf("el sender debería invocarse una vez, got %d", sender.sent)
	}
	if sender.gotMsg.Subject != "Hola Jane" {
		t.Fatalf("subject renderizado inesperado: %q", sender.gotMsg.Subject)
	}
	if strings.Contains(sender.gotMsg.HTMLBody, "<script") {
		t.Fatalf("el HTML enviado debería ir saneado: %q", sender.gotMsg.HTMLBody)
	}
	if !strings.Contains(sender.gotMsg.HTMLBody, "Hola Jane") {
		t.Fatalf("el HTML debería renderizar el payload: %q", sender.gotMsg.HTMLBody)
	}
}
