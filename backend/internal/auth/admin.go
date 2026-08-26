package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"dvtech/qbn/internal/apperr"
	"dvtech/qbn/internal/crypto"
	"dvtech/qbn/internal/httpx"
	"dvtech/qbn/internal/store/sqlc"
)

// AdminQuerier es la porción del store que necesita la autenticación de admins.
type AdminQuerier interface {
	GetAdminUserByEmail(ctx context.Context, email string) (sqlc.AdminUser, error)
	GetAdminUser(ctx context.Context, id uuid.UUID) (sqlc.AdminUser, error)
	IncrementAdminTokenVersion(ctx context.Context, id uuid.UUID) (int32, error)
}

// AdminAuthenticator gestiona login, refresh y validación de sesión de admins.
type AdminAuthenticator struct {
	q      AdminQuerier
	issuer *TokenIssuer
	// hub es procovar-auth. nil = SSO apagado y todo funciona como antes.
	hub *ProcovarAuth
}

// NewAdminAuthenticator crea el autenticador de administración.
func NewAdminAuthenticator(q AdminQuerier, issuer *TokenIssuer) *AdminAuthenticator {
	return &AdminAuthenticator{q: q, issuer: issuer}
}

// ConHub engancha procovar-auth. Con hub nil el comportamiento no cambia.
func (a *AdminAuthenticator) ConHub(h *ProcovarAuth) *AdminAuthenticator {
	a.hub = h
	return a
}

// Mismo mensaje para email inexistente o password incorrecto (no revelar cuál).
var errInvalidCredentials = apperr.Unauthorized("invalid_credentials", "Invalid email or password")

// dummyPasswordHash iguala el coste del login cuando el email no existe, para no
// filtrar por timing qué cuentas existen (L10). Se calcula una sola vez.
var dummyPasswordHash = func() string {
	h, _ := crypto.HashPassword("constant-time-login-dummy-password")
	return h
}()

// Login valida email/password y devuelve un par de tokens + la identidad.
func (a *AdminAuthenticator) Login(ctx context.Context, email, password string) (access, refresh string, admin Admin, err error) {
	u, err := a.q.GetAdminUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Verifica contra un hash dummy para igualar el tiempo y no revelar
			// por timing si el email existe (anti-enumeración).
			_, _ = crypto.VerifyPassword(password, dummyPasswordHash)
			return "", "", Admin{}, errInvalidCredentials
		}
		return "", "", Admin{}, apperr.Internal(err)
	}
	if u.Status != sqlc.StatusActiveDisabledACTIVE {
		return "", "", Admin{}, apperr.Forbidden("account_disabled", "Account is disabled")
	}

	ok, err := crypto.VerifyPassword(password, u.PasswordHash)
	if err != nil || !ok {
		return "", "", Admin{}, errInvalidCredentials
	}

	admin = toAdmin(u)
	access, refresh, err = a.issuer.Issue(admin, int(u.TokenVersion))
	if err != nil {
		return "", "", Admin{}, apperr.Internal(err)
	}
	return access, refresh, admin, nil
}

// Refresh valida un refresh token, recomprueba que la cuenta siga activa y que
// la token_version del token siga vigente (revocación), y emite un par nuevo
// (con rol/app actualizados desde la BD).
func (a *AdminAuthenticator) Refresh(ctx context.Context, refreshToken string) (access, refresh string, err error) {
	claimed, ver, err := a.issuer.ParseRefresh(refreshToken)
	if err != nil {
		return "", "", apperr.Unauthorized("invalid_token", "Invalid refresh token")
	}
	u, err := a.q.GetAdminUser(ctx, claimed.ID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", apperr.Unauthorized("invalid_token", "Invalid refresh token")
		}
		return "", "", apperr.Internal(err)
	}
	if u.Status != sqlc.StatusActiveDisabledACTIVE {
		return "", "", apperr.Forbidden("account_disabled", "Account is disabled")
	}
	// Revocación: si la token_version del refresh no coincide con la de la BD, el
	// token fue invalidado (logout / cambio de contraseña) → se rechaza.
	if int(u.TokenVersion) != ver {
		return "", "", apperr.Unauthorized("invalid_token", "Invalid refresh token")
	}
	access, refresh, err = a.issuer.Issue(toAdmin(u), int(u.TokenVersion))
	if err != nil {
		return "", "", apperr.Internal(err)
	}
	return access, refresh, nil
}

// Logout invalida todos los refresh tokens del admin dueño del refresh token
// dado incrementando su token_version. Es idempotente: un token ausente o
// inválido no es error (el objetivo —no tener sesión— ya se cumple).
func (a *AdminAuthenticator) Logout(ctx context.Context, refreshToken string) error {
	claimed, _, err := a.issuer.ParseRefresh(refreshToken)
	if err != nil {
		return nil
	}
	if _, err := a.q.IncrementAdminTokenVersion(ctx, claimed.ID); err != nil {
		return apperr.Internal(err)
	}
	return nil
}

// Middleware valida el access token del header Authorization: Bearer e inyecta
// la identidad del admin en el contexto.
func (a *AdminAuthenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1) El camino de siempre: token propio de Avisos.
		if token, ok := bearerToken(r); ok {
			admin, err := a.issuer.ParseAccess(token)
			if err == nil {
				next.ServeHTTP(w, r.WithContext(ContextWithAdmin(r.Context(), admin)))
				return
			}
			// Un bearer presente pero malo es un error, no una invitación a
			// probar el SSO: quien manda un token caducado espera que se lo
			// digan, no acabar entrando como otra identidad.
			httpx.WriteProblem(w, r, apperr.Unauthorized("invalid_token", "Invalid or expired token"))
			return
		}

		// 2) Sin bearer: la sesión única de Procovar (procovar-auth).
		if admin, ok := a.desdeProcovar(r); ok {
			next.ServeHTTP(w, r.WithContext(ContextWithAdmin(r.Context(), admin)))
			return
		}

		httpx.WriteProblem(w, r, apperr.Unauthorized("unauthenticated", "Missing bearer token"))
	})
}

// desdeProcovar intenta identificar a quien entra por la cookie de sesión de
// procovar-auth.
//
// Devuelve (Admin, false) y NO escribe nada cuando no aplica —hub sin
// configurar, sin cookie, sesión mala o sin el permiso de Avisos—, para que el
// llamante responda un 401 igual que antes. Distinguir aquí entre "no tiene
// cookie" y "tiene cookie pero no le toca" solo serviría para que alguien de
// fuera averigüe quién tiene acceso.
func (a *AdminAuthenticator) desdeProcovar(r *http.Request) (Admin, bool) {
	if a.hub == nil {
		return Admin{}, false
	}
	c, err := r.Cookie(a.hub.CookieName())
	if err != nil || c.Value == "" {
		return Admin{}, false
	}
	ses, err := a.hub.VerifySession(r.Context(), c.Value)
	if err != nil {
		return Admin{}, false
	}
	// La llave de entrada. Sin ella no se pasa, aunque la sesión sea buena:
	// estar dado de alta en Procovar no es estar dado de alta en Avisos.
	if !ses.Puede(PermisoAvisosEntrar) {
		return Admin{}, false
	}
	// Quien administra Avisos lo administra entero: los tipos, las plantillas y
	// los canales son de la plataforma, no de una sucursal.
	rol := "APP_ADMIN"
	if ses.TodoVale || ses.Puede(PermisoAvisosManage) {
		rol = "SUPER_ADMIN"
	}
	return Admin{ID: idDeProcovar(ses.UserID), Role: rol}, true
}

// Claves del catálogo de procovar-auth que gobiernan Avisos.
const (
	PermisoAvisosEntrar = "avisos.entrar"
	PermisoAvisosManage = "avisos.manage"
)

// idDeProcovar convierte el id de usuario del hub —que es una cadena, no un
// UUID— en un UUID estable, porque `Admin.ID` es un uuid.UUID y se usa como
// autor en la auditoría.
//
// Determinista a propósito: la misma persona tiene que dejar siempre la misma
// huella en la auditoría, entre reinicios y entre instancias. Un UUID aleatorio
// por sesión convertiría el registro de auditoría en ruido.
func idDeProcovar(userID string) uuid.UUID {
	if u, err := uuid.Parse(userID); err == nil {
		return u
	}
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("procovar-auth:"+userID))
}

// RequireSuperAdmin exige rol SUPER_ADMIN. Debe montarse tras Middleware.
func RequireSuperAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		admin, ok := AdminFromContext(r.Context())
		if !ok {
			httpx.WriteProblem(w, r, apperr.Unauthorized("unauthenticated", "Authentication required"))
			return
		}
		if !admin.IsSuperAdmin() {
			httpx.WriteProblem(w, r, apperr.Forbidden("forbidden", "Requires super-admin role"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// toAdmin mapea el registro de BD a la identidad de dominio.
func toAdmin(u sqlc.AdminUser) Admin {
	admin := Admin{ID: u.ID, Role: string(u.Role)}
	if u.ApplicationID.Valid {
		id := uuid.UUID(u.ApplicationID.Bytes)
		admin.ApplicationID = &id
	}
	return admin
}

// bearerToken extrae el token de "Authorization: Bearer <token>".
func bearerToken(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(h) <= len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return "", false
	}
	return strings.TrimSpace(h[len(prefix):]), true
}
