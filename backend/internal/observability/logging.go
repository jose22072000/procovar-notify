// Package observability concentra el logging estructurado y la correlación de
// peticiones. El mismo identificador de correlación (request id) aparece en los
// logs y se devuelve como traceId en las respuestas de error, de modo que un
// fallo en producción se puede rastrear de punta a punta.
package observability

import (
	"context"
	"log/slog"
	"os"
)

// contextKey es un tipo privado para evitar colisiones de claves en context.
type contextKey int

const (
	loggerKey contextKey = iota
	requestIDKey
)

// NewLogger construye el logger raíz. En producción emite JSON; en desarrollo
// usa texto legible. El nivel se controla con el parámetro debug.
func NewLogger(production, debug bool) *slog.Logger {
	level := slog.LevelInfo
	if debug {
		level = slog.LevelDebug
	}
	opts := &slog.HandlerOptions{Level: level}

	var handler slog.Handler
	if production {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}
	return slog.New(handler)
}

// ContextWithLogger guarda un logger en el context para que las capas inferiores
// lo recuperen ya enriquecido con los campos de correlación.
func ContextWithLogger(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, l)
}

// LoggerFromContext recupera el logger del context. Si no hay ninguno devuelve
// el logger por defecto del proceso, de forma que nunca retorna nil.
func LoggerFromContext(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(loggerKey).(*slog.Logger); ok {
		return l
	}
	return slog.Default()
}

// ContextWithRequestID guarda el id de correlación en el context.
func ContextWithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

// RequestIDFromContext recupera el id de correlación, o "" si no existe.
func RequestIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey).(string); ok {
		return id
	}
	return ""
}
