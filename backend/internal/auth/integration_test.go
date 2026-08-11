package auth_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"dvtech/qbn/internal/auth"
	"dvtech/qbn/internal/crypto"
	"dvtech/qbn/internal/store"
	"dvtech/qbn/internal/store/sqlc"
	"dvtech/qbn/internal/storetest"
)

// --- HMAC ---

func TestHMACMiddleware(t *testing.T) {
	db := storetest.NewPostgres(t)
	ctx := context.Background()
	enc, _ := crypto.NewEncryptor(make([]byte, 32))

	app, err := db.Q.CreateApplication(ctx, sqlc.CreateApplicationParams{Name: "Acme", Slug: "acme"})
	mustNoErr(t, err)

	secret := []byte("secreto-hmac-de-prueba")
	secretEnc, err := enc.Encrypt(secret)
	mustNoErr(t, err)
	_, err = db.Q.CreateAPIKey(ctx, sqlc.CreateAPIKeyParams{
		ApplicationID: app.ID,
		KeyID:         "kid_test",
		SecretEnc:     secretEnc,
		Scopes:        []string{"notifications:send"},
	})
	mustNoErr(t, err)

	srv := hmacTestServer(db)

	t.Run("firma válida pasa y deriva el tenant", func(t *testing.T) {
		rec := do(srv, signed(t, "GET", "/whoami", nil, "kid_test", secret, time.Now().Unix()))
		if rec.Code != http.StatusOK {
			t.Fatalf("esperaba 200, got %d: %s", rec.Code, rec.Body)
		}
		if got := rec.Body.String(); got != app.ID.String() {
			t.Fatalf("application_id esperado %s, got %s", app.ID, got)
		}
	})

	t.Run("sin cabeceras → 401", func(t *testing.T) {
		rec := do(srv, httptest.NewRequest("GET", "/whoami", nil))
		assertCode(t, rec, http.StatusUnauthorized)
	})

	t.Run("firma inválida → 401", func(t *testing.T) {
		req := signed(t, "GET", "/whoami", nil, "kid_test", []byte("secreto-equivocado"), time.Now().Unix())
		assertCode(t, do(srv, req), http.StatusUnauthorized)
	})

	t.Run("timestamp fuera de ventana (replay) → 401", func(t *testing.T) {
		old := time.Now().Add(-10 * time.Minute).Unix()
		assertCode(t, do(srv, signed(t, "GET", "/whoami", nil, "kid_test", secret, old)), http.StatusUnauthorized)
	})

	t.Run("key inexistente → 401", func(t *testing.T) {
		assertCode(t, do(srv, signed(t, "GET", "/whoami", nil, "kid_desconocida", secret, time.Now().Unix())), http.StatusUnauthorized)
	})

	t.Run("scope insuficiente → 403", func(t *testing.T) {
		// La key solo tiene notifications:send; /admin-scoped exige otro scope.
		assertCode(t, do(srv, signed(t, "GET", "/needs-read", nil, "kid_test", secret, time.Now().Unix())), http.StatusForbidden)
	})

	t.Run("scope correcto → 200", func(t *testing.T) {
		assertCode(t, do(srv, signed(t, "GET", "/needs-send", nil, "kid_test", secret, time.Now().Unix())), http.StatusOK)
	})
}

func hmacTestServer(db *store.DB) http.Handler {
	enc, _ := crypto.NewEncryptor(make([]byte, 32))
	hmacAuth := auth.NewHMACAuthenticator(db.Q, enc, 300)

	r := chi.NewRouter()
	r.Use(hmacAuth.Middleware)
	r.Get("/whoami", func(w http.ResponseWriter, r *http.Request) {
		c, _ := auth.CallerFromContext(r.Context())
		_, _ = w.Write([]byte(c.ApplicationID.String()))
	})
	r.With(auth.RequireScope("notifications:read")).Get("/needs-read", okHandler)
	r.With(auth.RequireScope("notifications:send")).Get("/needs-send", okHandler)
	return r
}

// TestHMACKeyRevokedAndExpired (M3): una API key revocada o caducada se rechaza
// con 403 aunque la firma sea válida.
func TestHMACKeyRevokedAndExpired(t *testing.T) {
	db := storetest.NewPostgres(t)
	ctx := context.Background()
	enc, _ := crypto.NewEncryptor(make([]byte, 32))
	app, err := db.Q.CreateApplication(ctx, sqlc.CreateApplicationParams{Name: "Acme", Slug: "acme"})
	mustNoErr(t, err)
	secret := []byte("secreto-hmac")
	secretEnc, _ := enc.Encrypt(secret)

	mkKey := func(keyID string) {
		_, err := db.Q.CreateAPIKey(ctx, sqlc.CreateAPIKeyParams{
			ApplicationID: app.ID, KeyID: keyID, SecretEnc: secretEnc, Scopes: []string{"notifications:send"},
		})
		mustNoErr(t, err)
	}
	srv := hmacTestServer(db)

	t.Run("revocada → 403", func(t *testing.T) {
		mkKey("kid_revoked")
		_, err := db.Pool.Exec(ctx, `UPDATE api_keys SET status = 'REVOKED' WHERE key_id = 'kid_revoked'`)
		mustNoErr(t, err)
		assertCode(t, do(srv, signed(t, "GET", "/whoami", nil, "kid_revoked", secret, time.Now().Unix())), http.StatusForbidden)
	})

	t.Run("caducada → 403", func(t *testing.T) {
		mkKey("kid_expired")
		_, err := db.Pool.Exec(ctx, `UPDATE api_keys SET expires_at = now() - interval '1 hour' WHERE key_id = 'kid_expired'`)
		mustNoErr(t, err)
		assertCode(t, do(srv, signed(t, "GET", "/whoami", nil, "kid_expired", secret, time.Now().Unix())), http.StatusForbidden)
	})
}

// TestHMACSignsQuery verifica M16: la query queda autenticada. Una petición con
// query firmada pasa; alterar la query tras firmar invalida la firma.
func TestHMACSignsQuery(t *testing.T) {
	db := storetest.NewPostgres(t)
	ctx := context.Background()
	enc, _ := crypto.NewEncryptor(make([]byte, 32))
	app, err := db.Q.CreateApplication(ctx, sqlc.CreateApplicationParams{Name: "Acme", Slug: "acme"})
	mustNoErr(t, err)
	secret := []byte("secreto-hmac-de-prueba")
	secretEnc, _ := enc.Encrypt(secret)
	_, err = db.Q.CreateAPIKey(ctx, sqlc.CreateAPIKeyParams{
		ApplicationID: app.ID, KeyID: "kid_q", SecretEnc: secretEnc, Scopes: []string{"notifications:send"},
	})
	mustNoErr(t, err)
	srv := hmacTestServer(db)

	t.Run("query firmada pasa", func(t *testing.T) {
		assertCode(t, do(srv, signed(t, "GET", "/whoami?userId=abc&page=2", nil, "kid_q", secret, time.Now().Unix())), http.StatusOK)
	})

	t.Run("query alterada tras firmar → 401", func(t *testing.T) {
		req := signed(t, "GET", "/whoami?userId=abc", nil, "kid_q", secret, time.Now().Unix())
		req.URL.RawQuery = "userId=otro" // manipulación: la firma cubría userId=abc
		assertCode(t, do(srv, req), http.StatusUnauthorized)
	})
}

// TestHMACReplayGuard verifica M16: con caché de nonce, reenviar la misma firma
// (dentro de la ventana) se rechaza; firmas distintas no se ven afectadas.
func TestHMACReplayGuard(t *testing.T) {
	db := storetest.NewPostgres(t)
	ctx := context.Background()
	enc, _ := crypto.NewEncryptor(make([]byte, 32))
	app, err := db.Q.CreateApplication(ctx, sqlc.CreateApplicationParams{Name: "Acme", Slug: "acme"})
	mustNoErr(t, err)
	secret := []byte("secreto-hmac-de-prueba")
	secretEnc, _ := enc.Encrypt(secret)
	_, err = db.Q.CreateAPIKey(ctx, sqlc.CreateAPIKeyParams{
		ApplicationID: app.ID, KeyID: "kid_r", SecretEnc: secretEnc, Scopes: []string{"notifications:send"},
	})
	mustNoErr(t, err)

	mr, err := miniredis.Run()
	mustNoErr(t, err)
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	guard := auth.NewRedisReplayGuard(rdb, "test")

	hmacAuth := auth.NewHMACAuthenticator(db.Q, enc, 300).WithReplayGuard(guard)
	r := chi.NewRouter()
	r.Use(hmacAuth.Middleware)
	r.Get("/whoami", okHandler)

	ts := time.Now().Unix()
	first := signed(t, "GET", "/whoami", nil, "kid_r", secret, ts)
	assertCode(t, do(r, first), http.StatusOK)

	// Reenvío idéntico (misma firma) → rechazado por la caché de nonce.
	replay := signed(t, "GET", "/whoami", nil, "kid_r", secret, ts)
	assertCode(t, do(r, replay), http.StatusUnauthorized)

	// Una firma distinta (otro timestamp) sigue pasando.
	assertCode(t, do(r, signed(t, "GET", "/whoami", nil, "kid_r", secret, ts+1)), http.StatusOK)
}

// --- Admin (JWT + roles) ---

func TestAdminAuthFlow(t *testing.T) {
	db := storetest.NewPostgres(t)
	ctx := context.Background()

	hash, err := crypto.HashPassword("s3creta")
	mustNoErr(t, err)
	_, err = db.Q.CreateAdminUser(ctx, sqlc.CreateAdminUserParams{
		Email:        "admin@x.com",
		PasswordHash: hash,
		Role:         sqlc.AdminRoleSUPERADMIN,
	})
	mustNoErr(t, err)

	authn := auth.NewAdminAuthenticator(db.Q, auth.NewTokenIssuer("jwt-secret"))

	t.Run("login correcto emite tokens", func(t *testing.T) {
		access, refresh, admin, err := authn.Login(ctx, "admin@x.com", "s3creta")
		mustNoErr(t, err)
		if access == "" || refresh == "" {
			t.Fatal("tokens vacíos")
		}
		if !admin.IsSuperAdmin() {
			t.Fatal("debería ser super-admin")
		}
		// El access token valida en el middleware.
		mw := authn.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			a, ok := auth.AdminFromContext(r.Context())
			if !ok || a.ID != admin.ID {
				t.Error("el admin no llegó al contexto")
			}
			w.WriteHeader(http.StatusOK)
		}))
		req := httptest.NewRequest("GET", "/admin/me", nil)
		req.Header.Set("Authorization", "Bearer "+access)
		assertCode(t, do(mw, req), http.StatusOK)
	})

	t.Run("password incorrecta → error", func(t *testing.T) {
		if _, _, _, err := authn.Login(ctx, "admin@x.com", "mala"); err == nil {
			t.Fatal("debería fallar")
		}
	})

	t.Run("refresh emite nuevos tokens", func(t *testing.T) {
		_, refresh, _, err := authn.Login(ctx, "admin@x.com", "s3creta")
		mustNoErr(t, err)
		access2, _, err := authn.Refresh(ctx, refresh)
		mustNoErr(t, err)
		if access2 == "" {
			t.Fatal("refresh no devolvió access")
		}
	})

	t.Run("token basura en middleware → 401", func(t *testing.T) {
		mw := authn.Middleware(http.HandlerFunc(okHandler))
		req := httptest.NewRequest("GET", "/admin/me", nil)
		req.Header.Set("Authorization", "Bearer no-es-un-jwt")
		assertCode(t, do(mw, req), http.StatusUnauthorized)
	})
}

// TestAdminCanAccessApplication cubre el aislamiento de APP_ADMIN.
func TestAdminCanAccessApplication(t *testing.T) {
	appA := uuidMust("11111111-1111-1111-1111-111111111111")
	appB := uuidMust("22222222-2222-2222-2222-222222222222")

	super := auth.Admin{Role: "SUPER_ADMIN"}
	if !super.CanAccessApplication(appB) {
		t.Fatal("super-admin debería acceder a cualquier app")
	}

	appAdmin := auth.Admin{Role: "APP_ADMIN", ApplicationID: &appA}
	if !appAdmin.CanAccessApplication(appA) {
		t.Fatal("app-admin debería acceder a SU app")
	}
	if appAdmin.CanAccessApplication(appB) {
		t.Fatal("app-admin NO debería acceder a otra app")
	}
}

// --- Rate limiter ---

func TestRateLimiter(t *testing.T) {
	rdb := storetest.NewRedis(t)
	// burst=2, rate bajo: la 3ª petición inmediata debe ser rechazada.
	rl := auth.NewRateLimiter(rdb, "test", 1, 2)

	r := chi.NewRouter()
	r.Use(injectCaller("kid_rl"))
	r.Use(rl.Middleware)
	r.Get("/x", okHandler)

	codes := make([]int, 4)
	for i := range codes {
		codes[i] = do(r, httptest.NewRequest("GET", "/x", nil)).Code
	}
	// Las primeras 2 pasan (burst), luego al menos una 429.
	if codes[0] != 200 || codes[1] != 200 {
		t.Fatalf("las primeras 2 deberían pasar: %v", codes)
	}
	got429 := false
	for _, c := range codes[2:] {
		if c == http.StatusTooManyRequests {
			got429 = true
		}
	}
	if !got429 {
		t.Fatalf("se esperaba al menos un 429 tras agotar el burst: %v", codes)
	}
}

// TestAuthIPRateLimiter: el IPMiddleware limita por IP de origen (login admin).
func TestAuthIPRateLimiter(t *testing.T) {
	rdb := storetest.NewRedis(t)
	rl := auth.NewRateLimiter(rdb, "authtest", 1, 2) // burst=2

	r := chi.NewRouter()
	r.Use(rl.IPMiddleware)
	r.Get("/x", okHandler)

	req := func(remoteAddr string) int {
		rq := httptest.NewRequest("GET", "/x", nil)
		rq.RemoteAddr = remoteAddr
		return do(r, rq).Code
	}

	// Misma IP (distinto puerto): tras agotar el burst (2), al menos un 429.
	codes := []int{req("1.2.3.4:11"), req("1.2.3.4:22"), req("1.2.3.4:33"), req("1.2.3.4:44")}
	if codes[0] != 200 || codes[1] != 200 {
		t.Fatalf("las primeras 2 deberían pasar: %v", codes)
	}
	got429 := false
	for _, c := range codes[2:] {
		if c == http.StatusTooManyRequests {
			got429 = true
		}
	}
	if !got429 {
		t.Fatalf("se esperaba 429 tras el burst para la misma IP: %v", codes)
	}

	// Una IP distinta tiene su propio bucket (no afectada).
	if c := req("9.9.9.9:55"); c != 200 {
		t.Fatalf("una IP distinta no debería estar limitada: %d", c)
	}
}

// injectCaller simula un Caller autenticado para probar el rate limiter aislado.
func injectCaller(keyID string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := auth.ContextWithCaller(r.Context(), auth.Caller{KeyID: keyID})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// --- helpers ---

func okHandler(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }

func signed(t *testing.T, method, path string, body []byte, keyID string, secret []byte, ts int64) *http.Request {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	tss := strconv.FormatInt(ts, 10)
	req.Header.Set(auth.HeaderKeyID, keyID)
	req.Header.Set(auth.HeaderTimestamp, tss)
	req.Header.Set(auth.HeaderSignature, auth.Sign(secret, method, req.URL.Path, auth.CanonicalQuery(req.URL), body, tss))
	return req
}

func do(h http.Handler, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func assertCode(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("esperaba %d, got %d: %s", want, rec.Code, rec.Body)
	}
}

func mustNoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("error inesperado: %v", err)
	}
}

func uuidMust(s string) uuid.UUID { return uuid.MustParse(s) }
