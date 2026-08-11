package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"dvtech/qbn/internal/auth"
)

// problemCode decodifica el campo "code" del cuerpo problem+json de un error.
func problemCode(t *testing.T, body []byte) string {
	t.Helper()
	var p struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		t.Fatalf("la respuesta no es problem+json: %v (%s)", err, body)
	}
	return p.Code
}

// reqWithParam crea una petición con un URLParam de chi inyectado (sin montar un
// router completo) y el contexto base dado.
func reqWithParam(ctx context.Context, key, val string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, val)
	return httptest.NewRequest("GET", "/", nil).
		WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))
}

func TestClientIP(t *testing.T) {
	// Sin proxy de confianza (por defecto): XFF se ignora, se usa RemoteAddr (L9).
	t.Run("sin trust proxy ignora XFF", func(t *testing.T) {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = "192.0.2.9:5555"
		r.Header.Set("X-Forwarded-For", "203.0.113.7")
		if got := clientIP(r); got != "192.0.2.9" {
			t.Fatalf("clientIP = %q, want 192.0.2.9 (XFF spoofeable ignorado)", got)
		}
	})

	t.Run("remoteaddr sin puerto", func(t *testing.T) {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = "192.0.2.9"
		if got := clientIP(r); got != "192.0.2.9" {
			t.Fatalf("clientIP = %q, want 192.0.2.9", got)
		}
	})

	// Con proxy de confianza: se usa el primer salto de XFF.
	t.Run("con trust proxy usa XFF", func(t *testing.T) {
		SetTrustProxy(true)
		t.Cleanup(func() { SetTrustProxy(false) })
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = "10.0.0.1:1234"
		r.Header.Set("X-Forwarded-For", "203.0.113.7, 70.41.3.18")
		if got := clientIP(r); got != "203.0.113.7" {
			t.Fatalf("clientIP = %q, want 203.0.113.7", got)
		}
	})
}

func TestRequireAppAccess(t *testing.T) {
	appA, appB := uuid.New(), uuid.New()
	superAdmin := auth.Admin{ID: uuid.New(), Role: "SUPER_ADMIN"}
	appAdminA := auth.Admin{ID: uuid.New(), Role: "APP_ADMIN", ApplicationID: &appA}

	run := func(ctx context.Context, appID string) (int, string) {
		rec := httptest.NewRecorder()
		next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
		requireAppAccess(next).ServeHTTP(rec, reqWithParam(ctx, "appId", appID))
		return rec.Code, rec.Body.String()
	}

	t.Run("sin admin en contexto → 401", func(t *testing.T) {
		code, body := run(context.Background(), appA.String())
		if code != http.StatusUnauthorized {
			t.Fatalf("got %d", code)
		}
		if c := problemCode(t, []byte(body)); c != "unauthenticated" {
			t.Fatalf("code = %q", c)
		}
	})

	t.Run("appId inválido → 422", func(t *testing.T) {
		code, body := run(auth.ContextWithAdmin(context.Background(), superAdmin), "no-es-uuid")
		if code != http.StatusUnprocessableEntity {
			t.Fatalf("got %d", code)
		}
		if c := problemCode(t, []byte(body)); c != "invalid_app_id" {
			t.Fatalf("code = %q", c)
		}
	})

	t.Run("super admin accede a cualquier app", func(t *testing.T) {
		if code, _ := run(auth.ContextWithAdmin(context.Background(), superAdmin), appB.String()); code != http.StatusOK {
			t.Fatalf("got %d", code)
		}
	})

	t.Run("app admin accede a su app", func(t *testing.T) {
		if code, _ := run(auth.ContextWithAdmin(context.Background(), appAdminA), appA.String()); code != http.StatusOK {
			t.Fatalf("got %d", code)
		}
	})

	t.Run("app admin a otra app → 403 (aislamiento multi-tenant)", func(t *testing.T) {
		code, body := run(auth.ContextWithAdmin(context.Background(), appAdminA), appB.String())
		if code != http.StatusForbidden {
			t.Fatalf("got %d", code)
		}
		if c := problemCode(t, []byte(body)); c != "forbidden" {
			t.Fatalf("code = %q", c)
		}
	})
}

func TestAppIDParamAndParseID(t *testing.T) {
	valid := uuid.New()

	t.Run("appIDParam válido", func(t *testing.T) {
		rec := httptest.NewRecorder()
		got, ok := appIDParam(rec, reqWithParam(context.Background(), "appId", valid.String()))
		if !ok || got != valid {
			t.Fatalf("válido falló: ok=%v got=%v", ok, got)
		}
	})
	t.Run("appIDParam inválido → 422 invalid_app_id", func(t *testing.T) {
		rec := httptest.NewRecorder()
		if _, ok := appIDParam(rec, reqWithParam(context.Background(), "appId", "nope")); ok {
			t.Fatal("inválido debería devolver ok=false")
		}
		if c := problemCode(t, rec.Body.Bytes()); c != "invalid_app_id" {
			t.Fatalf("code = %q", c)
		}
	})
	t.Run("parseID válido", func(t *testing.T) {
		rec := httptest.NewRecorder()
		got, ok := parseID(rec, reqWithParam(context.Background(), "id", valid.String()))
		if !ok || got != valid {
			t.Fatalf("válido falló: ok=%v got=%v", ok, got)
		}
	})
	t.Run("parseID inválido → 422 invalid_id", func(t *testing.T) {
		rec := httptest.NewRecorder()
		if _, ok := parseID(rec, reqWithParam(context.Background(), "id", "nope")); ok {
			t.Fatal("inválido debería devolver ok=false")
		}
		if c := problemCode(t, rec.Body.Bytes()); c != "invalid_id" {
			t.Fatalf("code = %q", c)
		}
	})
}

// TestNotificationCreateValidation cubre las rutas de validación temprana de
// Create, que cortan ANTES de tocar el servicio (svc nil no se alcanza).
func TestNotificationCreateValidation(t *testing.T) {
	h := NewNotificationHandler(nil)
	caller := auth.Caller{ApplicationID: uuid.New(), Scopes: []string{"notifications:send"}}

	post := func(ctx context.Context, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/v1/notifications", strings.NewReader(body)).WithContext(ctx)
		rec := httptest.NewRecorder()
		h.Create(rec, r)
		return rec
	}
	withCaller := auth.ContextWithCaller(context.Background(), caller)

	t.Run("sin caller → 401", func(t *testing.T) {
		rec := post(context.Background(), `{"templateKey":"x","notificationType":"EMAIL"}`)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("got %d", rec.Code)
		}
		if c := problemCode(t, rec.Body.Bytes()); c != "unauthenticated" {
			t.Fatalf("code = %q", c)
		}
	})

	t.Run("campos faltantes → 422 missing_fields", func(t *testing.T) {
		rec := post(withCaller, `{}`)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("got %d", rec.Code)
		}
		if c := problemCode(t, rec.Body.Bytes()); c != "missing_fields" {
			t.Fatalf("code = %q", c)
		}
	})

	t.Run("scheduledAt no RFC3339 → 422 invalid_scheduled_at", func(t *testing.T) {
		rec := post(withCaller, `{"templateKey":"x","notificationType":"EMAIL","scheduledAt":"ayer"}`)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("got %d", rec.Code)
		}
		if c := problemCode(t, rec.Body.Bytes()); c != "invalid_scheduled_at" {
			t.Fatalf("code = %q", c)
		}
	})

	t.Run("body vacío → 422 empty_body", func(t *testing.T) {
		rec := post(withCaller, ``)
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("got %d", rec.Code)
		}
		if c := problemCode(t, rec.Body.Bytes()); c != "empty_body" {
			t.Fatalf("code = %q", c)
		}
	})
}

// TestCreateAPIKeyRejectsMalformedBody (L8): un cuerpo presente pero malformado
// (campo 'scope' en vez de 'scopes') debe dar 422, no descartarse en silencio y
// crear la key con scopes vacíos. El error corta antes de tocar el servicio.
func TestCreateAPIKeyRejectsMalformedBody(t *testing.T) {
	h := &AdminResourceHandler{} // svc nil no se alcanza en esta ruta
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("appId", uuid.New().String())
	r := httptest.NewRequest("POST", "/", strings.NewReader(`{"scope":["x"]}`)).
		WithContext(context.WithValue(context.Background(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()

	h.CreateAPIKey(rec, r)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("body malformado debería dar 422, got %d", rec.Code)
	}
	if c := problemCode(t, rec.Body.Bytes()); c != "invalid_json" {
		t.Fatalf("code = %q", c)
	}
}
