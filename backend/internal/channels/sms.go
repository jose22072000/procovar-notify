package channels

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"dvtech/qbn/internal/domain"
)

// SMSSender envía SMS según el proveedor configurado en la ruta. Hoy soporta
// Twilio; otros proveedores devuelven un error claro manteniendo la abstracción.
type SMSSender struct {
	http *http.Client
}

// NewSMSSender crea el sender de SMS con un cliente HTTP con timeout.
func NewSMSSender() *SMSSender {
	return &SMSSender{http: &http.Client{Timeout: 15 * time.Second}}
}

func (s *SMSSender) Channel() string { return domain.ChannelSMS }

// Send despacha según el proveedor de la ruta resuelta.
func (s *SMSSender) Send(ctx context.Context, msg RenderedMessage, route ResolvedRoute) (ProviderRef, error) {
	if route.Provider == nil {
		return "", fmt.Errorf("el canal SMS requiere un proveedor en la ruta")
	}
	if msg.Recipient.Phone == "" {
		return "", fmt.Errorf("falta el teléfono del destinatario")
	}
	switch strings.ToUpper(route.Provider.Provider) {
	case "TWILIO":
		return s.sendTwilio(ctx, msg, *route.Provider)
	default:
		return "", fmt.Errorf("proveedor SMS no soportado: %s", route.Provider.Provider)
	}
}

// sendTwilio publica el mensaje en la API REST de Twilio (Messages.json) con
// autenticación básica (accountSid:authToken). Config esperada:
//
//	accountSid, authToken, from[, messagingServiceSid][, baseUrl]
func (s *SMSSender) sendTwilio(ctx context.Context, msg RenderedMessage, p ProviderConfig) (ProviderRef, error) {
	sid := p.String("accountSid")
	token := p.String("authToken")
	if sid == "" || token == "" {
		return "", fmt.Errorf("twilio: faltan accountSid/authToken en la config")
	}
	from := p.String("from")
	msvc := p.String("messagingServiceSid")
	if from == "" && msvc == "" {
		return "", fmt.Errorf("twilio: se requiere 'from' o 'messagingServiceSid'")
	}

	base := p.String("baseUrl")
	if base == "" {
		base = "https://api.twilio.com"
	}
	endpoint := fmt.Sprintf("%s/2010-04-01/Accounts/%s/Messages.json", strings.TrimRight(base, "/"), url.PathEscape(sid))

	form := url.Values{}
	form.Set("To", msg.Recipient.Phone)
	if msvc != "" {
		form.Set("MessagingServiceSid", msvc)
	} else {
		form.Set("From", from)
	}
	form.Set("Body", messageText(msg))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(sid, token)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	res, err := s.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("twilio: %w", err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		// Twilio responde {code, message, ...} en JSON ante errores.
		var e struct {
			Message string `json:"message"`
			Code    int    `json:"code"`
		}
		_ = json.Unmarshal(body, &e)
		if e.Message != "" {
			return "", fmt.Errorf("twilio: %d (code %d): %s", res.StatusCode, e.Code, e.Message)
		}
		return "", fmt.Errorf("twilio: status %d", res.StatusCode)
	}

	var ok struct {
		SID string `json:"sid"`
	}
	_ = json.Unmarshal(body, &ok)
	return ProviderRef(ok.SID), nil
}
