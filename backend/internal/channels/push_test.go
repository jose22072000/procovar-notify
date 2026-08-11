package channels

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func fcmRoute(endpoint string) ResolvedRoute {
	return ResolvedRoute{
		Channel: "PUSH",
		Provider: &ProviderConfig{
			Provider: "FCM",
			Config:   map[string]any{"serverKey": "SK-secret", "endpoint": endpoint},
		},
	}
}

func TestPushSender_FCM_OK(t *testing.T) {
	var gotAuth string
	var payload map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &payload)
		_, _ = w.Write([]byte(`{"multicast_id":42,"success":1,"failure":0,"results":[{"message_id":"0:abc"}]}`))
	}))
	defer srv.Close()

	s := NewPushSender()
	s.http = srv.Client()
	msg := RenderedMessage{
		Subject:   "Título",
		HTMLBody:  "<p>Cuerpo</p>",
		Recipient: Recipient{PushToken: "device-token-xyz"},
	}

	ref, err := s.Send(context.Background(), msg, fcmRoute(srv.URL))
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if ref != "0:abc" {
		t.Errorf("ProviderRef = %q, quería 0:abc", ref)
	}
	if gotAuth != "key=SK-secret" {
		t.Errorf("Authorization = %q", gotAuth)
	}
	if payload["to"] != "device-token-xyz" {
		t.Errorf("to = %v", payload["to"])
	}
	notif, _ := payload["notification"].(map[string]any)
	if notif["title"] != "Título" || notif["body"] != "Cuerpo" {
		t.Errorf("notification = %v", notif)
	}
}

func TestPushSender_FCM_Rejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"multicast_id":1,"success":0,"failure":1,"results":[{"error":"NotRegistered"}]}`))
	}))
	defer srv.Close()

	s := NewPushSender()
	s.http = srv.Client()
	msg := RenderedMessage{Subject: "x", Recipient: Recipient{PushToken: "t"}}

	if _, err := s.Send(context.Background(), msg, fcmRoute(srv.URL)); err == nil {
		t.Fatal("se esperaba error por token rechazado")
	}
}

func TestPushSender_MissingToken(t *testing.T) {
	s := NewPushSender()
	if _, err := s.Send(context.Background(), RenderedMessage{}, fcmRoute("http://x")); err == nil {
		t.Fatal("se esperaba error por push token ausente")
	}
}

func TestPushSender_MissingServerKey(t *testing.T) {
	s := NewPushSender()
	route := ResolvedRoute{Channel: "PUSH", Provider: &ProviderConfig{Provider: "FCM", Config: map[string]any{}}}
	msg := RenderedMessage{Recipient: Recipient{PushToken: "t"}}
	if _, err := s.Send(context.Background(), msg, route); err == nil {
		t.Fatal("se esperaba error por serverKey ausente")
	}
}
