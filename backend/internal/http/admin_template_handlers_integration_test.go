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

	"dvtech/qbn/internal/auth"
	"dvtech/qbn/internal/crypto"
	"dvtech/qbn/internal/store/sqlc"
	"dvtech/qbn/internal/storetest"
	"dvtech/qbn/internal/template"
)

// TestTemplateHandlerCRUD ejercita el camino completo handler→servicio→BD de las
// plantillas (Create/Get/List/Delete) con una BD real (testcontainers), incluida
// la creación en modo html (que debe guardarse saneada).
func TestTemplateHandlerCRUD(t *testing.T) {
	ctx := context.Background()
	db := storetest.NewPostgres(t)
	app, err := db.Q.CreateApplication(ctx, sqlc.CreateApplicationParams{Name: "Acme", Slug: "acme"})
	if err != nil {
		t.Fatalf("app: %v", err)
	}
	hash, _ := crypto.HashPassword("x")
	admin, err := db.Q.CreateAdminUser(ctx, sqlc.CreateAdminUserParams{Email: "a@a.com", PasswordHash: hash, Role: sqlc.AdminRoleSUPERADMIN})
	if err != nil {
		t.Fatalf("admin: %v", err)
	}
	h := NewAdminTemplateHandler(template.NewService(db, slog.Default()))

	// req construye una petición con el admin en contexto + los URLParams dados.
	req := func(method, body string, params map[string]string) *http.Request {
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("appId", app.ID.String())
		for k, v := range params {
			rctx.URLParams.Add(k, v)
		}
		c := context.WithValue(ctx, chi.RouteCtxKey, rctx)
		c = auth.ContextWithAdmin(c, auth.Admin{ID: admin.ID, Role: "SUPER_ADMIN"})
		return httptest.NewRequest(method, "/", strings.NewReader(body)).WithContext(c)
	}

	// --- Create (modo html, con <script> que debe sanearse) ---
	recC := httptest.NewRecorder()
	h.Create(recC, req("POST", `{"key":"welcome","name":"Bienvenida","channel":"EMAIL","kind":"html","subject":"S","body":"<p>{{n}}</p><script>alert(1)</script>"}`, nil))
	if recC.Code != http.StatusCreated {
		t.Fatalf("create: got %d: %s", recC.Code, recC.Body)
	}
	var created struct {
		ID   string `json:"id"`
		Kind string `json:"kind"`
		Body string `json:"body"`
	}
	if err := json.Unmarshal(recC.Body.Bytes(), &created); err != nil {
		t.Fatalf("create resp: %v", err)
	}
	if created.Kind != "html" {
		t.Fatalf("kind = %q", created.Kind)
	}
	if strings.Contains(created.Body, "<script") {
		t.Fatalf("el body debería guardarse saneado: %q", created.Body)
	}
	if !strings.Contains(created.Body, "{{n}}") {
		t.Fatalf("el body debería conservar la variable: %q", created.Body)
	}

	// --- Get ---
	recG := httptest.NewRecorder()
	h.Get(recG, req("GET", "", map[string]string{"id": created.ID}))
	if recG.Code != http.StatusOK {
		t.Fatalf("get: got %d", recG.Code)
	}

	// --- List (debe incluir la plantilla creada) ---
	recL := httptest.NewRecorder()
	h.List(recL, req("GET", "", nil))
	if recL.Code != http.StatusOK {
		t.Fatalf("list: got %d", recL.Code)
	}
	var list struct {
		Data []struct {
			Key  string `json:"key"`
			Kind string `json:"kind"`
		} `json:"data"`
	}
	_ = json.Unmarshal(recL.Body.Bytes(), &list)
	if len(list.Data) != 1 || list.Data[0].Key != "welcome" || list.Data[0].Kind != "html" {
		t.Fatalf("list inesperado: %+v", list.Data)
	}

	// --- Delete ---
	recD := httptest.NewRecorder()
	h.Delete(recD, req("DELETE", "", map[string]string{"id": created.ID}))
	if recD.Code != http.StatusNoContent {
		t.Fatalf("delete: got %d", recD.Code)
	}
	recL2 := httptest.NewRecorder()
	h.List(recL2, req("GET", "", nil))
	var list2 struct {
		Data []json.RawMessage `json:"data"`
	}
	_ = json.Unmarshal(recL2.Body.Bytes(), &list2)
	if len(list2.Data) != 0 {
		t.Fatalf("tras borrar debería quedar vacío, got %d", len(list2.Data))
	}
}
