package notification

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"dvtech/qbn/internal/apperr"
	"dvtech/qbn/internal/store/sqlc"
)

// Preferences son las preferencias de notificación por canal de un usuario.
type Preferences struct {
	Email bool `json:"email"`
	Push  bool `json:"push"`
	SMS   bool `json:"sms"`
	InApp bool `json:"inApp"`
}

// defaultPreferences coincide con los DEFAULT de la tabla (opt-in salvo SMS).
func defaultPreferences() Preferences {
	return Preferences{Email: true, Push: true, SMS: false, InApp: true}
}

// GetPreferences devuelve las preferencias del usuario, o los valores por
// defecto si nunca se han fijado.
func (s *Service) GetPreferences(ctx context.Context, appID uuid.UUID, userID string) (Preferences, error) {
	p, err := s.db.Q.GetUserPreference(ctx, sqlc.GetUserPreferenceParams{ApplicationID: appID, UserID: userID})
	if errors.Is(err, pgx.ErrNoRows) {
		return defaultPreferences(), nil
	}
	if err != nil {
		return Preferences{}, apperr.Internal(err)
	}
	return Preferences{Email: p.EmailEnabled, Push: p.PushEnabled, SMS: p.SmsEnabled, InApp: p.InAppEnabled}, nil
}

// SetPreferences crea o actualiza las preferencias del usuario.
func (s *Service) SetPreferences(ctx context.Context, appID uuid.UUID, userID string, pref Preferences) (Preferences, error) {
	p, err := s.db.Q.UpsertUserPreference(ctx, sqlc.UpsertUserPreferenceParams{
		ApplicationID: appID,
		UserID:        userID,
		EmailEnabled:  pref.Email,
		PushEnabled:   pref.Push,
		SmsEnabled:    pref.SMS,
		InAppEnabled:  pref.InApp,
	})
	if err != nil {
		return Preferences{}, apperr.Internal(err)
	}
	return Preferences{Email: p.EmailEnabled, Push: p.PushEnabled, SMS: p.SmsEnabled, InApp: p.InAppEnabled}, nil
}
