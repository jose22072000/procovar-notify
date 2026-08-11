// Package httpx contiene utilidades HTTP de bajo nivel compartidas por la capa
// http y por los middlewares (auth, ratelimit...): serialización JSON de
// respuestas y renderizado de errores como application/problem+json (RFC 7807).
//
// Vive en su propio paquete (no en internal/http) para que tanto los handlers
// como los middlewares puedan usarlo sin crear ciclos de importación.
package httpx

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"dvtech/qbn/internal/apperr"
	"dvtech/qbn/internal/observability"
)

const problemContentType = "application/problem+json"

// problemResponse es el cuerpo de error (RFC 7807 + code de máquina + traceId).
type problemResponse struct {
	Type    string `json:"type"`
	Title   string `json:"title"`
	Status  int    `json:"status"`
	Detail  string `json:"detail"`
	Code    string `json:"code"`
	TraceID string `json:"traceId,omitempty"`
}

// WriteProblem traduce cualquier error a una respuesta application/problem+json.
// Los errores internos se loggean con su causa real pero al cliente solo se le
// devuelve un mensaje genérico, para no filtrar detalles de infraestructura.
func WriteProblem(w http.ResponseWriter, r *http.Request, err error) {
	ae := apperr.From(err)
	traceID := observability.RequestIDFromContext(r.Context())
	logger := observability.LoggerFromContext(r.Context())

	if ae.Kind == apperr.KindInternal {
		logger.Error("request failed", slog.String("code", ae.Code), slog.Any("error", ae.Err))
	} else {
		logger.Warn("request rejected",
			slog.String("code", ae.Code),
			slog.Int("status", ae.HTTPStatus()),
			slog.String("detail", ae.Detail),
		)
	}

	body := problemResponse{
		Type:    "about:blank",
		Title:   ae.Title(),
		Status:  ae.HTTPStatus(),
		Detail:  clientDetail(ae),
		Code:    ae.Code,
		TraceID: traceID,
	}

	w.Header().Set("Content-Type", problemContentType)
	w.WriteHeader(body.Status)
	if encErr := json.NewEncoder(w).Encode(body); encErr != nil {
		logger.Error("failed to encode problem response", slog.Any("error", encErr))
	}
}

func clientDetail(ae *apperr.Error) string {
	if ae.Kind == apperr.KindInternal {
		return "An unexpected error occurred"
	}
	return ae.Detail
}

// WriteJSON serializa v como JSON con el status indicado.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}
