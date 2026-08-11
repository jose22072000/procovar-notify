//go:build manualnet

package channels

import (
	"context"
	"testing"
	"time"
)

// Prueba manual (requiere red): verifica los dos rituales de cifrado contra un
// servidor real. Ejecutar con: go test -tags manualnet -run TestDialTLSModes ./internal/channels
func TestDialTLSModes(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// SSL implícito (465)
	if err := DialTest(ctx, SMTPConfig{Host: "smtp.gmail.com", Port: 465, Secure: true}); err != nil {
		t.Errorf("SSL implícito 465: %v", err)
	}
	// STARTTLS (587)
	if err := DialTest(ctx, SMTPConfig{Host: "smtp.gmail.com", Port: 587, Secure: true}); err != nil {
		t.Errorf("STARTTLS 587: %v", err)
	}
}
