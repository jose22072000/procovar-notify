package safedial

import (
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// TestRedirectHopIsGuarded (H5): un 302 hacia una IP interna se bloquea cuando
// el cliente sigue el redirect, porque cada salto vuelve a pasar por el Control
// del dialer. Se permite SOLO el primer dial (alcanzar el redirector) para
// aislar que es el salto del redirect el que el guard real rechaza.
func TestRedirectHopIsGuarded(t *testing.T) {
	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("secreto interno"))
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
						return nil // primer salto: permitir alcanzar el redirector
					}
					return Control(network, address, c) // el redirect pasa por el guard real
				},
			}).DialContext,
		},
	}

	resp, err := client.Get(redir.URL)
	if err == nil {
		_ = resp.Body.Close()
		t.Fatal("el redirect a loopback debería bloquearse en el dial del guard")
	}
	if dials.Load() < 2 {
		t.Fatalf("debería haberse intentado el 2º dial (el del redirect), dials=%d", dials.Load())
	}
}

// TestNewClientBlocksLoopback (rebinding, H5): NewClient bloquea loopback por la
// IP ya resuelta, tanto si la URL trae 127.0.0.1 como si llega por el nombre
// 'localhost'. Un nombre que apunta a una IP interna (rebinding) no elude el
// guard porque la decisión se toma sobre la IP resuelta, no sobre el host.
func TestNewClientBlocksLoopback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("interno"))
	}))
	defer srv.Close()

	client := NewClient(5 * time.Second) // guard ON (allowPrivate=false por defecto)

	if resp, err := client.Get(srv.URL); err == nil {
		_ = resp.Body.Close()
		t.Fatal("debería bloquear el destino loopback (127.0.0.1)")
	}

	u, _ := url.Parse(srv.URL)
	byName := "http://localhost:" + u.Port() + "/"
	if resp, err := client.Get(byName); err == nil {
		_ = resp.Body.Close()
		t.Fatal("acceso por nombre 'localhost' (rebinding) a loopback debería bloquearse")
	}
}

// TestNewClientAllowsPublicWithOptIn: con el opt-in activo (entornos cerrados),
// el guard no bloquea; se restaura al terminar para no contaminar otros tests.
func TestControlOptIn(t *testing.T) {
	SetAllowPrivate(true)
	t.Cleanup(func() { SetAllowPrivate(false) })
	if err := Control("tcp", "127.0.0.1:80", nil); err != nil {
		t.Errorf("con allowPrivate el guard no debería bloquear: %v", err)
	}
}
