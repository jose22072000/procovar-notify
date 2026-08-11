package http

import (
	"net/http"

	"dvtech/qbn/internal/apperr"
	"dvtech/qbn/internal/auth"
	"dvtech/qbn/internal/httpx"
)

// AdminAuthHandler expone los endpoints de autenticación del panel.
type AdminAuthHandler struct {
	auth   *auth.AdminAuthenticator
	cookie CookieConfig
}

// NewAdminAuthHandler crea el handler de auth de administración. cookie define
// cómo se emite la cookie HttpOnly del refresh token.
func NewAdminAuthHandler(a *auth.AdminAuthenticator, cookie CookieConfig) *AdminAuthHandler {
	return &AdminAuthHandler{auth: a, cookie: cookie}
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// tokenResponse: el refresh token viaja en la cookie HttpOnly, NO en el cuerpo,
// para que un XSS no pueda leerlo desde JS. El cuerpo solo lleva el access
// (corto, en localStorage del cliente) y la identidad.
type tokenResponse struct {
	AccessToken string     `json:"accessToken"`
	Admin       *adminView `json:"admin,omitempty"`
}

type adminView struct {
	ID            string  `json:"id"`
	Role          string  `json:"role"`
	ApplicationID *string `json:"applicationId,omitempty"`
}

// Login: POST /admin/auth/login
func (h *AdminAuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}

	access, refresh, admin, err := h.auth.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}

	setRefreshCookie(w, h.cookie, refresh)
	httpx.WriteJSON(w, http.StatusOK, tokenResponse{
		AccessToken: access,
		Admin:       toAdminView(admin),
	})
}

// Refresh: POST /admin/auth/refresh — el refresh token llega en la cookie
// HttpOnly (no en el cuerpo). Rota la cookie y devuelve un access nuevo.
func (h *AdminAuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	refreshToken := readRefreshCookie(r, h.cookie)

	access, refresh, err := h.auth.Refresh(r.Context(), refreshToken)
	if err != nil {
		// Refresh inválido/revocado: limpia la cookie muerta para no reintentar.
		clearRefreshCookie(w, h.cookie)
		httpx.WriteProblem(w, r, err)
		return
	}

	setRefreshCookie(w, h.cookie, refresh)
	httpx.WriteJSON(w, http.StatusOK, tokenResponse{AccessToken: access})
}

// Logout: POST /admin/auth/logout — invalida todos los refresh del admin
// (incrementa token_version) y borra la cookie. Público e idempotente: no exige
// access token válido, así que también funciona con la sesión ya caducada.
func (h *AdminAuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	refreshToken := readRefreshCookie(r, h.cookie)
	if err := h.auth.Logout(r.Context(), refreshToken); err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	clearRefreshCookie(w, h.cookie)
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
}

// Me: GET /admin/me — identidad del admin autenticado.
func (h *AdminAuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	admin, ok := auth.AdminFromContext(r.Context())
	if !ok {
		httpx.WriteProblem(w, r, apperr.Unauthorized("unauthenticated", "Authentication required"))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toAdminView(admin))
}

func toAdminView(a auth.Admin) *adminView {
	v := &adminView{ID: a.ID.String(), Role: a.Role}
	if a.ApplicationID != nil {
		s := a.ApplicationID.String()
		v.ApplicationID = &s
	}
	return v
}

// WhoAmI: GET /v1/whoami — identidad del tenant autenticado por HMAC.
func WhoAmI(w http.ResponseWriter, r *http.Request) {
	caller, ok := auth.CallerFromContext(r.Context())
	if !ok {
		httpx.WriteProblem(w, r, apperr.Unauthorized("unauthenticated", "Authentication required"))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"applicationId": caller.ApplicationID.String(),
		"keyId":         caller.KeyID,
		"scopes":        caller.Scopes,
	})
}
