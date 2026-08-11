// Package channels abstrae el envío por cada canal (Email, In-App, Push, SMS)
// tras una interfaz común Sender. Añadir un canal/proveedor nuevo = implementar
// Sender + registrarlo en el Dispatcher; el resto del sistema no cambia.
package channels

import (
	"context"
	"fmt"
)

// ProviderRef es el identificador que devuelve el proveedor para el mensaje
// enviado (p. ej. el Message-ID SMTP), útil para trazabilidad.
type ProviderRef string

// Recipient son los datos de destino (según el canal se usa uno u otro campo).
type Recipient struct {
	Email     string
	Name      string
	Phone     string
	PushToken string
}

// Attachment es un adjunto ya resuelto en memoria (contenido descargado/decodificado).
type Attachment struct {
	Filename    string
	ContentType string
	Content     []byte
}

// RenderedMessage es el contenido ya renderizado, listo para enviar.
type RenderedMessage struct {
	Subject     string
	HTMLBody    string
	Recipient   Recipient
	Attachments []Attachment
}

// SMTPConfig son los datos (ya descifrados) de una conexión SMTP.
type SMTPConfig struct {
	Host      string
	Port      int
	Username  string
	Password  string
	FromEmail string
	FromName  string
	Secure    bool
}

// ProviderConfig lleva la configuración (ya descifrada) de un ChannelProvider
// para los canales no-email (Push/SMS). Provider identifica la integración
// concreta (FCM, TWILIO…) y Config son sus credenciales/ajustes.
type ProviderConfig struct {
	Provider string         // FCM | APNS | TWILIO | …
	Config   map[string]any // credenciales y ajustes descifrados
}

// String devuelve un valor de config como cadena (vacío si falta o no es texto).
func (p ProviderConfig) String(key string) string {
	if v, ok := p.Config[key].(string); ok {
		return v
	}
	return ""
}

// ResolvedRoute lleva la configuración concreta del proveedor para este envío,
// resultado de resolver el ChannelRoute.
type ResolvedRoute struct {
	Channel  string
	SMTP     *SMTPConfig     // presente si Channel == EMAIL
	Provider *ProviderConfig // presente si Channel == PUSH | SMS
}

// Sender envía un mensaje renderizado por un canal concreto.
type Sender interface {
	Channel() string
	Send(ctx context.Context, msg RenderedMessage, route ResolvedRoute) (ProviderRef, error)
}

// Dispatcher enruta cada envío al Sender registrado para su canal.
type Dispatcher struct {
	senders map[string]Sender
}

// NewDispatcher registra los senders por su Channel().
func NewDispatcher(senders ...Sender) *Dispatcher {
	m := make(map[string]Sender, len(senders))
	for _, s := range senders {
		m[s.Channel()] = s
	}
	return &Dispatcher{senders: m}
}

// Send despacha al Sender del canal indicado.
func (d *Dispatcher) Send(ctx context.Context, channel string, msg RenderedMessage, route ResolvedRoute) (ProviderRef, error) {
	s, ok := d.senders[channel]
	if !ok {
		return "", fmt.Errorf("no hay sender registrado para el canal %q", channel)
	}
	return s.Send(ctx, msg, route)
}
