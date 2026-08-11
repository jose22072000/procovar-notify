package admin

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"dvtech/qbn/internal/apperr"
	"dvtech/qbn/internal/audit"
	"dvtech/qbn/internal/store"
	"dvtech/qbn/internal/store/sqlc"
)

// RouteInput son los datos de un "Tipo de notificación" (canal + plantilla +
// destino). El envío lo referencia por id o por nombre (NotificationType).
type RouteInput struct {
	NotificationType  string
	Channel           string
	TemplateKey       *string
	SMTPConnectionID  *uuid.UUID
	ChannelProviderID *uuid.UUID
	// SendPriority: prioridad que heredan los envíos de este Tipo cuando el
	// request no trae una explícita ("" => NORMAL).
	SendPriority string
	// PIIRetentionDays: días hasta anonimizar payload/recipient. NULL hereda
	// el valor de la aplicación.
	PIIRetentionDays *int32
	// RetentionDays: días hasta el borrado físico (notificación + intentos +
	// logs). NULL = no se borra nunca.
	RetentionDays *int32
}

// ListRoutes lista las rutas de canal de una app.
func (s *Service) ListRoutes(ctx context.Context, appID uuid.UUID) ([]sqlc.ChannelRoute, error) {
	rs, err := s.db.Q.ListChannelRoutes(ctx, appID)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return rs, nil
}

// CreateRoute crea un "Tipo de notificación" (canal + plantilla + destino).
// Valida el destino según el canal: EMAIL exige SMTP; SMS exige proveedor; PUSH
// admite proveedor opcional (sin él se entrega como bandeja interna hasta que se
// configure FCM/APNS); IN_APP no lleva destino externo.
func (s *Service) CreateRoute(ctx context.Context, actor, appID uuid.UUID, in RouteInput) (sqlc.ChannelRoute, error) {
	if err := validateRouteDestination(in); err != nil {
		return sqlc.ChannelRoute{}, err
	}
	sendPriority, err := normalizeSendPriority(in.SendPriority)
	if err != nil {
		return sqlc.ChannelRoute{}, err
	}
	if err := validateRetention(in.PIIRetentionDays, in.RetentionDays); err != nil {
		return sqlc.ChannelRoute{}, err
	}
	var route sqlc.ChannelRoute
	err = s.db.Tx(ctx, func(q *sqlc.Queries) error {
		var err error
		route, err = q.CreateChannelRoute(ctx, sqlc.CreateChannelRouteParams{
			ApplicationID:     appID,
			NotificationType:  in.NotificationType,
			Channel:           sqlc.Channel(in.Channel),
			TemplateKey:       in.TemplateKey,
			SmtpConnectionID:  optUUID(in.SMTPConnectionID),
			ChannelProviderID: optUUID(in.ChannelProviderID),
			SendPriority:      sendPriority,
			PiiRetentionDays:  in.PIIRetentionDays,
			RetentionDays:     in.RetentionDays,
		})
		if err != nil {
			return err
		}
		return s.rec(ctx, q, actor, &appID, audit.ActionRouteCreate, "channel_route", route.ID.String())
	})
	if err != nil {
		return sqlc.ChannelRoute{}, store.MapError(err, "", "route_exists")
	}
	return route, nil
}

// RouteSettings son los ajustes operables del Tipo (lo editable sin recrearlo).
type RouteSettings struct {
	SendPriority     string
	PIIRetentionDays *int32 // NULL hereda el de la aplicación
	RetentionDays    *int32 // NULL = no borrar nunca
}

// UpdateRouteSettings cambia prioridad y retención del Tipo; el resto se
// recrea (cambiarlo perdería el id que los clientes referencian).
func (s *Service) UpdateRouteSettings(ctx context.Context, actor, appID, id uuid.UUID, in RouteSettings) (sqlc.ChannelRoute, error) {
	sendPriority, err := normalizeSendPriority(in.SendPriority)
	if err != nil {
		return sqlc.ChannelRoute{}, err
	}
	if err := validateRetention(in.PIIRetentionDays, in.RetentionDays); err != nil {
		return sqlc.ChannelRoute{}, err
	}
	var route sqlc.ChannelRoute
	err = s.db.Tx(ctx, func(q *sqlc.Queries) error {
		var err error
		route, err = q.UpdateChannelRouteSettings(ctx, sqlc.UpdateChannelRouteSettingsParams{
			ID: id, ApplicationID: appID, SendPriority: sendPriority,
			PiiRetentionDays: in.PIIRetentionDays, RetentionDays: in.RetentionDays,
		})
		if err != nil {
			return err
		}
		return s.rec(ctx, q, actor, &appID, audit.ActionRouteUpdate, "channel_route", route.ID.String())
	})
	if err != nil {
		return sqlc.ChannelRoute{}, store.MapError(err, "route_not_found", "")
	}
	return route, nil
}

// validateRetention exige valores positivos y que el borrado no ocurra antes
// que la anonimización cuando ambos están definidos.
func validateRetention(piiDays, delDays *int32) error {
	if piiDays != nil && *piiDays <= 0 {
		return apperr.Validation("invalid_retention", "piiRetentionDays debe ser > 0")
	}
	if delDays != nil && *delDays <= 0 {
		return apperr.Validation("invalid_retention", "retentionDays debe ser > 0")
	}
	if piiDays != nil && delDays != nil && *delDays < *piiDays {
		return apperr.Validation("invalid_retention", "retentionDays debe ser >= piiRetentionDays (primero se anonimiza, después se borra)")
	}
	return nil
}

// DeleteRoute elimina una ruta de canal.
func (s *Service) DeleteRoute(ctx context.Context, actor, appID, id uuid.UUID) error {
	return s.db.Tx(ctx, func(q *sqlc.Queries) error {
		if err := q.DeleteChannelRoute(ctx, sqlc.DeleteChannelRouteParams{ID: id, ApplicationID: appID}); err != nil {
			return err
		}
		return s.rec(ctx, q, actor, &appID, audit.ActionRouteDelete, "channel_route", id.String())
	})
}

// validateRouteDestination comprueba que el destino encaje con el canal y que el
// Tipo tenga una plantilla asignada.
func validateRouteDestination(in RouteInput) error {
	if in.TemplateKey == nil || *in.TemplateKey == "" {
		return apperr.Validation("template_required", "Debes elegir una plantilla para el Tipo de notificación")
	}
	switch sqlc.Channel(in.Channel) {
	case sqlc.ChannelEMAIL:
		if in.SMTPConnectionID == nil {
			return apperr.Validation("smtp_required", "El canal EMAIL requiere una conexión SMTP")
		}
		if in.ChannelProviderID != nil {
			return apperr.Validation("invalid_destination", "El canal EMAIL no usa proveedor")
		}
	case sqlc.ChannelSMS:
		if in.ChannelProviderID == nil {
			return apperr.Validation("provider_required", "El canal SMS requiere un proveedor")
		}
		if in.SMTPConnectionID != nil {
			return apperr.Validation("invalid_destination", "El canal SMS no usa conexión SMTP")
		}
	case sqlc.ChannelPUSH:
		// PUSH admite proveedor opcional (sin él, entrega como bandeja interna).
		if in.SMTPConnectionID != nil {
			return apperr.Validation("invalid_destination", "El canal PUSH no usa conexión SMTP")
		}
	case sqlc.ChannelINAPP:
		if in.SMTPConnectionID != nil || in.ChannelProviderID != nil {
			return apperr.Validation("invalid_destination", "El canal IN_APP no lleva destino externo")
		}
	default:
		return apperr.Validation("invalid_channel", "Canal no válido")
	}
	return nil
}

// normalizeSendPriority valida la prioridad de envío del Tipo ("" => NORMAL).
// Rechaza valores desconocidos en vez de degradarlos en silencio.
func normalizeSendPriority(p string) (sqlc.Priority, error) {
	switch p {
	case "":
		return sqlc.PriorityNORMAL, nil
	case "LOW", "NORMAL", "HIGH", "URGENT":
		return sqlc.Priority(p), nil
	default:
		return "", apperr.Validation("invalid_priority", "sendPriority debe ser LOW, NORMAL, HIGH o URGENT")
	}
}

func optUUID(id *uuid.UUID) pgtype.UUID {
	if id == nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: *id, Valid: true}
}
