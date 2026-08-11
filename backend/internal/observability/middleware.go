package observability

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// HeaderRequestID es la cabecera donde se lee/propaga el id de correlación.
const HeaderRequestID = "X-Request-Id"

// RequestID es un middleware que asegura un id de correlación por petición:
// reutiliza el de la cabecera entrante si viene, o genera uno nuevo. Lo deja en
// el context, lo devuelve en la cabecera de respuesta y adjunta un logger ya
// enriquecido con ese id (y método/ruta) para las capas siguientes.
func RequestID(base *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := r.Header.Get(HeaderRequestID)
			if id == "" {
				id = uuid.NewString()
			}

			ctx := r.Context()
			ctx = ContextWithRequestID(ctx, id)
			reqLogger := base.With(
				slog.String("request_id", id),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
			)
			ctx = ContextWithLogger(ctx, reqLogger)

			w.Header().Set(HeaderRequestID, id)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// statusRecorder captura el status escrito para poder loggearlo al terminar.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (sr *statusRecorder) WriteHeader(code int) {
	sr.status = code
	sr.ResponseWriter.WriteHeader(code)
}

// Flush delega en el ResponseWriter subyacente para no romper streaming (SSE).
func (sr *statusRecorder) Flush() {
	if f, ok := sr.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// AccessLog registra una línea estructurada por petición con status y duración.
func AccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sr := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(sr, r)

		LoggerFromContext(r.Context()).Info("request completed",
			slog.Int("status", sr.status),
			slog.Duration("duration", time.Since(start)),
			slog.String("remote_addr", r.RemoteAddr),
		)
	})
}
