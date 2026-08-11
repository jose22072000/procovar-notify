package auth

import (
	"context"

	"github.com/google/uuid"
)

type contextKey int

const (
	callerKey contextKey = iota
	adminKey
)

// Caller es la identidad de un backend (tenant) autenticado por HMAC. Se inyecta
// en el contexto de la petición tras verificar la firma, y los handlers leen de
// aquí el application_id con el que filtran todas las queries.
type Caller struct {
	ApplicationID uuid.UUID
	APIKeyID      uuid.UUID
	KeyID         string
	Scopes        []string
}

// HasScope indica si la API key posee el scope dado.
func (c Caller) HasScope(scope string) bool {
	for _, s := range c.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}

// ContextWithCaller guarda la identidad del tenant en el contexto.
func ContextWithCaller(ctx context.Context, c Caller) context.Context {
	return context.WithValue(ctx, callerKey, c)
}

// CallerFromContext recupera la identidad del tenant (ok=false si no hay).
func CallerFromContext(ctx context.Context) (Caller, bool) {
	c, ok := ctx.Value(callerKey).(Caller)
	return c, ok
}

// Admin es la identidad de un usuario del panel autenticado por JWT.
type Admin struct {
	ID            uuid.UUID
	Role          string     // SUPER_ADMIN | APP_ADMIN
	ApplicationID *uuid.UUID // nil para SUPER_ADMIN
}

// IsSuperAdmin indica si el admin tiene rol de plataforma.
func (a Admin) IsSuperAdmin() bool { return a.Role == "SUPER_ADMIN" }

// CanAccessApplication comprueba si el admin puede operar sobre la app dada:
// los SUPER_ADMIN sobre cualquiera; los APP_ADMIN solo sobre la suya.
func (a Admin) CanAccessApplication(appID uuid.UUID) bool {
	if a.IsSuperAdmin() {
		return true
	}
	return a.ApplicationID != nil && *a.ApplicationID == appID
}

// ContextWithAdmin guarda la identidad del admin en el contexto.
func ContextWithAdmin(ctx context.Context, a Admin) context.Context {
	return context.WithValue(ctx, adminKey, a)
}

// AdminFromContext recupera la identidad del admin (ok=false si no hay).
func AdminFromContext(ctx context.Context) (Admin, bool) {
	a, ok := ctx.Value(adminKey).(Admin)
	return a, ok
}
