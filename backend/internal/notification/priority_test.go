package notification_test

import (
	"context"
	"testing"
	"time"
)

// TestPriorityInheritedFromRoute: la prioridad es política del servidor — el
// Tipo de notificación lleva send_priority y las notificaciones la heredan
// cuando el cliente no manda una explícita (#2 del análisis de robustez: un
// OTP debe ir a la cola critical sin depender del integrador).
func TestPriorityInheritedFromRoute(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	// Tipo urgente apuntando a la misma plantilla/SMTP del fixture.
	_, err := f.db.Pool.Exec(ctx, `
		INSERT INTO channel_routes (application_id, notification_type, channel, template_key, smtp_connection_id, send_priority)
		SELECT application_id, 'otp_email', channel, template_key, smtp_connection_id, 'URGENT'
		FROM channel_routes WHERE application_id = $1 AND notification_type = 'transactional'`, f.appID)
	must(t, err)

	// Sin priority en el request → hereda URGENT del tipo.
	in := baseInput(f.appID)
	in.Type = "otp_email"
	in.TemplateKey = "" // que lo resuelva el tipo
	n, err := f.service(&fakeEnqueuer{}).Create(ctx, in)
	must(t, err)
	if string(n.Priority) != "URGENT" {
		t.Fatalf("debería heredar URGENT del Tipo, got %s", n.Priority)
	}

	// Override explícito del cliente: gana sobre la del tipo.
	in2 := baseInput(f.appID)
	in2.Type = "otp_email"
	in2.TemplateKey = ""
	in2.Priority = "LOW"
	n2, err := f.service(&fakeEnqueuer{}).Create(ctx, in2)
	must(t, err)
	if string(n2.Priority) != "LOW" {
		t.Fatalf("el override explícito debería ganar, got %s", n2.Priority)
	}

	// Un tipo sin send_priority explícito sigue siendo NORMAL (default de la
	// columna): el comportamiento actual no cambia para las rutas existentes.
	in3 := baseInput(f.appID)
	n3, err := f.service(&fakeEnqueuer{}).Create(ctx, in3)
	must(t, err)
	if string(n3.Priority) != "NORMAL" {
		t.Fatalf("sin send_priority el default sigue siendo NORMAL, got %s", n3.Priority)
	}
}

// TestExpiresAtFromRouteRetention: el retention_days del Tipo fija expires_at
// al crear (vida útil de la notificación; el barrido la borra al vencer).
func TestExpiresAtFromRouteRetention(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	_, err := f.db.Pool.Exec(ctx, `UPDATE channel_routes SET retention_days = 30 WHERE application_id = $1 AND notification_type = 'transactional'`, f.appID)
	must(t, err)

	in := baseInput(f.appID)
	in.Type = "transactional"
	in.TemplateKey = ""
	n, err := f.service(&fakeEnqueuer{}).Create(ctx, in)
	must(t, err)
	if !n.ExpiresAt.Valid {
		t.Fatal("expires_at debería fijarse desde retention_days del Tipo")
	}
	days := time.Until(n.ExpiresAt.Time).Hours() / 24
	if days < 29 || days > 31 {
		t.Fatalf("expires_at debería ser ~30 días, got %.1f", days)
	}

	// Sin retention_days en el Tipo, expires_at queda NULL (no se borra nunca).
	in2 := baseInput(f.appID)
	n2, err := f.service(&fakeEnqueuer{}).Create(ctx, in2)
	must(t, err)
	if n2.ExpiresAt.Valid {
		t.Fatal("sin retention_days, expires_at debe quedar NULL")
	}
}
