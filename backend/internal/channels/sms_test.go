package channels

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func twilioRoute(baseURL string) ResolvedRoute {
	return ResolvedRoute{
		Channel: "SMS",
		Provider: &ProviderConfig{
			Provider: "TWILIO",
			Config: map[string]any{
				"accountSid": "AC123",
				"authToken":  "tok-secret",
				"from":       "+15550001111",
				"baseUrl":    baseURL,
			},
		},
	}
}

func TestSMSSender_Twilio_OK(t *testing.T) {
	var gotPath, gotTo, gotFrom, gotBody, gotUser, gotPass string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotUser, gotPass, _ = r.BasicAuth()
		_ = r.ParseForm()
		gotTo = r.PostForm.Get("To")
		gotFrom = r.PostForm.Get("From")
		gotBody = r.PostForm.Get("Body")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"sid":"SM999","status":"queued"}`))
	}))
	defer srv.Close()

	s := NewSMSSender()
	s.http = srv.Client()
	msg := RenderedMessage{
		Subject:   "Hola",
		HTMLBody:  "<p>Tu código es <b>1234</b></p>",
		Recipient: Recipient{Phone: "+15557778888"},
	}

	ref, err := s.Send(context.Background(), msg, twilioRoute(srv.URL))
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if ref != "SM999" {
		t.Errorf("ProviderRef = %q, quería SM999", ref)
	}
	if gotPath != "/2010-04-01/Accounts/AC123/Messages.json" {
		t.Errorf("path = %q", gotPath)
	}
	if gotUser != "AC123" || gotPass != "tok-secret" {
		t.Errorf("basic auth = %q/%q", gotUser, gotPass)
	}
	if gotTo != "+15557778888" || gotFrom != "+15550001111" {
		t.Errorf("To/From = %q/%q", gotTo, gotFrom)
	}
	if gotBody != "Tu código es 1234" {
		t.Errorf("Body = %q", gotBody)
	}
}

func TestSMSSender_Twilio_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"code":21211,"message":"Invalid 'To' Phone Number"}`))
	}))
	defer srv.Close()

	s := NewSMSSender()
	s.http = srv.Client()
	msg := RenderedMessage{Subject: "x", Recipient: Recipient{Phone: "+1"}}

	if _, err := s.Send(context.Background(), msg, twilioRoute(srv.URL)); err == nil {
		t.Fatal("se esperaba error de la API")
	}
}

func TestSMSSender_MissingPhone(t *testing.T) {
	s := NewSMSSender()
	_, err := s.Send(context.Background(), RenderedMessage{}, twilioRoute("http://x"))
	if err == nil {
		t.Fatal("se esperaba error por teléfono ausente")
	}
}

func TestSMSSender_UnsupportedProvider(t *testing.T) {
	s := NewSMSSender()
	route := ResolvedRoute{Channel: "SMS", Provider: &ProviderConfig{Provider: "NEXMO"}}
	msg := RenderedMessage{Recipient: Recipient{Phone: "+1"}}
	if _, err := s.Send(context.Background(), msg, route); err == nil {
		t.Fatal("se esperaba error por proveedor no soportado")
	}
}

func TestSMSSender_NoProvider(t *testing.T) {
	s := NewSMSSender()
	msg := RenderedMessage{Recipient: Recipient{Phone: "+1"}}
	if _, err := s.Send(context.Background(), msg, ResolvedRoute{Channel: "SMS"}); err == nil {
		t.Fatal("se esperaba error por ruta sin proveedor")
	}
}
