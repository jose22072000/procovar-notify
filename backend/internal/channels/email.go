package channels

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/wneessen/go-mail"

	"dvtech/qbn/internal/domain"
)

// EmailSender envía correos por SMTP usando la conexión de la ResolvedRoute.
type EmailSender struct{}

// NewEmailSender crea el sender de email.
func NewEmailSender() *EmailSender { return &EmailSender{} }

func (s *EmailSender) Channel() string { return domain.ChannelEmail }

// Send construye el correo y lo envía por la conexión SMTP resuelta. Devuelve el
// Message-ID como ProviderRef.
func (s *EmailSender) Send(ctx context.Context, msg RenderedMessage, route ResolvedRoute) (ProviderRef, error) {
	if route.SMTP == nil {
		return "", errors.New("el canal EMAIL requiere una conexión SMTP en la ruta")
	}
	if msg.Recipient.Email == "" {
		// Dato del cliente, no fallo de infraestructura: permanente (no abre el breaker).
		return "", Permanent(errors.New("falta el email del destinatario"))
	}
	cfg := route.SMTP

	m := mail.NewMsg()
	if err := m.FromFormat(cfg.FromName, cfg.FromEmail); err != nil {
		return "", fmt.Errorf("from inválido: %w", err)
	}
	if err := m.AddToFormat(msg.Recipient.Name, msg.Recipient.Email); err != nil {
		return "", fmt.Errorf("destinatario inválido: %w", err)
	}
	m.Subject(msg.Subject)
	m.SetBodyString(mail.TypeTextHTML, msg.HTMLBody)

	for _, att := range msg.Attachments {
		var opts []mail.FileOption
		if att.ContentType != "" {
			opts = append(opts, mail.WithFileContentType(mail.ContentType(att.ContentType)))
		}
		if err := m.AttachReader(att.Filename, bytes.NewReader(att.Content), opts...); err != nil {
			return "", fmt.Errorf("adjuntando %q: %w", att.Filename, err)
		}
	}

	messageID := uuid.NewString() + "@procovar.cloud"
	m.SetMessageIDWithValue(messageID)

	client, err := newSMTPClient(cfg)
	if err != nil {
		return "", err
	}
	if err := client.DialAndSendWithContext(ctx, m); err != nil {
		return "", fmt.Errorf("envío SMTP: %w", err)
	}
	return ProviderRef(messageID), nil
}

// DialTest verifica la conexión SMTP (conecta y cierra) sin enviar nada. Lo usa
// el panel de administración para el botón "probar conexión".
func DialTest(ctx context.Context, cfg SMTPConfig) error {
	client, err := newSMTPClient(&cfg)
	if err != nil {
		return err
	}
	if err := client.DialWithContext(ctx); err != nil {
		return fmt.Errorf("conexión SMTP: %w", err)
	}
	return client.Close()
}

// newSMTPClient construye el cliente go-mail según la configuración: cifrado si
// Secure — SSL implícito en el puerto 465, STARTTLS en el resto (587, …) —,
// autenticación solo si hay usuario (MailHog en local no usa ninguna).
func newSMTPClient(cfg *SMTPConfig) (*mail.Client, error) {
	opts := []mail.Option{
		mail.WithPort(cfg.Port),
		mail.WithTimeout(15 * time.Second),
	}
	switch {
	case cfg.Secure && cfg.Port == 465:
		opts = append(opts, mail.WithSSL())
	case cfg.Secure:
		opts = append(opts, mail.WithTLSPolicy(mail.TLSMandatory))
	default:
		opts = append(opts, mail.WithTLSPolicy(mail.NoTLS))
	}
	if cfg.Username != "" {
		opts = append(opts,
			mail.WithSMTPAuth(mail.SMTPAuthPlain),
			mail.WithUsername(cfg.Username),
			mail.WithPassword(cfg.Password),
		)
	}
	client, err := mail.NewClient(cfg.Host, opts...)
	if err != nil {
		return nil, fmt.Errorf("creando cliente SMTP: %w", err)
	}
	return client, nil
}
