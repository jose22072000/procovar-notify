package notification

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"dvtech/qbn/internal/channels"
)

// Límites de adjuntos (§16: "límite de tamaño y si se aceptan por URL o inline").
const (
	maxAttachmentBytes = 10 << 20 // 10 MiB por adjunto
	maxAttachments     = 20
)

// attachmentSpec es el formato declarado en Notification.attachments:
// [{filename, contentType, url | contentBase64}].
type attachmentSpec struct {
	Filename      string `json:"filename"`
	ContentType   string `json:"contentType"`
	URL           string `json:"url"`
	ContentBase64 string `json:"contentBase64"`
}

// attachmentFetcher es el cliente HTTP usado para los adjuntos por URL. Tiene un
// timeout acotado, el tamaño se limita con un io.LimitReader al leer, y el
// dialer aplica el guard anti-SSRF (ssrfControl) sobre la IP ya resuelta.
var attachmentFetcher = &http.Client{
	Timeout: 20 * time.Second,
	Transport: &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: 10 * time.Second,
			Control: ssrfControl,
		}).DialContext,
	},
	// Re-aplica la allowlist de host en cada redirect (defensa en profundidad
	// sobre el guard por IP, que ya actúa en el dialer también en los saltos). L4.
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("demasiados redirects")
		}
		return checkHostAllowed(req.URL.Hostname())
	},
}

// resolveAttachments materializa los adjuntos declarados: decodifica los inline
// (base64) y descarga los referenciados por URL (con tope de tamaño). Devuelve
// error si algún adjunto no se puede resolver, para que el envío reintente/falle
// de forma visible en lugar de mandar un correo incompleto.
func resolveAttachments(ctx context.Context, specs []attachmentSpec) ([]channels.Attachment, error) {
	if len(specs) == 0 {
		return nil, nil
	}
	if len(specs) > maxAttachments {
		return nil, fmt.Errorf("demasiados adjuntos: %d (máx %d)", len(specs), maxAttachments)
	}
	out := make([]channels.Attachment, 0, len(specs))
	for i, s := range specs {
		name := s.Filename
		if name == "" {
			name = fmt.Sprintf("attachment-%d", i+1)
		}
		var content []byte
		switch {
		case s.ContentBase64 != "":
			b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s.ContentBase64))
			if err != nil {
				return nil, fmt.Errorf("adjunto %q: base64 inválido: %w", name, err)
			}
			if len(b) > maxAttachmentBytes {
				return nil, fmt.Errorf("adjunto %q supera el tamaño máximo", name)
			}
			content = b
		case s.URL != "":
			b, err := fetchAttachment(ctx, s.URL)
			if err != nil {
				return nil, fmt.Errorf("adjunto %q: %w", name, err)
			}
			content = b
		default:
			return nil, fmt.Errorf("adjunto %q: requiere contentBase64 o url", name)
		}
		out = append(out, channels.Attachment{Filename: name, ContentType: s.ContentType, Content: content})
	}
	return out, nil
}

// fetchAttachment descarga un adjunto por URL (solo http/https) con tope de
// tamaño. El guard anti-SSRF se aplica en el dialer (ssrfControl): bloquea por
// la IP ya resuelta (loopback/privadas/link-local) salvo opt-in explícito.
func fetchAttachment(ctx context.Context, rawURL string) ([]byte, error) {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("esquema de URL no soportado")
	}
	if err := checkHostAllowed(u.Hostname()); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	res, err := attachmentFetcher.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("descarga falló: status %d", res.StatusCode)
	}
	// +1 para detectar que el contenido excede el límite.
	b, err := io.ReadAll(io.LimitReader(res.Body, maxAttachmentBytes+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxAttachmentBytes {
		return nil, fmt.Errorf("supera el tamaño máximo (%d bytes)", maxAttachmentBytes)
	}
	return b, nil
}
