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
}

// NewAdminAuthenticator crea el autenticador de administración.
func NewAdminAuthenticator(q AdminQuerier, issuer *TokenIssuer) *AdminAuthenticator {
	return &AdminAuthenticator{q: q, issuer: issuer}
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
		token, ok := bearerToken(r)
		if !ok {
			httpx.WriteProblem(w, r, apperr.Unauthorized("unauthenticated", "Missing bearer token"))
			return
		}
		admin, err := a.issuer.ParseAccess(token)
		if err != nil {
			httpx.WriteProblem(w, r, apperr.Unauthorized("invalid_token", "Invalid or expired token"))
			return
		}
		next.ServeHTTP(w, r.WithContext(ContextWithAdmin(r.Context(), admin)))
	})
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
