package auth

import (
	"net/url"
	"testing"
)

func TestSignAndVerify(t *testing.T) {
	secret := []byte("secreto-de-la-api-key")
	method, path, query := "POST", "/v1/notifications", "a=1&b=2"
	body := []byte(`{"templateKey":"welcome"}`)
	ts := "1700000000"

	sig := Sign(secret, method, path, query, body, ts)
	if sig == "" {
		t.Fatal("la firma no debería estar vacía")
	}
	if !verifySignature(secret, method, path, query, body, ts, sig) {
		t.Fatal("la firma correcta debería verificar")
	}
}

func TestVerifyRejectsTampering(t *testing.T) {
	secret := []byte("secreto")
	body := []byte("cuerpo")
	sig := Sign(secret, "GET", "/v1/x", "q=1", body, "100")

	cases := []struct {
		name                               string
		secret                             []byte
		method, path, query, ts, signature string
		body                               []byte
	}{
		{"secret distinto", []byte("otro"), "GET", "/v1/x", "q=1", "100", sig, body},
		{"método distinto", secret, "POST", "/v1/x", "q=1", "100", sig, body},
		{"path distinto", secret, "GET", "/v1/y", "q=1", "100", sig, body},
		{"query distinta", secret, "GET", "/v1/x", "q=2", "100", sig, body},
		{"query ausente", secret, "GET", "/v1/x", "", "100", sig, body},
		{"timestamp distinto", secret, "GET", "/v1/x", "q=1", "101", sig, body},
		{"body distinto", secret, "GET", "/v1/x", "q=1", "100", sig, []byte("otro")},
		{"firma basura", secret, "GET", "/v1/x", "q=1", "100", "deadbeef", body},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if verifySignature(c.secret, c.method, c.path, c.query, c.body, c.ts, c.signature) {
				t.Fatal("no debería verificar")
			}
		})
	}
}

func TestStringToSignDeterministic(t *testing.T) {
	a := StringToSign("GET", "/x", "q=1", []byte("b"), "1")
	b := StringToSign("GET", "/x", "q=1", []byte("b"), "1")
	if a != b {
		t.Fatal("StringToSign debería ser determinista")
	}
}

// TestCanonicalQueryOrderIndependent verifica que el orden de los parámetros no
// cambia la cadena canónica (claves ordenadas), de modo que cliente y servidor
// firman lo mismo.
func TestCanonicalQueryOrderIndependent(t *testing.T) {
	u1, _ := url.Parse("/x?b=2&a=1")
	u2, _ := url.Parse("/x?a=1&b=2")
	if CanonicalQuery(u1) != CanonicalQuery(u2) {
		t.Fatalf("la query canónica debería ser independiente del orden: %q vs %q", CanonicalQuery(u1), CanonicalQuery(u2))
	}
}
