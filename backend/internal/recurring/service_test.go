package recurring_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"

	"dvtech/qbn/internal/notification"
	"dvtech/qbn/internal/recurring"
	"dvtech/qbn/internal/store/sqlc"
	"dvtech/qbn/internal/storetest"
)

type fakeEnq struct{ n int }

func (f *fakeEnq) EnqueueSend(_ context.Context, _ uuid.UUID, _ string, _ int, _ time.Time) error {
	f.n++
	return nil
}

func TestProcessCreatesNotification(t *testing.T) {
	db := storetest.NewPostgres(t)
	ctx := context.Background()
	app, err := db.Q.CreateApplication(ctx, sqlc.CreateApplicationParams{Name: "Acme", Slug: "acme"})
	if err != nil {
		t.Fatalf("create app: %v", err)
	}

	// Template (lo necesita Create para validar el payload).
	_, err = db.Pool.Exec(ctx, `
		INSERT INTO templates (application_id, key, channel, name, subject, structure, body, required_variables, locale, version)
		VALUES ($1,'welcome','EMAIL','B','S','{}'::jsonb,'<p>{{firstName}}</p>',
		        '{"type":"object","required":["firstName"],"properties":{"firstName":{"type":"string"}}}'::jsonb,'en',1)`, app.ID)
	if err != nil {
		t.Fatalf("insert template: %v", err)
	}

	// Horario recurrente.
	var schedID uuid.UUID
	err = db.Pool.QueryRow(ctx, `
		INSERT INTO recurring_schedules (application_id,name,template_key,notification_type,channel,recipient,payload,cron)
		VALUES ($1,'daily-digest','welcome','transactional','EMAIL','{"email":"u@a.test"}'::jsonb,'{"firstName":"U"}'::jsonb,'@daily')
		RETURNING id`, app.ID).Scan(&schedID)
	if err != nil {
		t.Fatalf("insert schedule: %v", err)
	}

	enq := &fakeEnq{}
	svc := recurring.NewService(db, notification.NewService(db, enq, slog.Default()), slog.Default())
	if err := svc.Process(ctx, schedID); err != nil {
		t.Fatalf("process: %v", err)
	}

	// Se creó (y encoló) una notificación para la app.
	var count int
	if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE application_id=$1`, app.ID).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 || enq.n != 1 {
		t.Fatalf("debería haberse creado y encolado 1 notificación; count=%d enq=%d", count, enq.n)
	}
}

// TestProcessTickIsIdempotent (#4): la reentrega del disparo (crash a mitad de
// Process) no crea una notificación duplicada — la clave determinista por
// (horario, minuto) hace que el segundo Create devuelva la misma fila.
func TestProcessTickIsIdempotent(t *testing.T) {
	db := storetest.NewPostgres(t)
	ctx := context.Background()
	app, err := db.Q.CreateApplication(ctx, sqlc.CreateApplicationParams{Name: "Acme", Slug: "acme"})
	if err != nil {
		t.Fatalf("create app: %v", err)
	}
	_, err = db.Pool.Exec(ctx, `
		INSERT INTO templates (application_id, key, channel, name, subject, structure, body, required_variables, locale, version)
		VALUES ($1,'welcome','EMAIL','B','S','{}'::jsonb,'<p>{{firstName}}</p>',
		        '{"type":"object","required":["firstName"],"properties":{"firstName":{"type":"string"}}}'::jsonb,'en',1)`, app.ID)
	if err != nil {
		t.Fatalf("insert template: %v", err)
	}
	var schedID uuid.UUID
	err = db.Pool.QueryRow(ctx, `
		INSERT INTO recurring_schedules (application_id,name,template_key,notification_type,channel,recipient,payload,cron)
		VALUES ($1,'daily-digest','welcome','transactional','EMAIL','{"email":"u@a.test"}'::jsonb,'{"firstName":"U"}'::jsonb,'@daily')
		RETURNING id`, app.ID).Scan(&schedID)
	if err != nil {
		t.Fatalf("insert schedule: %v", err)
	}

	enq := &fakeEnq{}
	svc := recurring.NewService(db, notification.NewService(db, enq, slog.Default()), slog.Default())
	// Dos ejecuciones del mismo tick (mismo minuto): simulan la reentrega.
	if err := svc.Process(ctx, schedID); err != nil {
		t.Fatalf("process 1: %v", err)
	}
	if err := svc.Process(ctx, schedID); err != nil {
		t.Fatalf("process 2: %v", err)
	}

	var count int
	if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE application_id=$1`, app.ID).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("la reentrega del tick no debe duplicar: esperaba 1 notificación, got %d", count)
	}
}
