package admin

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"dvtech/qbn/internal/apperr"
	"dvtech/qbn/internal/audit"
	"dvtech/qbn/internal/channels"
	"dvtech/qbn/internal/store"
	"dvtech/qbn/internal/store/sqlc"
)

// SMTPInput son los datos de una conexión SMTP (password en claro; se cifra).
type SMTPInput struct {
	Name      string
	Host      string
	Port      int32
	Username  string
	Password  string
	FromEmail string
	FromName  string
	Secure    bool
	Status    string // ACTIVE | DISABLED (solo en update)
}

// ListSMTP lista las conexiones SMTP de una app.
func (s *Service) ListSMTP(ctx context.Context, appID uuid.UUID) ([]sqlc.SmtpConnection, error) {
	cs, err := s.db.Q.ListSmtpConnections(ctx, appID)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return cs, nil
}

// CreateSMTP crea una conexión SMTP (cifra el password).
func (s *Service) CreateSMTP(ctx context.Context, actor, appID uuid.UUID, in SMTPInput) (sqlc.SmtpConnection, error) {
	pwdEnc, err := s.enc.Encrypt([]byte(in.Password))
	if err != nil {
		return sqlc.SmtpConnection{}, apperr.Internal(err)
	}
	var c sqlc.SmtpConnection
	err = s.db.Tx(ctx, func(q *sqlc.Queries) error {
		var err error
		c, err = q.CreateSmtpConnection(ctx, sqlc.CreateSmtpConnectionParams{
			ApplicationID: appID, Name: in.Name, Host: in.Host, Port: in.Port,
			Username: in.Username, PasswordEnc: pwdEnc, FromEmail: in.FromEmail,
			FromName: in.FromName, Secure: in.Secure,
		})
		if err != nil {
			return err
		}
		return s.rec(ctx, q, actor, &appID, audit.ActionSMTPCreate, "smtp_connection", c.ID.String())
	})
	if err != nil {
		return sqlc.SmtpConnection{}, store.MapError(err, "", "smtp_name_exists")
	}
	return c, nil
}

// UpdateSMTP actualiza una conexión SMTP (re-cifra el password si se provee).
func (s *Service) UpdateSMTP(ctx context.Context, actor, appID, id uuid.UUID, in SMTPInput) (sqlc.SmtpConnection, error) {
	current, err := s.getSMTP(ctx, appID, id)
	if err != nil {
		return sqlc.SmtpConnection{}, err
	}

	pwdEnc := current.PasswordEnc
	if in.Password != "" {
		if pwdEnc, err = s.enc.Encrypt([]byte(in.Password)); err != nil {
			return sqlc.SmtpConnection{}, apperr.Internal(err)
		}
	}
	status := sqlc.StatusActiveDisabled(in.Status)
	if in.Status == "" {
		status = current.Status
	}

	var c sqlc.SmtpConnection
	err = s.db.Tx(ctx, func(q *sqlc.Queries) error {
		var err error
		c, err = q.UpdateSmtpConnection(ctx, sqlc.UpdateSmtpConnectionParams{
			ID: id, ApplicationID: appID, Name: in.Name, Host: in.Host, Port: in.Port,
			Username: in.Username, PasswordEnc: pwdEnc, FromEmail: in.FromEmail,
			FromName: in.FromName, Secure: in.Secure, Status: status,
		})
		if err != nil {
			return err
		}
		return s.rec(ctx, q, actor, &appID, audit.ActionSMTPUpdate, "smtp_connection", id.String())
	})
	if err != nil {
		return sqlc.SmtpConnection{}, apperr.Internal(err)
	}
	return c, nil
}

// DeleteSMTP elimina una conexión SMTP.
func (s *Service) DeleteSMTP(ctx context.Context, actor, appID, id uuid.UUID) error {
	if _, err := s.getSMTP(ctx, appID, id); err != nil {
		return err
	}
	return s.db.Tx(ctx, func(q *sqlc.Queries) error {
		if err := q.DeleteSmtpConnection(ctx, sqlc.DeleteSmtpConnectionParams{ID: id, ApplicationID: appID}); err != nil {
			return err
		}
		return s.rec(ctx, q, actor, &appID, audit.ActionSMTPDelete, "smtp_connection", id.String())
	})
}

// TestSMTP prueba la conexión de una conexión SMTP existente (descifra password).
func (s *Service) TestSMTP(ctx context.Context, appID, id uuid.UUID) error {
	c, err := s.getSMTP(ctx, appID, id)
	if err != nil {
		return err
	}
	password, err := s.enc.Decrypt(c.PasswordEnc)
	if err != nil {
		return apperr.Internal(err)
	}
	if derr := channels.DialTest(ctx, channels.SMTPConfig{
		Host: c.Host, Port: int(c.Port), Username: c.Username, Password: string(password),
		FromEmail: c.FromEmail, FromName: c.FromName, Secure: c.Secure,
	}); derr != nil {
		return apperr.Validation("smtp_test_failed", derr.Error())
	}
	return nil
}

func (s *Service) getSMTP(ctx context.Context, appID, id uuid.UUID) (sqlc.SmtpConnection, error) {
	c, err := s.db.Q.GetSmtpConnection(ctx, sqlc.GetSmtpConnectionParams{ID: id, ApplicationID: appID})
	if errors.Is(err, pgx.ErrNoRows) {
		return sqlc.SmtpConnection{}, apperr.NotFound("smtp_not_found", "SMTP connection not found")
	}
	if err != nil {
		return sqlc.SmtpConnection{}, apperr.Internal(err)
	}
	return c, nil
}
