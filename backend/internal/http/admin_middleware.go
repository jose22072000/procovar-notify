package http

import (
	"net"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"dvtech/qbn/internal/apperr"
	"dvtech/qbn/internal/audit"
	"dvtech/qbn/internal/auth"
	"dvtech/qbn/internal/httpx"
)

// trustProxy indica si confiar en X-Forwarded-For para derivar la IP del cliente.
// Por defecto false: sin un proxy de confianza, XFF es spoofeable y no debe usarse
// (L9). Se habilita en el arranque vía SetTrustProxy cuando el servicio corre tras
// un proxy/balanceador de confianza.
var trustProxy atomic.Bool

// SetTrustProxy habilita/inhabilita la confianza en X-Forwarded-For.
func SetTrustProxy(v bool) { trustProxy.Store(v) }

// auditMeta inyecta IP y user-agent de la petición en el contexto, para que las
// entradas de AdminAuditLog los registren (§11).
func auditMeta(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := audit.ContextWithRequestMeta(r.Context(), clientIP(r), r.UserAgent())
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// clientIP deriva la IP del cliente. Solo usa X-Forwarded-For si hay un proxy de
// confianza configurado (SetTrustProxy); en otro caso la cabecera es spoofeable y
// se ignora, usando RemoteAddr (L9).
func clientIP(r *http.Request) string {
	if trustProxy.Load() {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			return strings.TrimSpace(strings.Split(xff, ",")[0])
		}
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// requireAppAccess es un middleware (tras AdminAuth.Middleware) que exige que el
// admin tenga acceso a la aplicación del path param {appId}: SUPER_ADMIN a
// cualquiera, APP_ADMIN solo a la suya.
func requireAppAccess(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		admin, ok := auth.AdminFromContext(r.Context())
		if !ok {
			httpx.WriteProblem(w, r, apperr.Unauthorized("unauthenticated", "Authentication required"))
			return
		}
		appID, err := uuid.Parse(chi.URLParam(r, "appId"))
		if err != nil {
			httpx.WriteProblem(w, r, apperr.Validation("invalid_app_id", "Invalid application id"))
			return
		}
		if !admin.CanAccessApplication(appID) {
			httpx.WriteProblem(w, r, apperr.Forbidden("forbidden", "No access to this application"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// appIDParam extrae y valida el {appId} del path.
func appIDParam(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	appID, err := uuid.Parse(chi.URLParam(r, "appId"))
	if err != nil {
		httpx.WriteProblem(w, r, apperr.Validation("invalid_app_id", "Invalid application id"))
		return uuid.Nil, false
	}
	return appID, true
}
