package admin_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"dvtech/qbn/internal/admin"
	"dvtech/qbn/internal/crypto"
	"dvtech/qbn/internal/store"
	"dvtech/qbn/internal/store/sqlc"
	"dvtech/qbn/internal/storetest"
)

func setup(t *testing.T) (*store.DB, *admin.Service, *crypto.Encryptor, uuid.UUID) {
	t.Helper()
	db := storetest.NewPostgres(t)
	enc, _ := crypto.NewEncryptor(make([]byte, 32))
	svc := admin.NewService(db, enc, slog.Default())

	// Super-admin actor (necesario para el FK de auditoría).
	hash, _ := crypto.HashPassword("x")
	actor, err := db.Q.CreateAdminUser(context.Background(), sqlc.CreateAdminUserParams{
		Email: "root@x.com", PasswordHash: hash, Role: sqlc.AdminRoleSUPERADMIN,
	})
	if err != nil {
		t.Fatalf("create actor: %v", err)
	}
	return db, svc, enc, actor.ID
}

func TestApplicationLifecycleAndAudit(t *testing.T) {
	db, svc, _, actor := setup(t)
	ctx := context.Background()

	app, err := svc.CreateApplication(ctx, actor, "Acme", "acme")
	if err != nil {
		t.Fatalf("create app: %v", err)
	}

	// Slug duplicado → conflicto.
	if _, err := svc.CreateApplication(ctx, actor, "Acme2", "acme"); err == nil {
		t.Fatal("slug duplicado debería dar conflicto")
	}

	// Auditoría registrada para la app.
	logs, err := db.Q.ListAuditLogsByApplication(ctx, sqlc.ListAuditLogsByApplicationParams{
		ApplicationID: pgUUID(app.ID), Limit: 10, Offset: 0,
	})
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	if len(logs) == 0 || logs[len(logs)-1].Action != "application.create" {
		t.Fatalf("debería haberse auditado application.create: %+v", logs)
	}

	// Update sin cambios (name=nil, slug=nil, status=nil) devuelve la app actual, no en blanco.
	noop, err := svc.UpdateApplication(ctx, actor, app.ID, nil, nil, nil)
	if err != nil {
		t.Fatalf("update no-op: %v", err)
	}
	if noop.ID != app.ID || noop.Name != "Acme" {
		t.Fatalf("un update sin cambios debería devolver la app actual, got %+v", noop)
	}

	// Update con nombre nuevo.
	newName := "Acme Renamed"
	renamed, err := svc.UpdateApplication(ctx, actor, app.ID, &newName, nil, nil)
	if err != nil {
		t.Fatalf("update name: %v", err)
	}
	if renamed.Name != newName {
		t.Fatalf("nombre no actualizado: %+v", renamed)
	}

	// El slug también debe poder cambiarse (era inmutable: no había query ni campo).
	newSlug := "acme-renamed"
	reslugged, err := svc.UpdateApplication(ctx, actor, app.ID, nil, &newSlug, nil)
	if err != nil {
		t.Fatalf("update slug: %v", err)
	}
	if reslugged.Slug != newSlug {
		t.Fatalf("slug no actualizado: %+v", reslugged)
	}
	// Y el nombre anterior no se pisa al cambiar solo el slug.
	if reslugged.Name != newName {
		t.Fatalf("cambiar el slug pisó el nombre: %+v", reslugged)
	}
}

func TestCreateAPIKeyReturnsSecretOnceAndEncrypts(t *testing.T) {
	db, svc, enc, actor := setup(t)
	ctx := context.Background()
	app, _ := svc.CreateApplication(ctx, actor, "Acme", "acme")

	created, err := svc.CreateAPIKey(ctx, actor, app.ID, "qb-back", nil)
	if err != nil {
		t.Fatalf("create key: %v", err)
	}
	if created.Secret == "" || created.KeyID == "" {
		t.Fatal("debería devolver keyId y secret en claro")
	}

	// El secret persistido está cifrado y descifra al mismo valor.
	row, err := db.Q.GetAPIKeyByKeyID(ctx, created.KeyID)
	if err != nil {
		t.Fatalf("get key: %v", err)
	}
	plain, err := enc.Decrypt(row.SecretEnc)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if string(plain) != created.Secret {
		t.Fatal("el secret cifrado no coincide con el devuelto")
	}

	// Revoca y comprueba estado.
	if err := svc.RevokeAPIKey(ctx, actor, app.ID, created.ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	row, _ = db.Q.GetAPIKeyByKeyID(ctx, created.KeyID)
	if string(row.Status) != "REVOKED" {
		t.Fatalf("la key debería estar REVOKED, got %s", row.Status)
	}
}

func TestSMTPCreateAndIsolation(t *testing.T) {
	db, svc, enc, actor := setup(t)
	ctx := context.Background()
	appA, _ := svc.CreateApplication(ctx, actor, "A", "a")
	appB, _ := svc.CreateApplication(ctx, actor, "B", "b")

	c, err := svc.CreateSMTP(ctx, actor, appA.ID, admin.SMTPInput{
		Name: "mailhog", Host: "localhost", Port: 1025, Username: "", Password: "secreto",
		FromEmail: "n@a.test", FromName: "A", Secure: false,
	})
	if err != nil {
		t.Fatalf("create smtp: %v", err)
	}

	// El password se guardó cifrado.
	stored, _ := db.Q.GetSmtpConnection(ctx, sqlc.GetSmtpConnectionParams{ID: c.ID, ApplicationID: appA.ID})
	plain, _ := enc.Decrypt(stored.PasswordEnc)
	if string(plain) != "secreto" {
		t.Fatal("el password SMTP no se cifró/recuperó bien")
	}

	// El tenant B no ve la conexión de A.
	listB, _ := svc.ListSMTP(ctx, appB.ID)
	if len(listB) != 0 {
		t.Fatalf("aislamiento roto: B ve %d conexiones de A", len(listB))
	}
}

func TestRouteCreate(t *testing.T) {
	db, svc, _, actor := setup(t)
	ctx := context.Background()
	app, _ := svc.CreateApplication(ctx, actor, "Acme", "acme")
	smtp, _ := svc.CreateSMTP(ctx, actor, app.ID, admin.SMTPInput{Name: "m", Host: "h", Port: 25, FromEmail: "a@b.c", FromName: "x"})

	tplKey := "welcome"
	route, err := svc.CreateRoute(ctx, actor, app.ID, admin.RouteInput{
		NotificationType: "transactional", Channel: "EMAIL", TemplateKey: &tplKey, SMTPConnectionID: &smtp.ID,
	})
	if err != nil {
		t.Fatalf("create route: %v", err)
	}
	if route.NotificationType != "transactional" {
		t.Fatalf("ruta inesperada: %+v", route)
	}

	// La creación se audita como route.create y el borrado como route.delete
	// (antes ambos quedaban como route.update).
	if err := svc.DeleteRoute(ctx, actor, app.ID, route.ID); err != nil {
		t.Fatalf("delete route: %v", err)
	}
	logs, err := db.Q.ListAuditLogsByApplication(ctx, sqlc.ListAuditLogsByApplicationParams{
		ApplicationID: pgUUID(app.ID), Limit: 50, Offset: 0,
	})
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	actions := make([]string, len(logs))
	for i, l := range logs {
		actions[i] = l.Action
	}
	for _, want := range []string{"route.create", "route.delete"} {
		found := false
		for _, a := range actions {
			if a == want {
				found = true
			}
		}
		if !found {
			t.Errorf("falta la acción de auditoría %q; acciones: %v", want, actions)
		}
	}
}

// TestProviderConfigEncrypted (H7): CreateProvider cifra la config en reposo;
// la persistida no contiene el secreto en claro y descifra al valor original.
func TestProviderConfigEncrypted(t *testing.T) {
	db, svc, enc, actor := setup(t)
	ctx := context.Background()
	app, _ := svc.CreateApplication(ctx, actor, "Acme", "acme")

	const apiKey = "super-secret-fcm-key"
	p, err := svc.CreateProvider(ctx, actor, app.ID, admin.ProviderInput{
		Channel:  "PUSH",
		Provider: "FCM",
		Name:     "firebase",
		Config:   map[string]any{"server_key": apiKey, "project": "acme"},
	})
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}

	stored, err := db.Q.GetChannelProvider(ctx, sqlc.GetChannelProviderParams{ID: p.ID, ApplicationID: app.ID})
	if err != nil {
		t.Fatalf("get provider: %v", err)
	}
	// No debe verse el secreto en claro en la columna cifrada.
	if bytes.Contains(stored.ConfigEnc, []byte(apiKey)) {
		t.Fatal("la config_enc no debería contener el secreto en claro")
	}
	// Descifra y coincide con la config original.
	raw, err := enc.Decrypt(stored.ConfigEnc)
	if err != nil {
		t.Fatalf("decrypt config: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("config descifrada no es JSON: %v", err)
	}
	if cfg["server_key"] != apiKey || cfg["project"] != "acme" {
		t.Fatalf("config descifrada no coincide: %+v", cfg)
	}
}

// TestUpdateProvider (L20): la ruta PATCH ya existe; el update re-cifra la
// config nueva y audita provider.update.
func TestUpdateProvider(t *testing.T) {
	db, svc, enc, actor := setup(t)
	ctx := context.Background()
	app, _ := svc.CreateApplication(ctx, actor, "Acme", "acme")

	p, err := svc.CreateProvider(ctx, actor, app.ID, admin.ProviderInput{
		Channel: "PUSH", Provider: "FCM", Name: "firebase", Config: map[string]any{"k": "v1"},
	})
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}

	updated, err := svc.UpdateProvider(ctx, actor, app.ID, p.ID, admin.ProviderInput{
		Channel: "PUSH", Provider: "FCM", Name: "firebase", Config: map[string]any{"k": "v2"}, Status: "DISABLED",
	})
	if err != nil {
		t.Fatalf("update provider: %v", err)
	}
	if string(updated.Status) != "DISABLED" {
		t.Fatalf("estado esperado DISABLED, got %s", updated.Status)
	}

	// La config se re-cifró al nuevo valor.
	raw, err := enc.Decrypt(updated.ConfigEnc)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	var cfg map[string]any
	_ = json.Unmarshal(raw, &cfg)
	if cfg["k"] != "v2" {
		t.Fatalf("la config no se re-cifró: %+v", cfg)
	}

	// Se auditó provider.update.
	logs, _ := db.Q.ListAuditLogsByApplication(ctx, sqlc.ListAuditLogsByApplicationParams{
		ApplicationID: pgUUID(app.ID), Limit: 50, Offset: 0,
	})
	found := false
	for _, l := range logs {
		if l.Action == "provider.update" {
			found = true
		}
	}
	if !found {
		t.Error("debería auditarse provider.update")
	}
}

// TestWebhookSecretEncrypted (H7): CreateWebhook devuelve el secreto en claro
// una vez y lo persiste cifrado; la columna no lo contiene en claro y descifra
// al valor devuelto.
func TestWebhookSecretEncrypted(t *testing.T) {
	db, svc, enc, actor := setup(t)
	ctx := context.Background()
	app, _ := svc.CreateApplication(ctx, actor, "Acme", "acme")

	created, err := svc.CreateWebhook(ctx, actor, app.ID, "https://hooks.acme.test/qbn", []string{"notification.sent"})
	if err != nil {
		t.Fatalf("create webhook: %v", err)
	}
	if created.Secret == "" {
		t.Fatal("debería devolver el secreto en claro una vez")
	}

	eps, err := db.Q.ListWebhookEndpoints(ctx, app.ID)
	if err != nil {
		t.Fatalf("list webhooks: %v", err)
	}
	if len(eps) != 1 {
		t.Fatalf("debería haber 1 endpoint, got %d", len(eps))
	}
	stored := eps[0]
	if bytes.Contains(stored.SecretEnc, []byte(created.Secret)) {
		t.Fatal("la secret_enc no debería contener el secreto en claro")
	}
	plain, err := enc.Decrypt(stored.SecretEnc)
	if err != nil {
		t.Fatalf("decrypt secret: %v", err)
	}
	if string(plain) != created.Secret {
		t.Fatal("el secreto cifrado no coincide con el devuelto")
	}
}

// TestSuppressionAddValidationAndRoundtrip (M9): AddSuppression valida campos
// obligatorios; el alta y la baja se auditan y respetan el aislamiento por app.
func TestSuppressionAddValidationAndRoundtrip(t *testing.T) {
	db, svc, _, actor := setup(t)
	ctx := context.Background()
	app, _ := svc.CreateApplication(ctx, actor, "Acme", "acme")

	// Validación: faltan channel/recipient.
	if _, err := svc.AddSuppression(ctx, actor, app.ID, "", "", ""); err == nil {
		t.Error("AddSuppression sin channel/recipient debería fallar")
	}

	row, err := svc.AddSuppression(ctx, actor, app.ID, "EMAIL", "bounce@x.test", "")
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if row.Reason != "manual" { // reason vacío → "manual"
		t.Errorf("reason por defecto = %q, want manual", row.Reason)
	}

	// Listado del tenant lo incluye; otro tenant no.
	list, _ := svc.ListSuppressions(ctx, app.ID, 50, 0)
	if len(list) != 1 {
		t.Fatalf("debería listar 1 supresión, got %d", len(list))
	}
	otherApp, _ := svc.CreateApplication(ctx, actor, "Other", "other")
	if other, _ := svc.ListSuppressions(ctx, otherApp.ID, 50, 0); len(other) != 0 {
		t.Fatalf("otro tenant no debería ver supresiones ajenas, got %d", len(other))
	}

	// Baja + auditoría.
	if err := svc.DeleteSuppression(ctx, actor, app.ID, row.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	logs, _ := db.Q.ListAuditLogsByApplication(ctx, sqlc.ListAuditLogsByApplicationParams{
		ApplicationID: pgUUID(app.ID), Limit: 50, Offset: 0,
	})
	var add, rem bool
	for _, l := range logs {
		if l.Action == "suppression.add" {
			add = true
		}
		if l.Action == "suppression.remove" {
			rem = true
		}
	}
	if !add || !rem {
		t.Errorf("faltan auditorías suppression.add/remove (add=%v rem=%v)", add, rem)
	}
}

func pgUUID(id uuid.UUID) pgtype.UUID { return pgtype.UUID{Bytes: id, Valid: true} }
