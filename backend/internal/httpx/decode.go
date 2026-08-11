package httpx

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"dvtech/qbn/internal/apperr"
)

// DecodeJSON deserializa el cuerpo de la petición en v, rechazando campos
// desconocidos. Devuelve un apperr.Validation si el JSON es inválido.
func DecodeJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		if errors.Is(err, io.EOF) {
			return apperr.Validation("empty_body", "Request body is required")
		}
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return apperr.PayloadTooLarge("body_too_large", "Request body too large")
		}
		return apperr.Validation("invalid_json", "Invalid request body: "+err.Error())
	}
	return nil
}
