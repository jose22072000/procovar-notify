// Package audit registra acciones sensibles del panel de administración en la
// tabla admin_audit_logs. Toda mutación relevante del panel (crear/revocar API
// keys, editar SMTP/proveedores, publicar templates...) debe pasar por Record,
// idealmente dentro de la misma transacción que la mutación.
//
// Definido en la Fase 1; se cablea en la Fase 6.
package audit

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"dvtech/qbn/internal/store/sqlc"
)

// Action es el identificador estable de una acción auditable. La taxonomía usa
// el patrón "recurso.verbo".
type Action string

const (
	ActionApplicationCreate  Action = "application.create"
	ActionApplicationUpdate  Action = "application.update"
	ActionApplicationSuspend Action = "application.suspend"

	ActionAPIKeyCreate Action = "api_key.create"
	ActionAPIKeyRevoke Action = "api_key.revoke"
	ActionAPIKeyDelete Action = "api_key.delete"

	ActionSMTPCreate Action = "smtp.create"
	ActionSMTPUpdate Action = "smtp.update"
	ActionSMTPDelete Action = "smtp.delete"

	ActionProviderCreate Action = "provider.create"
	ActionProviderUpdate Action = "provider.update"
	ActionProviderDelete Action = "provider.delete"

	ActionRouteCreate Action = "route.create"
	ActionRouteUpdate Action = "route.update"
	ActionRouteDelete Action = "route.delete"

	ActionTemplateCreate  Action = "template.create"
	ActionTemplateUpdate  Action = "template.update"
	ActionTemplatePublish Action = "template.publish"
	ActionTemplateDelete  Action = "template.delete"

	ActionAdminUserCreate Action = "admin_user.create"
	ActionAdminUserUpdate Action = "admin_user.update"

	ActionRecurringCreate Action = "recurring.create"
	ActionRecurringDelete Action = "recurring.delete"

	ActionRetentionUpdate Action = "retention.update"
	ActionUserForget      Action = "user.forget"

	ActionWebhookCreate Action = "webhook.create"
	ActionWebhookDelete Action = "webhook.delete"

	ActionSuppressionAdd    Action = "suppression.add"
	ActionSuppressionRemove Action = "suppression.remove"
)

// Entry describe una acción a auditar.
type Entry struct {
	ActorAdminID  uuid.UUID  // quién realiza la acción
	ApplicationID *uuid.UUID // recurso afectado; nil = acción global
	Action        Action
	TargetType    string // tipo del recurso afectado ("api_key", "smtp_connection"...)
	TargetID      string // id del recurso (opcional)
	Details       json.RawMessage
	IP            string
	UserAgent     string
}

// requestMeta lleva el IP y user-agent de la petición admin por el contexto,
// para enriquecer las entradas de auditoría (§11).
type requestMeta struct{ ip, userAgent string }

type metaKeyType struct{}

var metaKey metaKeyType

// ContextWithRequestMeta adjunta IP y user-agent al contexto (lo setea el
// middleware admin).
func ContextWithRequestMeta(ctx context.Context, ip, userAgent string) context.Context {
	return context.WithValue(ctx, metaKey, requestMeta{ip: ip, userAgent: userAgent})
}

// RequestMetaFromContext recupera IP y user-agent (cadenas vacías si no hay).
func RequestMetaFromContext(ctx context.Context) (ip, userAgent string) {
	if m, ok := ctx.Value(metaKey).(requestMeta); ok {
		return m.ip, m.userAgent
	}
	return "", ""
}

// Record persiste una entrada de auditoría usando el *sqlc.Queries dado (que
// puede estar ligado a una transacción para atomicidad con la mutación). Si la
// Entry no trae IP/UserAgent, se completan desde el contexto.
func Record(ctx context.Context, q *sqlc.Queries, e Entry) error {
	if e.IP == "" && e.UserAgent == "" {
		e.IP, e.UserAgent = RequestMetaFromContext(ctx)
	}
	_, err := q.CreateAuditLog(ctx, sqlc.CreateAuditLogParams{
		ActorAdminID:  e.ActorAdminID,
		ApplicationID: toPgUUID(e.ApplicationID),
		Action:        string(e.Action),
		TargetType:    e.TargetType,
		TargetID:      strPtr(e.TargetID),
		Details:       e.Details,
		Ip:            strPtr(e.IP),
		UserAgent:     strPtr(e.UserAgent),
	})
	return err
}

// toPgUUID convierte un *uuid.UUID en el pgtype.UUID nullable que espera sqlc.
func toPgUUID(u *uuid.UUID) pgtype.UUID {
	if u == nil {
		return pgtype.UUID{Valid: false}
	}
	return pgtype.UUID{Bytes: *u, Valid: true}
}

// strPtr devuelve nil para la cadena vacía (NULL en BD) o un puntero al valor.
func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
