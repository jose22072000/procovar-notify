package notification

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"dvtech/qbn/internal/apperr"
	"dvtech/qbn/internal/store/sqlc"
)

// Límites de paginación.
const (
	defaultPageLimit = 20
	maxPageLimit     = 100
)

// ListParams son los filtros y paginación para listar notificaciones.
type ListParams struct {
	ApplicationID   uuid.UUID
	Status          *string
	Channel         *string
	RecipientUserID *string
	StartDate       *time.Time
	EndDate         *time.Time
	Cursor          string
	Limit           int
	// Archived: nil/false = solo las activas (lo que pide la campana), true = solo
	// el archivo. Sin esto la bandeja crecería sin fin.
	Archived *bool
}

// ListResult es una página de notificaciones con el cursor a la siguiente.
type ListResult struct {
	Items      []sqlc.Notification
	NextCursor string
}

// List devuelve una página de notificaciones del tenant (orden descendente por
// fecha) aplicando los filtros dados y paginación por cursor (keyset).
func (s *Service) List(ctx context.Context, p ListParams) (ListResult, error) {
	limit := p.Limit
	if limit <= 0 {
		limit = defaultPageLimit
	}
	if limit > maxPageLimit {
		limit = maxPageLimit
	}

	cursorCreated, cursorID := pgtype.Timestamptz{}, pgtype.UUID{}
	if p.Cursor != "" {
		t, id, err := decodeCursor(p.Cursor)
		if err != nil {
			return ListResult{}, apperr.Validation("invalid_cursor", "Invalid pagination cursor")
		}
		cursorCreated = pgtype.Timestamptz{Time: t, Valid: true}
		cursorID = pgtype.UUID{Bytes: id, Valid: true}
	}

	rows, err := s.db.Q.ListNotificationsCursor(ctx, sqlc.ListNotificationsCursorParams{
		ApplicationID:   p.ApplicationID,
		Archived:        p.Archived, // nil = solo activas (lo que pide la campana)
		Status:          toStatusPtr(p.Status),
		Channel:         toChannelPtr(p.Channel),
		RecipientUserID: emptyToNil(p.RecipientUserID),
		StartDate:       toTimestamptz(p.StartDate),
		EndDate:         toTimestamptz(p.EndDate),
		CursorCreated:   cursorCreated,
		CursorID:        cursorID,
		PageLimit:       int32(limit + 1), // +1 para detectar si hay más
	})
	if err != nil {
		return ListResult{}, apperr.Internal(err)
	}

	res := ListResult{}
	if len(rows) > limit {
		last := rows[limit-1]
		res.NextCursor = encodeCursor(last.CreatedAt.Time, last.ID)
		rows = rows[:limit]
	}
	res.Items = rows
	return res, nil
}

// Get devuelve una notificación del tenant (404 si no existe).
func (s *Service) Get(ctx context.Context, appID, id uuid.UUID) (sqlc.Notification, error) {
	n, err := s.db.Q.GetNotification(ctx, sqlc.GetNotificationParams{ID: id, ApplicationID: appID})
	if errors.Is(err, pgx.ErrNoRows) {
		return sqlc.Notification{}, apperr.NotFound("notification_not_found", "Notification not found")
	}
	if err != nil {
		return sqlc.Notification{}, apperr.Internal(err)
	}
	return n, nil
}

// Cancel cancela una notificación aún no enviada (409 si ya no es cancelable).
func (s *Service) Cancel(ctx context.Context, appID, id uuid.UUID) (sqlc.Notification, error) {
	n, err := s.db.Q.CancelNotification(ctx, sqlc.CancelNotificationParams{ID: id, ApplicationID: appID})
	if errors.Is(err, pgx.ErrNoRows) {
		// O no existe, o no está en un estado cancelable.
		if _, gerr := s.Get(ctx, appID, id); gerr != nil {
			return sqlc.Notification{}, gerr // 404
		}
		return sqlc.Notification{}, apperr.Conflict("not_cancellable", "Notification can no longer be cancelled")
	}
	if err != nil {
		return sqlc.Notification{}, apperr.Internal(err)
	}
	warnErr(s.logger, "no se pudo escribir el log de auditoría", s.db.Q.CreateNotificationLog(ctx, sqlc.CreateNotificationLogParams{
		ApplicationID: appID, NotificationID: pgUUID(id), Event: "cancelled",
	}))
	return n, nil
}

// MarkRead marca una notificación como leída (404 si no existe).
func (s *Service) MarkRead(ctx context.Context, appID, id uuid.UUID) (sqlc.Notification, error) {
	n, err := s.db.Q.MarkNotificationReadByID(ctx, sqlc.MarkNotificationReadByIDParams{ID: id, ApplicationID: appID})
	if errors.Is(err, pgx.ErrNoRows) {
		return sqlc.Notification{}, apperr.NotFound("notification_not_found", "Notification not found")
	}
	if err != nil {
		return sqlc.Notification{}, apperr.Internal(err)
	}
	return n, nil
}

// Archive saca la notificación de la bandeja sin borrarla ni perder su estado de
// entrega (archivar es una acción del destinatario, ortogonal a SENT/READ).
// Idempotente: re-archivar no mueve la fecha.
func (s *Service) Archive(ctx context.Context, appID, id uuid.UUID) (sqlc.Notification, error) {
	n, err := s.db.Q.ArchiveNotification(ctx, sqlc.ArchiveNotificationParams{ID: id, ApplicationID: appID})
	if errors.Is(err, pgx.ErrNoRows) {
		return sqlc.Notification{}, apperr.NotFound("notification_not_found", "Notification not found")
	}
	if err != nil {
		return sqlc.Notification{}, apperr.Internal(err)
	}
	return n, nil
}

// Unarchive la devuelve a la bandeja.
func (s *Service) Unarchive(ctx context.Context, appID, id uuid.UUID) (sqlc.Notification, error) {
	n, err := s.db.Q.UnarchiveNotification(ctx, sqlc.UnarchiveNotificationParams{ID: id, ApplicationID: appID})
	if errors.Is(err, pgx.ErrNoRows) {
		return sqlc.Notification{}, apperr.NotFound("notification_not_found", "Notification not found")
	}
	if err != nil {
		return sqlc.Notification{}, apperr.Internal(err)
	}
	return n, nil
}

// ArchiveAllRead archiva de golpe todas las leídas de un usuario: es la acción que
// de verdad limpia la bandeja. Va scopeada por destinatario, así que nunca puede
// tocar las de otro usuario.
func (s *Service) ArchiveAllRead(ctx context.Context, appID uuid.UUID, userID string) error {
	err := s.db.Q.ArchiveReadNotificationsByUser(ctx, sqlc.ArchiveReadNotificationsByUserParams{
		ApplicationID: appID, RecipientUserID: &userID,
	})
	if err != nil {
		return apperr.Internal(err)
	}
	return nil
}

// --- cursor (keyset created_at, id) ---

func encodeCursor(t time.Time, id uuid.UUID) string {
	raw := fmt.Sprintf("%d|%s", t.UnixNano(), id.String())
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeCursor(s string) (time.Time, uuid.UUID, error) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return time.Time{}, uuid.Nil, err
	}
	parts := strings.SplitN(string(raw), "|", 2)
	if len(parts) != 2 {
		return time.Time{}, uuid.Nil, errors.New("cursor mal formado")
	}
	nanos, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return time.Time{}, uuid.Nil, err
	}
	id, err := uuid.Parse(parts[1])
	if err != nil {
		return time.Time{}, uuid.Nil, err
	}
	return time.Unix(0, nanos), id, nil
}

// --- conversores a parámetros sqlc ---

func toStatusPtr(s *string) *sqlc.NotificationStatus {
	if s == nil || *s == "" {
		return nil
	}
	v := sqlc.NotificationStatus(*s)
	return &v
}

func toChannelPtr(s *string) *sqlc.Channel {
	if s == nil || *s == "" {
		return nil
	}
	v := sqlc.Channel(*s)
	return &v
}

func emptyToNil(s *string) *string {
	if s == nil || *s == "" {
		return nil
	}
	return s
}

func toTimestamptz(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}
