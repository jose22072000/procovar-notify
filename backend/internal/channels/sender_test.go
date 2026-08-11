package channels

import (
	"context"
	"errors"
	"testing"
)

// stubSender es un Sender de prueba con canal y respuesta configurables.
type stubSender struct {
	channel string
	ref     ProviderRef
	err     error
	calls   int
}

func (s *stubSender) Channel() string { return s.channel }
func (s *stubSender) Send(context.Context, RenderedMessage, ResolvedRoute) (ProviderRef, error) {
	s.calls++
	return s.ref, s.err
}

func TestDispatcherRoutesByChannel(t *testing.T) {
	email := &stubSender{channel: "EMAIL", ref: "email-ref"}
	push := &stubSender{channel: "PUSH", ref: "push-ref"}
	d := NewDispatcher(email, push)

	ref, err := d.Send(context.Background(), "PUSH", RenderedMessage{}, ResolvedRoute{})
	if err != nil {
		t.Fatalf("send PUSH: %v", err)
	}
	if ref != "push-ref" {
		t.Fatalf("ref = %q, want push-ref", ref)
	}
	if push.calls != 1 || email.calls != 0 {
		t.Fatalf("debería haberse invocado solo el sender PUSH (push=%d email=%d)", push.calls, email.calls)
	}
}

func TestDispatcherUnknownChannel(t *testing.T) {
	d := NewDispatcher(&stubSender{channel: "EMAIL"})
	if _, err := d.Send(context.Background(), "SMS", RenderedMessage{}, ResolvedRoute{}); err == nil {
		t.Fatal("un canal sin sender registrado debería dar error")
	}
}

func TestDispatcherPropagatesSenderError(t *testing.T) {
	boom := errors.New("boom")
	d := NewDispatcher(&stubSender{channel: "EMAIL", err: boom})
	if _, err := d.Send(context.Background(), "EMAIL", RenderedMessage{}, ResolvedRoute{}); !errors.Is(err, boom) {
		t.Fatalf("debería propagar el error del sender, got %v", err)
	}
}

// TestInAppSenderNoop: el sender in-app no tiene proveedor externo; confirma la
// entrega devolviendo la ref "inapp" (la fila ya es la bandeja).
func TestInAppSenderNoop(t *testing.T) {
	s := NewInAppSender()
	ref, err := s.Send(context.Background(), RenderedMessage{}, ResolvedRoute{})
	if err != nil {
		t.Fatalf("inapp send: %v", err)
	}
	if ref != "inapp" {
		t.Fatalf("ref = %q, want inapp", ref)
	}
	if s.Channel() != "IN_APP" {
		t.Fatalf("canal = %q, want IN_APP", s.Channel())
	}
}
