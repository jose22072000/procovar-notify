package observability

import (
	"context"
	"log/slog"
	"testing"
)

func TestLoggerFromContext(t *testing.T) {
	// Sin logger en contexto → el default (no nil, no panic).
	if LoggerFromContext(context.Background()) == nil {
		t.Fatal("LoggerFromContext debería devolver el default, no nil")
	}
	// Roundtrip.
	l := slog.Default().With("k", "v")
	ctx := ContextWithLogger(context.Background(), l)
	if LoggerFromContext(ctx) != l {
		t.Error("LoggerFromContext debería devolver el logger inyectado")
	}
}

func TestRequestIDFromContext(t *testing.T) {
	if RequestIDFromContext(context.Background()) != "" {
		t.Error("sin request id → cadena vacía")
	}
	ctx := ContextWithRequestID(context.Background(), "req-123")
	if RequestIDFromContext(ctx) != "req-123" {
		t.Error("RequestIDFromContext debería devolver el id inyectado")
	}
}
