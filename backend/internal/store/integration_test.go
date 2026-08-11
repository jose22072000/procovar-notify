package store_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"dvtech/qbn/internal/crypto"
	"dvtech/qbn/internal/store/sqlc"
	"dvtech/qbn/internal/storetest"
)

// newNotificationParams arma una notificación mínima válida para el tenant dado.
func newNotificationParams(app sqlc.Application) sqlc.CreateNotificationParams {
	return sqlc.CreateNotificationParams{
		ApplicationID:    app.ID,
		TemplateKey:      "welcome",
		NotificationType: "transactional",
		Channel:          sqlc.ChannelEMAIL,
		Recipient:        json.RawMessage(`{"email":"user@acme.com"}`),
		Payload:          json.RawMessage(`{"name":"Jane"}`),
		Priority:         sqlc.PriorityNORMAL,
		Status:           sqlc.NotificationStatusPENDING,
		MaxRetries:       3,
	}
}

// TestTenantIsolation es el test PERMANENTE de aislamiento: un tenant nunca debe
// poder leer recursos de otro. Debe mantenerse en verde para siempre.
func TestTenantIsolation(t *testing.T) {
	db := storetest.NewPostgres(t)
	ctx := context.Background()
	q := db.Q

	appA, err := q.CreateApplication(ctx, sqlc.CreateApplicationParams{Name: "A", Slug: "app-a"})
	mustNoErr(t, err)
	appB, err := q.CreateApplication(ctx, sqlc.CreateApplicationParams{Name: "B", Slug: "app-b"})
	mustNoErr(t, err)

	notif, err := q.CreateNotification(ctx, newNotificationParams(appA))
	mustNoErr(t, err)

	// La lista del tenant B no ve la notificación de A.
	listB, err := q.ListNotificationsByApplication(ctx, sqlc.ListNotificationsByApplicationParams{
		ApplicationID: appB.ID, Limit: 100, Offset: 0,
	})
	mustNoErr(t, err)
	if len(listB) != 0 {
		t.Fatalf("el tenant B no debería ver notificaciones; vio %d", len(listB))
	}

	// La lista del tenant A sí la ve.
	listA, err := q.ListNotificationsByApplication(ctx, sqlc.ListNotificationsByApplicationParams{
		ApplicationID: appA.ID, Limit: 100, Offset: 0,
	})
	mustNoErr(t, err)
	if len(listA) != 1 {
		t.Fatalf("el tenant A debería ver 1 notificación; vio %d", len(listA))
	}

	// GetNotification con el application_id equivocado no devuelve la fila.
	_, err = q.GetNotification(ctx, sqlc.GetNotificationParams{ID: notif.ID, ApplicationID: appB.ID})
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("leer la notificación de A como B debería dar ErrNoRows; dio: %v", err)
	}

	// Con el application_id correcto sí.
	got, err := q.GetNotification(ctx, sqlc.GetNotificationParams{ID: notif.ID, ApplicationID: appA.ID})
	mustNoErr(t, err)
	if got.ID != notif.ID {
		t.Fatalf("se esperaba la notificación %v, se obtuvo %v", notif.ID, got.ID)
	}
}

// TestAPIKeySecretRoundTrip prueba que un secreto se cifra, se persiste como
// bytea y se recupera/descifra correctamente, y que los scopes (text[]) se
// preservan.
func TestAPIKeySecretRoundTrip(t *testing.T) {
	db := storetest.NewPostgres(t)
	ctx := context.Background()
	q := db.Q

	enc, err := crypto.NewEncryptor(make([]byte, 32))
	mustNoErr(t, err)

	app, err := q.CreateApplication(ctx, sqlc.CreateApplicationParams{Name: "Acme", Slug: "acme"})
	mustNoErr(t, err)

	const secret = "el-secreto-de-la-api-key"
	ciphertext, err := enc.Encrypt([]byte(secret))
	mustNoErr(t, err)

	created, err := q.CreateAPIKey(ctx, sqlc.CreateAPIKeyParams{
		ApplicationID: app.ID,
		KeyID:         "kid_abc123",
		SecretEnc:     ciphertext,
		Scopes:        []string{"notifications:send", "notifications:read"},
	})
	mustNoErr(t, err)

	got, err := q.GetAPIKeyByKeyID(ctx, "kid_abc123")
	mustNoErr(t, err)
	if got.ID != created.ID {
		t.Fatalf("api key id no coincide")
	}

	plaintext, err := enc.Decrypt(got.SecretEnc)
	mustNoErr(t, err)
	if string(plaintext) != secret {
		t.Fatalf("el secreto descifrado no coincide: %q", plaintext)
	}
	if len(got.Scopes) != 2 || got.Scopes[0] != "notifications:send" {
		t.Fatalf("scopes no preservados: %v", got.Scopes)
	}
}

// TestTxRollback verifica que un error dentro de Tx revierte los cambios.
func TestTxRollback(t *testing.T) {
	db := storetest.NewPostgres(t)
	ctx := context.Background()

	sentinel := errors.New("boom")
	err := db.Tx(ctx, func(q *sqlc.Queries) error {
		if _, err := q.CreateApplication(ctx, sqlc.CreateApplicationParams{Name: "Tmp", Slug: "tmp"}); err != nil {
			return err
		}
		return sentinel // fuerza rollback
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("se esperaba el error sentinel, se obtuvo: %v", err)
	}

	if _, err := db.Q.GetApplicationBySlug(ctx, "tmp"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("la app no debería existir tras el rollback; err=%v", err)
	}
}

func mustNoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
}
