// Package apperr define el modelo de error central del servicio.
//
// Toda la aplicación produce errores tipados (a través de los constructores de
// este paquete) y un único middleware HTTP los traduce a respuestas
// application/problem+json (RFC 7807). Las capas internas NUNCA exponen errores
// del driver (pgx, redis...) directamente al cliente: se mapean a un Error de
// este paquete con un Kind estable.
package apperr

import (
	"errors"
	"fmt"
	"net/http"
)

// Kind clasifica el error y determina el status HTTP por defecto.
type Kind int

const (
	// KindInternal es el fallback: algo falló de forma inesperada (500).
	KindInternal Kind = iota
	// KindValidation indica entrada inválida del cliente (422).
	KindValidation
	// KindNotFound indica que el recurso solicitado no existe (404).
	KindNotFound
	// KindConflict indica un choque de estado, p. ej. unicidad/idempotencia (409).
	KindConflict
	// KindUnauthorized indica falta o invalidez de credenciales (401).
	KindUnauthorized
	// KindForbidden indica credenciales válidas sin permiso suficiente (403).
	KindForbidden
	// KindRateLimited indica que se superó el límite de peticiones (429).
	KindRateLimited
	// KindPayloadTooLarge indica un cuerpo de petición demasiado grande (413).
	KindPayloadTooLarge
)

// httpStatus mapea cada Kind a su status HTTP por defecto.
func (k Kind) httpStatus() int {
	switch k {
	case KindValidation:
		return http.StatusUnprocessableEntity
	case KindNotFound:
		return http.StatusNotFound
	case KindConflict:
		return http.StatusConflict
	case KindUnauthorized:
		return http.StatusUnauthorized
	case KindForbidden:
		return http.StatusForbidden
	case KindRateLimited:
		return http.StatusTooManyRequests
	case KindPayloadTooLarge:
		return http.StatusRequestEntityTooLarge
	default:
		return http.StatusInternalServerError
	}
}

// title devuelve un título legible y estable por Kind (campo "title" de RFC 7807).
func (k Kind) title() string {
	switch k {
	case KindValidation:
		return "Validation failed"
	case KindNotFound:
		return "Resource not found"
	case KindConflict:
		return "Conflict"
	case KindUnauthorized:
		return "Unauthorized"
	case KindForbidden:
		return "Forbidden"
	case KindRateLimited:
		return "Too many requests"
	case KindPayloadTooLarge:
		return "Payload too large"
	default:
		return "Internal server error"
	}
}

// Error es el error tipado de la aplicación.
//
// Code es un identificador de máquina estable (p. ej. "notification_not_found")
// pensado para que los clientes lo discriminen sin parsear texto. Detail es el
// mensaje legible para humanos. Err es la causa envuelta (nunca se expone al
// cliente; se usa para logs/correlación).
type Error struct {
	Kind   Kind
	Code   string
	Detail string
	Err    error
}

func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Detail, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Detail)
}

// Unwrap permite usar errors.Is / errors.As contra la causa envuelta.
func (e *Error) Unwrap() error { return e.Err }

// HTTPStatus devuelve el status HTTP que corresponde al Kind del error.
func (e *Error) HTTPStatus() int { return e.Kind.httpStatus() }

// Title devuelve el título RFC 7807 asociado al Kind.
func (e *Error) Title() string { return e.Kind.title() }

// New crea un Error tipado sin causa envuelta.
func New(kind Kind, code, detail string) *Error {
	return &Error{Kind: kind, Code: code, Detail: detail}
}

// Wrap crea un Error tipado envolviendo una causa subyacente.
func Wrap(err error, kind Kind, code, detail string) *Error {
	return &Error{Kind: kind, Code: code, Detail: detail, Err: err}
}

// Constructores de conveniencia para los casos más comunes.

func NotFound(code, detail string) *Error   { return New(KindNotFound, code, detail) }
func Validation(code, detail string) *Error { return New(KindValidation, code, detail) }
func Conflict(code, detail string) *Error   { return New(KindConflict, code, detail) }
func Unauthorized(code, detail string) *Error {
	return New(KindUnauthorized, code, detail)
}
func Forbidden(code, detail string) *Error   { return New(KindForbidden, code, detail) }
func RateLimited(code, detail string) *Error { return New(KindRateLimited, code, detail) }
func PayloadTooLarge(code, detail string) *Error {
	return New(KindPayloadTooLarge, code, detail)
}
func Internal(err error) *Error {
	return Wrap(err, KindInternal, "internal_error", "An unexpected error occurred")
}

// From normaliza cualquier error a un *Error. Si ya es un *Error lo devuelve tal
// cual; en caso contrario lo envuelve como KindInternal (sin filtrar el detalle
// real al cliente).
func From(err error) *Error {
	if err == nil {
		return nil
	}
	var ae *Error
	if errors.As(err, &ae) {
		return ae
	}
	return Internal(err)
}
