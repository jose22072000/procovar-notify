package notification_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"

	"dvtech/qbn/internal/channels"
	"dvtech/qbn/internal/store/sqlc"
)

// attempt devuelve un seam de metadatos de intento con retry/max fijos, para
// ejercitar las ramas que dependen del nº de reintento (asynq no es construible
// desde fuera de su paquete).
func attempt(retry, max int) func(context.Context) (int, int, string) {
	return func(context.Context) (int, int, string) { return retry, max, "task-test" }
}

// lastAttempt devuelve el último delivery_attempt (mayor attempt_number).
func lastAttempt(t *testing.T, f fixture, id uuid.UUID) sqlc.DeliveryAttempt {
	t.Helper()
	attempts, err := f.db.Q.ListDeliveryAttemptsByNotification(context.Background(), id)
	must(t, err)
	if len(attempts) == 0 {
		t.Fatal("no hay delivery attempts")
	}
	return attempts[len(attempts)-1]
}

func hasLogEvent(t *testing.T, f fixture, id uuid.UUID, event string) bool {
	t.Helper()
	logs, err := f.db.Q.ListNotificationLogsByNotification(context.Background(), pgtype.UUID{Bytes: [16]byte(id), Valid: true})
	must(t, err)
	for _, l := range logs {
		if l.Event == event {
			return true
		}
	}
	return false
}

// TestRetryBranchMarksRetry (H2): con reintentos disponibles, un fallo de envío
// deja el intento en RETRY, re-encola (QUEUED) y registra un log "retry".
func TestRetryBranchMarksRetry(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	n, err := f.service(&fakeEnqueuer{}).Create(ctx, baseInput(f.appID))
	must(t, err)

	sender := &fakeSender{channel: "EMAIL", err: errors.New("smtp caído")}
	proc := f.processor(sender).WithAttemptInfo(attempt(0, 3)) // quedan reintentos

	if err := proc.ProcessSend(ctx, n.ID); err == nil {
		t.Fatal("ProcessSend debería devolver el error (asynq reintenta)")
	}

	got, _ := f.db.Q.GetNotificationByID(ctx, n.ID)
	if string(got.Status) != "QUEUED" { // re-encolada entre reintentos (§3)
		t.Fatalf("estado esperado QUEUED, got %s", got.Status)
	}
	la := lastAttempt(t, f, n.ID)
	if la.Status != sqlc.DeliveryStatusRETRY {
		t.Fatalf("el intento debería ser RETRY, got %s", la.Status)
	}
	if !hasLogEvent(t, f, n.ID, "retry") {
		t.Fatal("debería haber un log de evento 'retry'")
	}
}

// TestRetryExhaustedMarksFailed (H2): sin reintentos restantes, el intento
// queda FAILED y la notificación FAILED.
func TestRetryExhaustedMarksFailed(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	n, err := f.service(&fakeEnqueuer{}).Create(ctx, baseInput(f.appID))
	must(t, err)

	sender := &fakeSender{channel: "EMAIL", err: errors.New("smtp caído")}
	proc := f.processor(sender).WithAttemptInfo(attempt(3, 3)) // agotados

	if err := proc.ProcessSend(ctx, n.ID); err == nil {
		t.Fatal("ProcessSend debería devolver el error")
	}
	got, _ := f.db.Q.GetNotificationByID(ctx, n.ID)
	if string(got.Status) != "FAILED" {
		t.Fatalf("estado esperado FAILED, got %s", got.Status)
	}
	if la := lastAttempt(t, f, n.ID); la.Status != sqlc.DeliveryStatusFAILED {
		t.Fatalf("el intento debería ser FAILED, got %s", la.Status)
	}
}

// TestFailPermanentRouteError (H4): sin ruta utilizable, falla de forma
// permanente (SkipRetry) con error_code route_error y sin tocar el sender.
func TestFailPermanentRouteError(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	n, err := f.service(&fakeEnqueuer{}).Create(ctx, baseInput(f.appID))
	must(t, err)

	// Quitar la ruta tras crear la notificación: la resolución fallará al enviar.
	_, err = f.db.Pool.Exec(ctx, `DELETE FROM channel_routes WHERE application_id = $1`, f.appID)
	must(t, err)

	sender := &fakeSender{channel: "EMAIL"}
	err = f.processor(sender).ProcessSend(ctx, n.ID)
	if !errors.Is(err, asynq.SkipRetry) {
		t.Fatalf("debería ser SkipRetry (no reintentar), got %v", err)
	}
	if sender.sent != 0 {
		t.Fatalf("el sender no debería invocarse sin ruta, got %d", sender.sent)
	}
	got, _ := f.db.Q.GetNotificationByID(ctx, n.ID)
	if string(got.Status) != "FAILED" {
		t.Fatalf("estado esperado FAILED, got %s", got.Status)
	}
	if la := lastAttempt(t, f, n.ID); la.ErrorCode == nil || *la.ErrorCode != "route_error" {
		t.Fatalf("error_code esperado route_error, got %v", la.ErrorCode)
	}
}

// TestRouteWithoutSMTPIsPermanent (L2): una ruta EMAIL sin conexión SMTP (id
// NULL) es un problema de configuración: el envío falla permanentemente
// (route_error, SkipRetry), no se reintenta. Distingue del caso transitorio
// (infra), que sí se reintentaría.
func TestRouteWithoutSMTPIsPermanent(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	n, err := f.service(&fakeEnqueuer{}).Create(ctx, baseInput(f.appID))
	must(t, err)

	// Deja la ruta sin conexión SMTP (config inválida, no fallo de infra).
	_, err = f.db.Pool.Exec(ctx, `UPDATE channel_routes SET smtp_connection_id = NULL WHERE application_id = $1`, f.appID)
	must(t, err)

	sendErr := f.processor(&fakeSender{channel: "EMAIL"}).ProcessSend(ctx, n.ID)
	if !errors.Is(sendErr, asynq.SkipRetry) {
		t.Fatalf("una ruta sin SMTP debería ser permanente (SkipRetry), got %v", sendErr)
	}
	got, _ := f.db.Q.GetNotificationByID(ctx, n.ID)
	if string(got.Status) != "FAILED" {
		t.Fatalf("estado esperado FAILED, got %s", got.Status)
	}
}

// TestFailPermanentRenderError (H4): si la plantilla no renderiza (Handlebars
// inválido), falla de forma permanente con error_code render_error.
func TestFailPermanentRenderError(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	n, err := f.service(&fakeEnqueuer{}).Create(ctx, baseInput(f.appID))
	must(t, err)

	// Corromper el body a Handlebars inválido (bloque sin cerrar) → Render falla.
	_, err = f.db.Pool.Exec(ctx,
		`UPDATE templates SET body = '<p>{{#each items}}</p>' WHERE application_id = $1 AND key = 'welcome'`, f.appID)
	must(t, err)

	sender := &fakeSender{channel: "EMAIL"}
	err = f.processor(sender).ProcessSend(ctx, n.ID)
	if !errors.Is(err, asynq.SkipRetry) {
		t.Fatalf("debería ser SkipRetry, got %v", err)
	}
	if sender.sent != 0 {
		t.Fatalf("el sender no debería invocarse si el render falla, got %d", sender.sent)
	}
	got, _ := f.db.Q.GetNotificationByID(ctx, n.ID)
	if string(got.Status) != "FAILED" {
		t.Fatalf("estado esperado FAILED, got %s", got.Status)
	}
	if la := lastAttempt(t, f, n.ID); la.ErrorCode == nil || *la.ErrorCode != "render_error" {
		t.Fatalf("error_code esperado render_error, got %v", la.ErrorCode)
	}
}

// TestCircuitBreakerOpens (H3): tras 3 fallos consecutivos al mismo destino el
// breaker se abre y la 4ª notificación se corta sin llamar al sender.
func TestCircuitBreakerOpens(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	sender := &fakeSender{channel: "EMAIL", err: errors.New("smtp caído")}
	// UN solo processor: comparte el breakerRegistry entre las llamadas.
	proc := f.processor(sender)

	var lastID uuid.UUID
	for i := 0; i < 4; i++ {
		n, err := f.service(&fakeEnqueuer{}).Create(ctx, baseInput(f.appID))
		must(t, err)
		lastID = n.ID
		_ = proc.ProcessSend(ctx, n.ID) // los 4 fallan (envío o breaker abierto)
	}

	// 3 ejecuciones reales + la 4ª cortada por el breaker abierto = sender.sent==3.
	if sender.sent != 3 {
		t.Fatalf("el breaker debería abrirse tras 3 fallos y cortar el 4º envío; sender.sent=%d", sender.sent)
	}
	// Con el breaker abierto el mensaje NO se pierde: queda QUEUED esperando la
	// recuperación del proveedor (reintento con retardo, sin consumir intentos).
	got, _ := f.db.Q.GetNotificationByID(ctx, lastID)
	if string(got.Status) != "QUEUED" {
		t.Fatalf("la 4ª notificación debería quedar QUEUED (retenida), got %s", got.Status)
	}
}

// TestProviderUnavailableIsNotFailure: el error de destino caído se clasifica
// Unavailable y para asynq no consume reintentos (IsFailure=false) — nada
// acaba FAILED por una caída del proveedor.
func TestProviderUnavailableIsNotFailure(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	sender := &fakeSender{channel: "EMAIL", err: errors.New("smtp caído")}
	proc := f.processor(sender)

	// Abre el breaker con 3 fallos y provoca el 4º (cortado por el breaker).
	var last uuid.UUID
	var lastErr error
	for i := 0; i < 4; i++ {
		n, err := f.service(&fakeEnqueuer{}).Create(ctx, baseInput(f.appID))
		must(t, err)
		last = n.ID
		lastErr = proc.ProcessSend(ctx, n.ID)
	}
	if !channels.IsUnavailable(lastErr) {
		t.Fatalf("con el breaker abierto el error debería ser Unavailable, got %v", lastErr)
	}
	// El intento queda auditado como provider_unavailable.
	var code string
	must(t, f.db.Pool.QueryRow(ctx,
		`SELECT coalesce(error_code,'') FROM delivery_attempts WHERE notification_id=$1 ORDER BY started_at DESC LIMIT 1`,
		last).Scan(&code))
	if code != "provider_unavailable" {
		t.Fatalf("error_code esperado provider_unavailable, got %q", code)
	}
}

// TestCircuitBreakerIgnoresPermanentErrors (L1): un error permanente (validación
// de datos) no cuenta como fallo de infra, así que el breaker NO se abre y el
// sender se sigue invocando en todas las pasadas.
func TestCircuitBreakerIgnoresPermanentErrors(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	sender := &fakeSender{channel: "EMAIL", err: channels.Permanent(errors.New("destinatario inválido"))}
	proc := f.processor(sender) // un solo processor: breaker compartido

	for i := 0; i < 5; i++ {
		n, err := f.service(&fakeEnqueuer{}).Create(ctx, baseInput(f.appID))
		must(t, err)
		_ = proc.ProcessSend(ctx, n.ID)
	}

	if sender.sent != 5 {
		t.Fatalf("el breaker no debería abrirse con errores permanentes; sender.sent=%d (esperado 5)", sender.sent)
	}
}

// TestHotPathCachesRouteAndTemplate: tras el primer envío, ruta+SMTP y
// plantilla se sirven de la caché del processor — un segundo envío funciona
// aunque las filas ya no existan en la BD (prueba observable de que no se
// re-consulta dentro del TTL).
func TestHotPathCachesRouteAndTemplate(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	sender := &fakeSender{channel: "EMAIL"}
	proc := f.processor(sender) // mismo processor: caché compartida

	n1, err := f.service(&fakeEnqueuer{}).Create(ctx, baseInput(f.appID))
	must(t, err)
	must(t, proc.ProcessSend(ctx, n1.ID))

	// Se crea la 2ª ANTES de borrar (Create valida plantilla/ruta en la api).
	n2, err := f.service(&fakeEnqueuer{}).Create(ctx, baseInput(f.appID))
	must(t, err)

	// Sin caché, esto rompería la resolución del worker.
	_, err = f.db.Pool.Exec(ctx, `DELETE FROM channel_routes WHERE application_id = $1`, f.appID)
	must(t, err)
	_, err = f.db.Pool.Exec(ctx, `UPDATE templates SET is_active = false WHERE application_id = $1`, f.appID)
	must(t, err)

	if err := proc.ProcessSend(ctx, n2.ID); err != nil {
		t.Fatalf("el 2º envío debería servirse de la caché: %v", err)
	}
	if sender.sent != 2 {
		t.Fatalf("esperaba 2 envíos, got %d", sender.sent)
	}
}

// TestRedeliveryDoesNotResend (#4): si el envío tuvo éxito pero el worker murió
// antes de marcar SENT (queda PROCESSING), la reentrega de asynq NO reenvía:
// detecta el intento SUCCESS previo, marca SENT y termina.
func TestRedeliveryDoesNotResend(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	sender := &fakeSender{channel: "EMAIL"}
	proc := f.processor(sender)

	n, err := f.service(&fakeEnqueuer{}).Create(ctx, baseInput(f.appID))
	must(t, err)
	must(t, proc.ProcessSend(ctx, n.ID)) // 1er envío: SUCCESS + SENT

	// Simular el crash post-envío/pre-marca: estado de vuelta a PROCESSING.
	_, err = f.db.Pool.Exec(ctx, `UPDATE notifications SET status='PROCESSING' WHERE id=$1`, n.ID)
	must(t, err)

	must(t, proc.ProcessSend(ctx, n.ID)) // reentrega
	if sender.sent != 1 {
		t.Fatalf("la reentrega no debe reenviar: esperaba 1 envío, got %d", sender.sent)
	}
	got, _ := f.db.Q.GetNotificationByID(ctx, n.ID)
	if string(got.Status) != "SENT" {
		t.Fatalf("debería reconciliarse a SENT, got %s", got.Status)
	}
}

// TestReaperReconcilesStuckProcessing (#4): PROCESSING atascada con intento
// SUCCESS => SENT sin reencolar; sin intento exitoso => reencolada.
func TestReaperReconcilesStuckProcessing(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	enq := &fakeEnqueuer{}
	svc := f.service(enq)

	withSuccess := insertNotifRow(t, f, "PROCESSING", 10*time.Minute, nil)
	_, err := f.db.Pool.Exec(ctx, `
		INSERT INTO delivery_attempts (notification_id, attempt_number, status, provider_ref)
		VALUES ($1, 1, 'SUCCESS', 'ref-1')`, withSuccess)
	must(t, err)
	withoutSuccess := insertNotifRow(t, f, "PROCESSING", 10*time.Minute, nil)
	fresh := insertNotifRow(t, f, "PROCESSING", 0, nil) // en vuelo: no tocar

	sent, requeued, err := svc.ReconcileStuckProcessing(ctx, 2*time.Minute, 100)
	must(t, err)
	if sent != 1 || requeued != 1 {
		t.Fatalf("esperaba 1 SENT y 1 reencolada, got sent=%d requeued=%d", sent, requeued)
	}
	s1, _ := f.db.Q.GetNotificationByID(ctx, withSuccess)
	if string(s1.Status) != "SENT" {
		t.Fatalf("con intento SUCCESS debería quedar SENT, got %s", s1.Status)
	}
	if len(enq.ids) != 1 || enq.ids[0] != withoutSuccess {
		t.Fatalf("solo la atascada sin éxito debía reencolarse, got %v", enq.ids)
	}
	s3, _ := f.db.Q.GetNotificationByID(ctx, fresh)
	if string(s3.Status) != "PROCESSING" {
		t.Fatalf("una PROCESSING fresca (en vuelo) no se toca, got %s", s3.Status)
	}
}
