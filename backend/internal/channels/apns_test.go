package channels

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// testP8 genera una clave EC P-256 y la devuelve como PEM PKCS#8 (.p8) junto a
// su clave pública para verificar la firma del JWT.
func testP8(t *testing.T) (string, *ecdsa.PublicKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("MarshalPKCS8: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	return string(pemBytes), &key.PublicKey
}

func TestPushSender_APNS_OK(t *testing.T) {
	pemKey, pub := testP8(t)

	var gotPath, gotAuth, gotTopic, gotType string
	var payload map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("authorization")
		gotTopic = r.Header.Get("apns-topic")
		gotType = r.Header.Get("apns-push-type")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &payload)
		w.Header().Set("apns-id", "APNS-ID-123")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := NewPushSender()
	s.http = srv.Client()
	route := ResolvedRoute{
		Channel: "PUSH",
		Provider: &ProviderConfig{
			Provider: "APNS",
			Config: map[string]any{
				"teamId":     "TEAM123456",
				"keyId":      "KEY1234567",
				"bundleId":   "com.acme.app",
				"privateKey": pemKey,
				"endpoint":   srv.URL,
			},
		},
	}
	msg := RenderedMessage{
		Subject:   "Hola",
		HTMLBody:  "<p>Cuerpo push</p>",
		Recipient: Recipient{PushToken: "devtoken99"},
	}

	ref, err := s.Send(context.Background(), msg, route)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if ref != "APNS-ID-123" {
		t.Errorf("ProviderRef = %q, quería APNS-ID-123", ref)
	}
	if gotPath != "/3/device/devtoken99" {
		t.Errorf("path = %q", gotPath)
	}
	if gotTopic != "com.acme.app" || gotType != "alert" {
		t.Errorf("apns-topic=%q apns-push-type=%q", gotTopic, gotType)
	}
	aps, _ := payload["aps"].(map[string]any)
	alert, _ := aps["alert"].(map[string]any)
	if alert["title"] != "Hola" || alert["body"] != "Cuerpo push" {
		t.Errorf("aps.alert = %v", alert)
	}

	// El JWT debe ser ES256 con kid=keyId, iss=teamId y firma válida.
	tok := strings.TrimPrefix(gotAuth, "bearer ")
	verifyAPNSJWT(t, tok, pub, "TEAM123456", "KEY1234567")
}

func verifyAPNSJWT(t *testing.T, tok string, pub *ecdsa.PublicKey, teamID, keyID string) {
	t.Helper()
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("JWT mal formado: %q", tok)
	}
	hdrRaw, _ := base64.RawURLEncoding.DecodeString(parts[0])
	clRaw, _ := base64.RawURLEncoding.DecodeString(parts[1])
	sig, _ := base64.RawURLEncoding.DecodeString(parts[2])

	var hdr struct{ Alg, Kid string }
	_ = json.Unmarshal(hdrRaw, &hdr)
	if hdr.Alg != "ES256" || hdr.Kid != keyID {
		t.Errorf("header JWT = %+v", hdr)
	}
	var cl struct {
		Iss string `json:"iss"`
		Iat int64  `json:"iat"`
	}
	_ = json.Unmarshal(clRaw, &cl)
	if cl.Iss != teamID {
		t.Errorf("iss = %q, quería %q", cl.Iss, teamID)
	}
	if cl.Iat == 0 || cl.Iat > time.Now().Unix()+5 {
		t.Errorf("iat fuera de rango: %d", cl.Iat)
	}
	if len(sig) != 64 {
		t.Fatalf("firma ES256 debe ser 64 bytes, got %d", len(sig))
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	r := new(big.Int).SetBytes(sig[:32])
	sgn := new(big.Int).SetBytes(sig[32:])
	if !ecdsa.Verify(pub, digest[:], r, sgn) {
		t.Error("firma JWT inválida")
	}
}

func TestTokenCache(t *testing.T) {
	c := newTokenCache()
	base := time.Unix(1_700_000_000, 0)
	if _, ok := c.get("k", base); ok {
		t.Fatal("caché vacía no debería dar hit")
	}
	c.set("k", "tok-1", base.Add(50*time.Minute))
	if v, ok := c.get("k", base.Add(10*time.Minute)); !ok || v != "tok-1" {
		t.Fatalf("debería dar hit dentro de vigencia: %q %v", v, ok)
	}
	if _, ok := c.get("k", base.Add(51*time.Minute)); ok {
		t.Fatal("no debería dar hit tras expirar")
	}
}

// TestPushSender_APNS_CachesToken: dos envíos seguidos reutilizan el mismo JWT
// (un único entry en la caché y misma cabecera Authorization).
func TestPushSender_APNS_CachesToken(t *testing.T) {
	pemKey, _ := testP8(t)
	var auths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auths = append(auths, r.Header.Get("authorization"))
		w.Header().Set("apns-id", "id")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := NewPushSender()
	s.http = srv.Client()
	route := fcmRouteAPNS(pemKey, srv.URL)
	msg := RenderedMessage{Subject: "x", Recipient: Recipient{PushToken: "t"}}

	for i := 0; i < 2; i++ {
		if _, err := s.Send(context.Background(), msg, route); err != nil {
			t.Fatalf("Send #%d: %v", i, err)
		}
	}
	if s.apns.len() != 1 {
		t.Errorf("la caché debería tener 1 entry, got %d", s.apns.len())
	}
	if len(auths) != 2 || auths[0] != auths[1] {
		t.Errorf("ambos envíos deberían reusar el mismo token: %v", auths)
	}
}

func fcmRouteAPNS(pemKey, endpoint string) ResolvedRoute {
	return ResolvedRoute{Channel: "PUSH", Provider: &ProviderConfig{Provider: "APNS", Config: map[string]any{
		"teamId": "TEAM", "keyId": "KEY", "bundleId": "b", "privateKey": pemKey, "endpoint": endpoint,
	}}}
}

func TestPushSender_APNS_Rejected(t *testing.T) {
	pemKey, _ := testP8(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"reason":"BadDeviceToken"}`))
	}))
	defer srv.Close()

	s := NewPushSender()
	s.http = srv.Client()
	route := ResolvedRoute{Channel: "PUSH", Provider: &ProviderConfig{Provider: "APNS", Config: map[string]any{
		"teamId": "T", "keyId": "K", "bundleId": "b", "privateKey": pemKey, "endpoint": srv.URL,
	}}}
	msg := RenderedMessage{Subject: "x", Recipient: Recipient{PushToken: "t"}}

	if _, err := s.Send(context.Background(), msg, route); err == nil {
		t.Fatal("se esperaba error de APNs")
	}
}

func TestPushSender_APNS_MissingConfig(t *testing.T) {
	s := NewPushSender()
	route := ResolvedRoute{Channel: "PUSH", Provider: &ProviderConfig{Provider: "APNS", Config: map[string]any{"teamId": "T"}}}
	msg := RenderedMessage{Recipient: Recipient{PushToken: "t"}}
	if _, err := s.Send(context.Background(), msg, route); err == nil {
		t.Fatal("se esperaba error por config incompleta")
	}
}

func TestPushSender_APNS_BadKey(t *testing.T) {
	s := NewPushSender()
	route := ResolvedRoute{Channel: "PUSH", Provider: &ProviderConfig{Provider: "APNS", Config: map[string]any{
		"teamId": "T", "keyId": "K", "bundleId": "b", "privateKey": "no-es-pem",
	}}}
	msg := RenderedMessage{Recipient: Recipient{PushToken: "t"}}
	if _, err := s.Send(context.Background(), msg, route); err == nil {
		t.Fatal("se esperaba error por clave inválida")
	}
}
