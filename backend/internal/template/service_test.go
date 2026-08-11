package template_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"dvtech/qbn/internal/crypto"
	"dvtech/qbn/internal/store"
	"dvtech/qbn/internal/store/sqlc"
	"dvtech/qbn/internal/storetest"
	"dvtech/qbn/internal/template"
)

const sampleStructure = `{"theme":{"primaryColor":"#0B5"},"sections":[
  {"id":"s1","type":"header","props":{"title":"{{appName}}"}},
  {"id":"s2","type":"text","props":{"text":"Hola {{firstName}}"}},
  {"id":"s3","type":"button","props":{"text":"Ir","url":"{{actionUrl}}"}}
]}`

// seedActor crea un super-admin para satisfacer el FK de auditoría.
func seedActor(t *testing.T, db *store.DB) uuid.UUID {
	t.Helper()
	hash, _ := crypto.HashPassword("x")
	a, err := db.Q.CreateAdminUser(context.Background(), sqlc.CreateAdminUserParams{
		Email: "root@x.com", PasswordHash: hash, Role: sqlc.AdminRoleSUPERADMIN,
	})
	if err != nil {
		t.Fatalf("create actor: %v", err)
	}
	return a.ID
}

func newAppAndSvc(t *testing.T) (appID, actor uuid.UUID, svc *template.Service) {
	t.Helper()
	db := storetest.NewPostgres(t)
	app, err := db.Q.CreateApplication(context.Background(), sqlc.CreateApplicationParams{Name: "Acme", Slug: "acme"})
	if err != nil {
		t.Fatalf("create app: %v", err)
	}
	return app.ID, seedActor(t, db), template.NewService(db, slog.Default())
}

func TestCreateCompilesAndDerives(t *testing.T) {
	ctx := context.Background()
	appID, actor, svc := newAppAndSvc(t)

	tpl, err := svc.Create(ctx, actor, appID, template.Input{
		Key:       "welcome",
		Name:      "Bienvenida",
		Subject:   "Hola {{firstName}}",
		Structure: json.RawMessage(sampleStructure),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if tpl.Version != 1 || !tpl.IsActive {
		t.Fatalf("debería ser versión 1 activa, got v%d active=%v", tpl.Version, tpl.IsActive)
	}
	if !strings.Contains(tpl.Body, "{{firstName}}") || !strings.Contains(tpl.Body, "<!doctype html>") {
		t.Fatal("el body compilado debería ser HTML con las variables intactas")
	}
	// required_variables derivadas: appName, firstName, actionUrl.
	for _, v := range []string{"appName", "firstName", "actionUrl"} {
		if !strings.Contains(string(tpl.RequiredVariables), v) {
			t.Errorf("required_variables debería incluir %q", v)
		}
	}

	// Crear de nuevo la misma key → conflicto.
	if _, err := svc.Create(ctx, actor, appID, template.Input{Key: "welcome", Name: "x", Structure: json.RawMessage(sampleStructure)}); err == nil {
		t.Fatal("recrear la misma key debería dar conflicto")
	}
}

func TestUpdateCreatesNewVersion(t *testing.T) {
	ctx := context.Background()
	appID, actor, svc := newAppAndSvc(t)

	v1, err := svc.Create(ctx, actor, appID, template.Input{Key: "welcome", Name: "B", Subject: "S", Structure: json.RawMessage(sampleStructure)})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	v2, err := svc.Update(ctx, actor, appID, v1.ID, template.Input{Name: "B v2"})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if v2.Version != 2 || !v2.IsActive {
		t.Fatalf("debería ser versión 2 activa, got v%d active=%v", v2.Version, v2.IsActive)
	}

	// La versión 1 quedó inactiva (pero existe).
	old, err := svc.Get(ctx, appID, v1.ID)
	if err != nil {
		t.Fatalf("get v1: %v", err)
	}
	if old.IsActive {
		t.Fatal("la versión 1 debería estar inactiva tras el update")
	}

	// Solo 1 template activo en el listado.
	active, err := svc.List(ctx, appID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(active) != 1 || active[0].Version != 2 {
		t.Fatalf("debería listar solo la v2 activa, got %d", len(active))
	}
}

func TestPreview(t *testing.T) {
	ctx := context.Background()
	appID, actor, svc := newAppAndSvc(t)
	tpl, err := svc.Create(ctx, actor, appID, template.Input{Key: "welcome", Name: "B", Subject: "Hola {{firstName}}", Structure: json.RawMessage(sampleStructure)})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	res, err := svc.Preview(ctx, appID, tpl.ID, map[string]any{"firstName": "Jane", "appName": "Acme", "actionUrl": "https://x"})
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if res.Subject != "Hola Jane" {
		t.Fatalf("subject renderizado inesperado: %q", res.Subject)
	}
	if !strings.Contains(res.HTML, "Hola Jane") || len(res.MissingVariables) != 0 {
		t.Fatalf("preview incompleto: missing=%v", res.MissingVariables)
	}

	// Payload incompleto → reporta variables faltantes (sin fallar).
	res2, err := svc.Preview(ctx, appID, tpl.ID, map[string]any{"firstName": "Jane"})
	if err != nil {
		t.Fatalf("preview2: %v", err)
	}
	if len(res2.MissingVariables) == 0 {
		t.Fatal("debería reportar variables faltantes")
	}
}

func TestCreateFromBaseTemplate(t *testing.T) {
	ctx := context.Background()
	db := storetest.NewPostgres(t)
	app, _ := db.Q.CreateApplication(ctx, sqlc.CreateApplicationParams{Name: "Acme", Slug: "acme"})
	actor := seedActor(t, db)
	svc := template.NewService(db, slog.Default())

	// Sembrar un BaseTemplate (la librería global está vacía en el test).
	var baseID uuid.UUID
	err := db.Pool.QueryRow(ctx, `
		INSERT INTO base_templates (key, channel, category, name, structure)
		VALUES ('welcome','EMAIL','transactional','Bienvenida', $1::jsonb) RETURNING id`,
		sampleStructure).Scan(&baseID)
	if err != nil {
		t.Fatalf("seed base: %v", err)
	}

	tpl, err := svc.Create(ctx, actor, app.ID, template.Input{
		Key:            "welcome",
		Name:           "Mi bienvenida",
		Subject:        "Hola {{firstName}}",
		BaseTemplateID: &baseID, // sin Structure → clona la del base
	})
	if err != nil {
		t.Fatalf("create from base: %v", err)
	}
	if !tpl.BaseTemplateID.Valid || uuid.UUID(tpl.BaseTemplateID.Bytes) != baseID {
		t.Fatal("debería referenciar el base_template de origen")
	}
	if !strings.Contains(tpl.Body, "{{appName}}") {
		t.Fatal("debería haber clonado la estructura del base")
	}
}

// TestTemplateActionsAudited verifica M7: crear y versionar un template deja
// rastro en admin_audit_logs con la acción correcta (template.create/update).
func TestTemplateActionsAudited(t *testing.T) {
	ctx := context.Background()
	db := storetest.NewPostgres(t)
	app, _ := db.Q.CreateApplication(ctx, sqlc.CreateApplicationParams{Name: "Acme", Slug: "acme"})
	actor := seedActor(t, db)
	svc := template.NewService(db, slog.Default())

	v1, err := svc.Create(ctx, actor, app.ID, template.Input{Key: "welcome", Name: "B", Subject: "S", Structure: json.RawMessage(sampleStructure)})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.Update(ctx, actor, app.ID, v1.ID, template.Input{Name: "B v2"}); err != nil {
		t.Fatalf("update: %v", err)
	}

	logs, err := db.Q.ListAuditLogsByApplication(ctx, sqlc.ListAuditLogsByApplicationParams{
		ApplicationID: pgUUID(app.ID), Limit: 10, Offset: 0,
	})
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	got := map[string]bool{}
	for _, l := range logs {
		if l.TargetType == "template" {
			got[l.Action] = true
		}
	}
	if !got["template.create"] || !got["template.update"] {
		t.Fatalf("faltan acciones de auditoría de templates, got=%v", got)
	}
}

func TestCreateHTMLMode(t *testing.T) {
	ctx := context.Background()
	appID, actor, svc := newAppAndSvc(t)

	tpl, err := svc.Create(ctx, actor, appID, template.Input{
		Key:     "promo",
		Name:    "Promo",
		Channel: "EMAIL",
		Kind:    template.KindHTML,
		Subject: "Hola {{firstName}}",
		Body:    `<html><body><p>Hola {{firstName}}, código {{otpCode}}</p><script>alert(1)</script><a href="{{url}}">ir</a></body></html>`,
	})
	if err != nil {
		t.Fatalf("create html: %v", err)
	}
	if tpl.Kind != template.KindHTML {
		t.Fatalf("kind debería ser html, got %q", tpl.Kind)
	}
	// El body queda saneado (sin script) pero con las variables intactas.
	if strings.Contains(tpl.Body, "<script") || strings.Contains(tpl.Body, "alert(") {
		t.Fatalf("el body html debería estar saneado: %q", tpl.Body)
	}
	if !strings.Contains(tpl.Body, "{{firstName}}") {
		t.Fatalf("el body debería conservar las variables: %q", tpl.Body)
	}
	// Variables derivadas del HTML crudo (subject + body).
	for _, v := range []string{"firstName", "otpCode", "url"} {
		if !strings.Contains(string(tpl.RequiredVariables), v) {
			t.Errorf("required_variables debería incluir %q", v)
		}
	}
}

func TestConvertBuilderToHTMLPreservesStructure(t *testing.T) {
	ctx := context.Background()
	appID, actor, svc := newAppAndSvc(t)

	v1, err := svc.Create(ctx, actor, appID, template.Input{
		Key: "welcome", Name: "B", Subject: "S", Structure: json.RawMessage(sampleStructure),
	})
	if err != nil {
		t.Fatalf("create builder: %v", err)
	}

	// Conversión a html: se conserva la structure de bloques para "volver a visual".
	v2, err := svc.Update(ctx, actor, appID, v1.ID, template.Input{
		Kind: template.KindHTML,
		Body: `<html><body><p>{{firstName}} en modo html</p></body></html>`,
	})
	if err != nil {
		t.Fatalf("convert to html: %v", err)
	}
	if v2.Kind != template.KindHTML {
		t.Fatalf("v2 debería ser html, got %q", v2.Kind)
	}
	if !strings.Contains(string(v2.Structure), "header") || !strings.Contains(string(v2.Structure), "appName") {
		t.Fatalf("la structure de bloques debería conservarse para revertir: %s", v2.Structure)
	}
	if !strings.Contains(v2.Body, "modo html") {
		t.Fatalf("el body debería ser el HTML nuevo: %q", v2.Body)
	}
}

func TestPreviewDraftHTMLNeutralizesTripleStache(t *testing.T) {
	// PreviewDraft no usa BD, así que un Service sin db basta.
	svc := template.NewService(nil, slog.Default())
	res, err := svc.PreviewDraft("EMAIL", template.KindHTML, nil, "x",
		"<p>{{{userHtml}}}</p>", nil,
		map[string]any{"userHtml": "<img src=x onerror=alert(1)>"})
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if strings.Contains(res.HTML, "<img src=x onerror") {
		t.Fatalf("el triple-stache no debería inyectar HTML sin escapar: %q", res.HTML)
	}
	if !strings.Contains(res.HTML, "&lt;img") {
		t.Fatalf("el valor del payload debería ir escapado: %q", res.HTML)
	}
}

func TestHTMLModeRejectsNonEmailChannel(t *testing.T) {
	ctx := context.Background()
	appID, actor, svc := newAppAndSvc(t)

	_, err := svc.Create(ctx, actor, appID, template.Input{
		Key: "x", Name: "X", Channel: "SMS", Kind: template.KindHTML, Body: "<p>hola</p>",
	})
	if err == nil {
		t.Fatal("el modo html en un canal que no es EMAIL debería fallar")
	}
}

// pgUUID convierte uuid.UUID al pgtype.UUID que espera sqlc.
func pgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}
