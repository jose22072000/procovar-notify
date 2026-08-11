package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"dvtech/qbn/internal/auth"
	"dvtech/qbn/internal/crypto"
	"dvtech/qbn/internal/store/sqlc"
	"dvtech/qbn/internal/storetest"
)

// TestAdminAuthCookieFlow cubre el Bloque B end-to-end en la capa HTTP: el
// refresh token viaja en una cookie HttpOnly (no en el cuerpo), el refresh la
// rota, y el logout revoca (token_version++) de modo que la cookie previa deja
// de valer.
func TestAdminAuthCookieFlow(t *testing.T) {
	db := storetest.NewPostgres(t)
	ctx := context.Background()

	hash, err := crypto.HashPassword("s3creta")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if _, err := db.Q.CreateAdminUser(ctx, sqlc.CreateAdminUserParams{
		Email: "admin@x.com", PasswordHash: hash, Role: sqlc.AdminRoleSUPERADMIN,
	}); err != nil {
		t.Fatalf("crear admin: %v", err)
	}

	h := NewAdminAuthHandler(
		auth.NewAdminAuthenticator(db.Q, auth.NewTokenIssuer("clave-jwt-de-prueba-suficientemente-larga")),
		CookieConfig{Name: "qbn_refresh", SameSite: http.SameSiteLaxMode},
	)

	// --- Login: 200, cookie HttpOnly con el refresh, cuerpo con access y SIN refresh ---
	loginRec := httptest.NewRecorder()
	h.Login(loginRec, httptest.NewRequest("POST", "/admin/auth/login",
		strings.NewReader(`{"email":"admin@x.com","password":"s3creta"}`)))
	if loginRec.Code != http.StatusOK {
		t.Fatalf("login: esperaba 200, got %d: %s", loginRec.Code, loginRec.Body)
	}
	var body map[string]any
	if err := json.Unmarshal(loginRec.Body.Bytes(), &body); err != nil {
		t.Fatalf("login body: %v", err)
	}
	if s, _ := body["accessToken"].(string); s == "" {
		t.Fatal("login no devolvió accessToken")
	}
	if _, present := body["refreshToken"]; present {
		t.Fatal("el refresh token NO debe ir en el cuerpo (va en la cookie)")
	}
	cookie := cookieNamed(loginRec.Result().Cookies(), "qbn_refresh")
	if cookie == nil || cookie.Value == "" {
		t.Fatal("login no fijó la cookie del refresh")
	}
	if !cookie.HttpOnly {
		t.Fatal("la cookie del refresh debe ser HttpOnly (inaccesible a JS)")
	}

	// --- Refresh con la cookie: 200, nuevo access, cookie rotada ---
	refreshReq := httptest.NewRequest("POST", "/admin/auth/refresh", nil)
	refreshReq.AddCookie(cookie)
	refreshRec := httptest.NewRecorder()
	h.Refresh(refreshRec, refreshReq)
	if refreshRec.Code != http.StatusOK {
		t.Fatalf("refresh: esperaba 200, got %d: %s", refreshRec.Code, refreshRec.Body)
	}
	rotated := cookieNamed(refreshRec.Result().Cookies(), "qbn_refresh")
	if rotated == nil || rotated.Value == "" {
		t.Fatal("refresh no rotó la cookie")
	}

	// --- Refresh sin cookie: 401 ---
	noCookieRec := httptest.NewRecorder()
	h.Refresh(noCookieRec, httptest.NewRequest("POST", "/admin/auth/refresh", nil))
	if noCookieRec.Code != http.StatusUnauthorized {
		t.Fatalf("refresh sin cookie: esperaba 401, got %d", noCookieRec.Code)
	}

	// --- Logout con la cookie: 200 y cookie de borrado (valor vacío) ---
	logoutReq := httptest.NewRequest("POST", "/admin/auth/logout", nil)
	logoutReq.AddCookie(rotated)
	logoutRec := httptest.NewRecorder()
	h.Logout(logoutRec, logoutReq)
	if logoutRec.Code != http.StatusOK {
		t.Fatalf("logout: esperaba 200, got %d: %s", logoutRec.Code, logoutRec.Body)
	}
	if cleared := cookieNamed(logoutRec.Result().Cookies(), "qbn_refresh"); cleared == nil || cleared.Value != "" {
		t.Fatal("logout debe emitir una cookie de borrado (valor vacío)")
	}

	// --- Revocación: la cookie previa (versión anterior) ya no vale tras el logout ---
	revReq := httptest.NewRequest("POST", "/admin/auth/refresh", nil)
	revReq.AddCookie(rotated)
	revRec := httptest.NewRecorder()
	h.Refresh(revRec, revReq)
	if revRec.Code != http.StatusUnauthorized {
		t.Fatalf("refresh tras logout: esperaba 401 (token revocado), got %d", revRec.Code)
	}
}

func cookieNamed(cs []*http.Cookie, name string) *http.Cookie {
	for _, c := range cs {
		if c.Name == name {
			return c
		}
	}
	return nil
}
