package channels

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"dvtech/qbn/internal/domain"
)

// PushSender envía notificaciones push según el proveedor de la ruta. Soporta
// FCM (HTTP con server key) y APNS (token JWT ES256). FCM HTTP v1 (OAuth2) queda
// como ampliación; un proveedor no soportado devuelve un error claro.
type PushSender struct {
	http *http.Client
	apns *tokenCache
}

// NewPushSender crea el sender push con un cliente HTTP con timeout.
func NewPushSender() *PushSender {
	return &PushSender{
		http: &http.Client{Timeout: 15 * time.Second},
		apns: newTokenCache(),
	}
}

func (s *PushSender) Channel() string { return domain.ChannelPush }

func (s *PushSender) Send(ctx context.Context, msg RenderedMessage, route ResolvedRoute) (ProviderRef, error) {
	if route.Provider == nil {
		// Sin proveedor (FCM/APNS): PUSH se entrega como bandeja interna. La
		// notificación ya está persistida; la app la lee mientras está abierta.
		return ProviderRef("push:stored"), nil
	}
	if msg.Recipient.PushToken == "" {
		return "", fmt.Errorf("falta el push token del destinatario")
	}
	switch strings.ToUpper(route.Provider.Provider) {
	case "FCM":
		return s.sendFCM(ctx, msg, *route.Provider)
	case "APNS":
		return s.sendAPNS(ctx, msg, *route.Provider)
	default:
		return "", fmt.Errorf("proveedor PUSH no soportado: %s", route.Provider.Provider)
	}
}

// sendFCM publica en el endpoint HTTP de FCM con autenticación por server key
// (cabecera `Authorization: key=<serverKey>`). Config esperada:
//
//	serverKey[, endpoint]
func (s *PushSender) sendFCM(ctx context.Context, msg RenderedMessage, p ProviderConfig) (ProviderRef, error) {
	serverKey := p.String("serverKey")
	if serverKey == "" {
		return "", fmt.Errorf("fcm: falta serverKey en la config")
	}
	endpoint := p.String("endpoint")
	if endpoint == "" {
		endpoint = "https://fcm.googleapis.com/fcm/send"
	}

	payload := map[string]any{
		"to": msg.Recipient.PushToken,
		"notification": map[string]string{
			"title": msg.Subject,
			"body":  messageText(msg),
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "key="+serverKey)
	req.Header.Set("Content-Type", "application/json")

	res, err := s.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("fcm: %w", err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("fcm: status %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}

	// Respuesta FCM: {"multicast_id":N,"success":1,"failure":0,"results":[{...}]}
	var out struct {
		Failure     int   `json:"failure"`
		MulticastID int64 `json:"multicast_id"`
		Results     []struct {
			MessageID string `json:"message_id"`
			Error     string `json:"error"`
		} `json:"results"`
	}
	_ = json.Unmarshal(body, &out)
	if out.Failure > 0 && len(out.Results) > 0 && out.Results[0].Error != "" {
		return "", fmt.Errorf("fcm: rechazado: %s", out.Results[0].Error)
	}
	if len(out.Results) > 0 && out.Results[0].MessageID != "" {
		return ProviderRef(out.Results[0].MessageID), nil
	}
	return ProviderRef(fmt.Sprintf("%d", out.MulticastID)), nil
}
