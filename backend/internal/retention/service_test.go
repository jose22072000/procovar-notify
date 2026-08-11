package retention_test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/google/uuid"

	"dvtech/qbn/internal/retention"
	"dvtech/qbn/internal/store/sqlc"
	"dvtech/qbn/internal/storetest"
)

func TestPurgeExpiredAnonymizes(t *testing.T) {
	db := storetest.NewPostgres(t)
	ctx := context.Background()
	app, err := db.Q.CreateApplication(ctx, sqlc.CreateApplicationParams{Name: "Acme", Slug: "acme"})
	if err != nil {
		t.Fatalf("create app: %v", err)
	}

	// Notificación antigua con PII.
	var nid uuid.UUID
	err = db.Pool.QueryRow(ctx, `
		INSERT INTO notifications (application_id, template_key, notification_type, channel, recipient, payload, status, created_at)
		VALUES ($1,'welcome','transactional','EMAIL','{"email":"x@a.test"}'::jsonb,'{"firstName":"Jane"}'::jsonb,'SENT', now() - interval '1 day')
		RETURNING id`, app.ID).Scan(&nid)
	if err != nil {
		t.Fatalf("insert notif: %v", err)
	}

	// Retención 0 días → la notificación de ayer está vencida.
	_, err = db.Pool.Exec(ctx, `UPDATE applications SET pii_retention_days = 0 WHERE id = $1`, app.ID)
	if err != nil {
		t.Fatalf("set retention: %v", err)
	}

	n, err := retention.NewService(db, slog.Default()).PurgeExpired(ctx)
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if n < 1 {
		t.Fatalf("debería haber anonimizado al menos 1, got %d", n)
	}

	var payload, recipient string
	must2(t, db.Pool.QueryRow(ctx, `SELECT payload::text, recipient::text FROM notifications WHERE id=$1`, nid).Scan(&payload, &recipient))
	if payload != "{}" || recipient != "{}" {
		t.Fatalf("payload/recipient deberían estar vacíos: payload=%s recipient=%s", payload, recipient)
	}
}

func must2(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
}

// TestRouteOverrideKeepsPIILonger: el Tipo puede retener PII más tiempo que el
// default de la app (era la limitación que motivó la retención por ruta).
func TestRouteOverrideKeepsPIILonger(t *testing.T) {
	db := storetest.NewPostgres(t)
	ctx := context.Background()
	app, err := db.Q.CreateApplication(ctx, sqlc.CreateApplicationParams{Name: "Acme", Slug: "acme"})
	must2(t, err)
	// App agresiva (0 días) pero el tipo 'invoice' retiene 365.
	_, err = db.Pool.Exec(ctx, `UPDATE applications SET pii_retention_days = 0 WHERE id = $1`, app.ID)
	must2(t, err)
	_, err = db.Pool.Exec(ctx, `
		INSERT INTO channel_routes (application_id, notification_type, channel, template_key, pii_retention_days)
		VALUES ($1,'invoice','IN_APP','invoice-tpl',365)`, app.ID)
	must2(t, err)

	var protectedID, expiredID uuid.UUID
	must2(t, db.Pool.QueryRow(ctx, `
		INSERT INTO notifications (application_id, template_key, notification_type, channel, recipient, payload, status, created_at)
		VALUES ($1,'invoice-tpl','invoice','IN_APP','{"userId":"u1"}'::jsonb,'{"total":"9"}'::jsonb,'SENT', now() - interval '1 day')
		RETURNING id`, app.ID).Scan(&protectedID))
	must2(t, db.Pool.QueryRow(ctx, `
		INSERT INTO notifications (application_id, template_key, notification_type, channel, recipient, payload, status, created_at)
		VALUES ($1,'welcome','transactional','EMAIL','{"email":"x@a.t"}'::jsonb,'{"n":"J"}'::jsonb,'SENT', now() - interval '1 day')
		RETURNING id`, app.ID).Scan(&expiredID))

	_, err = retention.NewService(db, slog.Default()).PurgeExpired(ctx)
	must2(t, err)

	var p1, p2 string
	must2(t, db.Pool.QueryRow(ctx, `SELECT payload::text FROM notifications WHERE id=$1`, protectedID).Scan(&p1))
	must2(t, db.Pool.QueryRow(ctx, `SELECT payload::text FROM notifications WHERE id=$1`, expiredID).Scan(&p2))
	if p1 == "{}" {
		t.Fatal("el override del Tipo (365d) debería proteger la PII frente al default de la app (0d)")
	}
	if p2 != "{}" {
		t.Fatal("sin override, el default de la app (0d) debería anonimizar")
	}
}

// TestDeleteExpiredCascades: el borrado por expires_at elimina la notificación
// terminal vencida con sus intentos y logs; respeta no-terminales y no vencidas.
func TestDeleteExpiredCascades(t *testing.T) {
	db := storetest.NewPostgres(t)
	ctx := context.Background()
	app, err := db.Q.CreateApplication(ctx, sqlc.CreateApplicationParams{Name: "Acme", Slug: "acme"})
	must2(t, err)

	insert := func(status string, expires string) uuid.UUID {
		var id uuid.UUID
		must2(t, db.Pool.QueryRow(ctx, `
			INSERT INTO notifications (application_id, template_key, notification_type, channel, recipient, payload, status, expires_at)
			VALUES ($1,'welcome','transactional','EMAIL','{}'::jsonb,'{}'::jsonb,$2,`+expires+`)
			RETURNING id`, app.ID, status).Scan(&id))
		return id
	}
	expired := insert("SENT", "now() - interval '1 hour'")
	pendingExpired := insert("PENDING", "now() - interval '1 hour'") // no terminal: no se borra
	alive := insert("SENT", "now() + interval '1 day'")
	forever := insert("SENT", "NULL") // sin TTL: no se borra nunca

	// Hijos de la vencida (deben caer por cascade).
	_, err = db.Pool.Exec(ctx, `INSERT INTO notification_logs (application_id, notification_id, event) VALUES ($1,$2,'sent')`, app.ID, expired)
	must2(t, err)

	_, err = retention.NewService(db, slog.Default()).PurgeExpired(ctx)
	must2(t, err)

	var count int
	must2(t, db.Pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE id = ANY($1)`,
		[]uuid.UUID{expired, pendingExpired, alive, forever}).Scan(&count))
	if count != 3 {
		t.Fatalf("solo la SENT vencida debía borrarse (esperaba 3 supervivientes, got %d)", count)
	}
	var logs int
	must2(t, db.Pool.QueryRow(ctx, `SELECT count(*) FROM notification_logs WHERE notification_id=$1`, expired).Scan(&logs))
	if logs != 0 {
		t.Fatalf("los logs de la borrada debían caer por cascade, quedan %d", logs)
	}
}
