package notification_test

import (
	"context"
	"strings"
	"testing"
)

// TestRenderFallbackToDefaultLocale: se pide un locale inexistente (fr) y el
// render cae al locale por defecto (en) de la misma key.
func TestRenderFallbackToDefaultLocale(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	in := baseInput(f.appID)
	fr := "fr"
	in.Locale = &fr // no existe variante 'fr'; sí 'en'

	n, err := f.service(&fakeEnqueuer{}).Create(ctx, in)
	must(t, err)

	sender := &capturingSender{channel: "EMAIL"}
	must(t, f.processor(sender).ProcessSend(ctx, n.ID))

	if sender.sent != 1 {
		t.Fatalf("el sender debería invocarse 1 vez, got %d", sender.sent)
	}
	if !strings.Contains(sender.gotMsg.HTMLBody, "Jane") {
		t.Errorf("el cuerpo debería renderizar la plantilla 'en': %q", sender.gotMsg.HTMLBody)
	}
	got, err := f.db.Q.GetNotificationByID(ctx, n.ID)
	must(t, err)
	if string(got.Status) != "SENT" {
		t.Fatalf("estado esperado SENT, got %s", got.Status)
	}
}

// TestRenderFallbackToAnyActiveLocale: ni el locale pedido (fr) ni el por
// defecto (en) existen para la key; cae a cualquier variante activa (es).
func TestRenderFallbackToAnyActiveLocale(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	// Template solo en 'es' para una key nueva.
	_, err := f.db.Pool.Exec(ctx, `
		INSERT INTO templates (application_id, key, channel, name, subject, structure, body, required_variables, locale, version, is_active)
		VALUES ($1,'solo-es','EMAIL','Solo ES','Asunto','{}'::jsonb,'<p>Hola {{firstName}} (ES)</p>','{}'::jsonb,'es',1,true)`, f.appID)
	must(t, err)

	in := baseInput(f.appID)
	in.TemplateKey = "solo-es"
	fr := "fr"
	in.Locale = &fr

	n, err := f.service(&fakeEnqueuer{}).Create(ctx, in)
	must(t, err)

	sender := &capturingSender{channel: "EMAIL"}
	must(t, f.processor(sender).ProcessSend(ctx, n.ID))

	if sender.sent != 1 || !strings.Contains(sender.gotMsg.HTMLBody, "(ES)") {
		t.Fatalf("debería caer a la variante 'es': sent=%d body=%q", sender.sent, sender.gotMsg.HTMLBody)
	}
}
