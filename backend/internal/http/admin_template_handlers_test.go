package http

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"dvtech/qbn/internal/template"
)

// postWithApp arma un POST con cuerpo y el URLParam appId de chi (sin router).
func postWithApp(appID, body string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("appId", appID)
	return httptest.NewRequest("POST", "/", strings.NewReader(body)).
		WithContext(context.WithValue(context.Background(), chi.RouteCtxKey, rctx))
}

// TestTemplateCreateValidation cubre las rutas de validación de Create que cortan
// antes de tocar el servicio (svc nil no se alcanza).
func TestTemplateCreateValidation(t *testing.T) {
	h := NewAdminTemplateHandler(nil)
	app := uuid.New().String()
	call := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.Create(rec, postWithApp(app, body))
		return rec
	}

	t.Run("faltan key/name → 422 missing_fields", func(t *testing.T) {
		rec := call(`{"channel":"EMAIL"}`)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("got %d", rec.Code)
		}
		if c := problemCode(t, rec.Body.Bytes()); c != "missing_fields" {
			t.Fatalf("code = %q", c)
		}
	})

	t.Run("JSON malformado → 422 invalid_json", func(t *testing.T) {
		rec := call(`{not json`)
		if c := problemCode(t, rec.Body.Bytes()); c != "invalid_json" {
			t.Fatalf("code = %q", c)
		}
	})

	t.Run("baseTemplateId inválido → 422 invalid_base_template_id", func(t *testing.T) {
		rec := call(`{"key":"k","name":"n","baseTemplateId":"no-es-uuid"}`)
		if c := problemCode(t, rec.Body.Bytes()); c != "invalid_base_template_id" {
			t.Fatalf("code = %q", c)
		}
	})

	t.Run("appId inválido → 422 invalid_app_id", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.Create(rec, postWithApp("no-uuid", `{"key":"k","name":"n"}`))
		if c := problemCode(t, rec.Body.Bytes()); c != "invalid_app_id" {
			t.Fatalf("code = %q", c)
		}
	})
}

// TestPreviewDraftHandlerHTML ejercita el handler de preview en modo html con un
// Service real (PreviewDraft no toca la BD, así que db nil basta): valida que el
// saneado y el render funcionan de extremo a extremo a nivel HTTP.
func TestPreviewDraftHandlerHTML(t *testing.T) {
	h := NewAdminTemplateHandler(template.NewService(nil, slog.Default()))
	app := uuid.New().String()

	body := `{"channel":"EMAIL","kind":"html","subject":"Hola {{n}}","body":"<p>{{n}}</p><script>alert(1)</script>","payload":{"n":"Jane"}}`
	rec := httptest.NewRecorder()
	h.PreviewDraft(rec, postWithApp(app, body))

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d: %s", rec.Code, rec.Body)
	}
	var res template.PreviewResult
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("respuesta no válida: %v", err)
	}
	if strings.Contains(res.HTML, "<script") {
		t.Fatalf("el script debería estar saneado: %q", res.HTML)
	}
	if !strings.Contains(res.HTML, "Jane") {
		t.Fatalf("debería renderizar el payload: %q", res.HTML)
	}
	if res.Subject != "Hola Jane" {
		t.Fatalf("subject renderizado inesperado: %q", res.Subject)
	}
}

// TestPreviewDraftHandlerBuilder: modo builder compila la estructura y renderiza.
func TestPreviewDraftHandlerBuilder(t *testing.T) {
	h := NewAdminTemplateHandler(template.NewService(nil, slog.Default()))
	app := uuid.New().String()

	body := `{"channel":"EMAIL","subject":"x","structure":{"sections":[{"id":"s1","type":"text","props":{"text":"Hola {{n}}"}}]},"payload":{"n":"Ana"}}`
	rec := httptest.NewRecorder()
	h.PreviewDraft(rec, postWithApp(app, body))

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d: %s", rec.Code, rec.Body)
	}
	var res template.PreviewResult
	_ = json.Unmarshal(rec.Body.Bytes(), &res)
	if !strings.Contains(res.HTML, "Ana") {
		t.Fatalf("debería renderizar el payload: %q", res.HTML)
	}
}
