package notification

import (
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// TestAttachmentRedirectBlocked (H5): un adjunto cuyo servidor responde 302 hacia
// loopback se bloquea en el salto del redirect, porque cada dial vuelve a pasar
// por ssrfControl (el guard de adjuntos). Se permite SOLO el primer dial para
// aislar que es el salto del redirect el que el guard rechaza.
func TestAttachmentRedirectBlocked(t *testing.T) {
	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("contenido interno"))
	}))
	defer internal.Close()
	redir := httptest.NewServer(http.RedirectHandler(internal.URL, http.StatusFound))
	defer redir.Close()

	var dials atomic.Int32
	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Control: func(network, address string, c syscall.RawConn) error {
					if dials.Add(1) == 1 {
						return nil // primer salto: alcanzar el redirector
					}
					return ssrfControl(network, address, c) // el redirect pasa por el guard real
				},
			}).DialContext,
		},
	}

	resp, err := client.Get(redir.URL)
	if err == nil {
		_ = resp.Body.Close()
		t.Fatal("el redirect a loopback debería bloquearse por ssrfControl")
	}
	if dials.Load() < 2 {
		t.Fatalf("debería haberse intentado el 2º dial (el del redirect), dials=%d", dials.Load())
	}
}
