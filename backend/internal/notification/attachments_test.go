package notification

import (
	"context"
	"encoding/base64"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResolveAttachments_Base64(t *testing.T) {
	content := []byte("hola mundo PDF")
	specs := []attachmentSpec{{
		Filename:      "doc.pdf",
		ContentType:   "application/pdf",
		ContentBase64: base64.StdEncoding.EncodeToString(content),
	}}
	got, err := resolveAttachments(context.Background(), specs)
	if err != nil {
		t.Fatalf("resolveAttachments: %v", err)
	}
	if len(got) != 1 || got[0].Filename != "doc.pdf" || string(got[0].Content) != "hola mundo PDF" {
		t.Fatalf("adjunto mal resuelto: %+v", got)
	}
	if got[0].ContentType != "application/pdf" {
		t.Errorf("contentType = %q", got[0].ContentType)
	}
}

// allowPrivateForTest habilita el acceso a loopback (httptest) durante el test
// y lo restaura al terminar.
func allowPrivateForTest(t *testing.T) {
	t.Helper()
	SetAllowPrivateHosts(true)
	t.Cleanup(func() { SetAllowPrivateHosts(false) })
}

func TestResolveAttachments_URL(t *testing.T) {
	allowPrivateForTest(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("contenido remoto"))
	}))
	defer srv.Close()

	got, err := resolveAttachments(context.Background(), []attachmentSpec{{URL: srv.URL}})
	if err != nil {
		t.Fatalf("resolveAttachments: %v", err)
	}
	if len(got) != 1 || string(got[0].Content) != "contenido remoto" {
		t.Fatalf("adjunto remoto mal resuelto: %+v", got)
	}
	if got[0].Filename != "attachment-1" {
		t.Errorf("filename por defecto = %q", got[0].Filename)
	}
}

func TestResolveAttachments_Empty(t *testing.T) {
	got, err := resolveAttachments(context.Background(), nil)
	if err != nil || got != nil {
		t.Fatalf("vacío debería devolver (nil,nil): %v %v", got, err)
	}
}

func TestResolveAttachments_Errors(t *testing.T) {
	ctx := context.Background()
	cases := map[string][]attachmentSpec{
		"base64 inválido": {{Filename: "x", ContentBase64: "@@no-base64@@"}},
		"sin contenido":   {{Filename: "x"}},
		"esquema malo":    {{Filename: "x", URL: "ftp://host/f"}},
	}
	for name, specs := range cases {
		if _, err := resolveAttachments(ctx, specs); err == nil {
			t.Errorf("%s: se esperaba error", name)
		}
	}
}

func TestResolveAttachments_TooMany(t *testing.T) {
	specs := make([]attachmentSpec, maxAttachments+1)
	for i := range specs {
		specs[i] = attachmentSpec{ContentBase64: base64.StdEncoding.EncodeToString([]byte("x"))}
	}
	if _, err := resolveAttachments(context.Background(), specs); err == nil {
		t.Fatal("se esperaba error por exceso de adjuntos")
	}
}

func TestResolveAttachments_TooLargeBase64(t *testing.T) {
	big := strings.Repeat("A", maxAttachmentBytes+1)
	specs := []attachmentSpec{{ContentBase64: base64.StdEncoding.EncodeToString([]byte(big))}}
	if _, err := resolveAttachments(context.Background(), specs); err == nil {
		t.Fatal("se esperaba error por tamaño")
	}
}

func TestResolveAttachments_URLTooLarge(t *testing.T) {
	allowPrivateForTest(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(make([]byte, maxAttachmentBytes+10))
	}))
	defer srv.Close()
	if _, err := resolveAttachments(context.Background(), []attachmentSpec{{URL: srv.URL}}); err == nil {
		t.Fatal("se esperaba error por tamaño remoto")
	}
}

// TestResolveAttachments_SSRFBlocked: con el guard activo (por defecto), una URL
// que apunta a loopback debe rechazarse.
func TestResolveAttachments_SSRFBlocked(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("interno"))
	}))
	defer srv.Close()
	// allowPrivateHosts está en false por defecto: el dial a 127.0.0.1 se bloquea.
	if _, err := resolveAttachments(context.Background(), []attachmentSpec{{URL: srv.URL}}); err == nil {
		t.Fatal("se esperaba bloqueo SSRF a loopback")
	}
}

// TestResolveAttachments_RedirectHostAllowlist (L4): con allowlist de host, un
// redirect a un host fuera de ella se bloquea (CheckRedirect), no solo el guard
// por IP. El primer salto (loopback) está permitido por la allowlist.
func TestResolveAttachments_RedirectHostAllowlist(t *testing.T) {
	allowPrivateForTest(t)
	redir := httptest.NewServer(http.RedirectHandler("http://evil.example/payload", http.StatusFound))
	defer redir.Close()

	// La allowlist solo incluye el host del redirector (loopback); el destino del
	// redirect (evil.example) queda fuera.
	withHostAllowlist(t, "127.0.0.1")
	if _, err := resolveAttachments(context.Background(), []attachmentSpec{{URL: redir.URL}}); err == nil {
		t.Fatal("el redirect a un host fuera de la allowlist debería bloquearse")
	}
}

func TestIsBlockedIP(t *testing.T) {
	blocked := []string{"127.0.0.1", "10.1.2.3", "192.168.0.1", "172.16.5.5", "169.254.1.1", "::1", "fc00::1", "0.0.0.0"}
	for _, s := range blocked {
		if !isBlockedIP(net.ParseIP(s)) {
			t.Errorf("%s debería bloquearse", s)
		}
	}
	allowed := []string{"8.8.8.8", "1.1.1.1", "203.0.113.10", "2606:4700:4700::1111"}
	for _, s := range allowed {
		if isBlockedIP(net.ParseIP(s)) {
			t.Errorf("%s NO debería bloquearse", s)
		}
	}
}

// withHostAllowlist fija una allowlist de hosts durante el test y la limpia al final.
func withHostAllowlist(t *testing.T, hosts ...string) {
	t.Helper()
	SetHostAllowlist(hosts)
	t.Cleanup(func() { SetHostAllowlist(nil) })
}

func TestHostAllowlist(t *testing.T) {
	SetHostAllowlist(nil)
	if err := checkHostAllowed("cualquier.host"); err != nil {
		t.Fatalf("sin allowlist no debería filtrar: %v", err)
	}
	withHostAllowlist(t, "cdn.example.com")
	if err := checkHostAllowed("CDN.Example.com"); err != nil {
		t.Errorf("host permitido (case-insensitive) no debería fallar: %v", err)
	}
	if err := checkHostAllowed("evil.example"); err == nil {
		t.Error("host fuera de la allowlist debería rechazarse")
	}
}

// TestResolveAttachments_HostAllowlist: con allowlist configurada, una URL cuyo
// host no está en ella se rechaza; si se añade el host, se permite. Se habilita
// el acceso a loopback para aislar el filtro por host del guard por IP.
func TestResolveAttachments_HostAllowlist(t *testing.T) {
	allowPrivateForTest(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	withHostAllowlist(t, "solo-este.example.com")
	if _, err := resolveAttachments(context.Background(), []attachmentSpec{{URL: srv.URL}}); err == nil {
		t.Fatal("se esperaba rechazo por allowlist de host")
	}

	host, _, _ := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	SetHostAllowlist([]string{host})
	if _, err := resolveAttachments(context.Background(), []attachmentSpec{{URL: srv.URL}}); err != nil {
		t.Fatalf("host en la allowlist debería permitirse: %v", err)
	}
}
