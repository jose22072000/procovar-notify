package apperr

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
)

func TestKindHTTPStatus(t *testing.T) {
	cases := []struct {
		err  *Error
		want int
	}{
		{Validation("c", "d"), http.StatusUnprocessableEntity},
		{NotFound("c", "d"), http.StatusNotFound},
		{Conflict("c", "d"), http.StatusConflict},
		{Unauthorized("c", "d"), http.StatusUnauthorized},
		{Forbidden("c", "d"), http.StatusForbidden},
		{RateLimited("c", "d"), http.StatusTooManyRequests},
		{PayloadTooLarge("c", "d"), http.StatusRequestEntityTooLarge},
		{Internal(errors.New("x")), http.StatusInternalServerError},
	}
	for _, c := range cases {
		if got := c.err.HTTPStatus(); got != c.want {
			t.Errorf("code %q: HTTPStatus = %d, want %d", c.err.Code, got, c.want)
		}
		if c.err.Title() == "" {
			t.Errorf("code %q: Title vacío", c.err.Code)
		}
	}
}

func TestFromPassthroughAndWrap(t *testing.T) {
	if From(nil) != nil {
		t.Error("From(nil) debería ser nil")
	}
	// Un *Error se devuelve tal cual (aunque esté envuelto).
	orig := Validation("missing_fields", "x")
	wrapped := fmt.Errorf("contexto: %w", orig)
	if got := From(wrapped); got != orig {
		t.Errorf("From debería desenvolver al *Error original, got %+v", got)
	}
	// Un error cualquiera se convierte en interno (500) sin filtrar la causa.
	generic := From(errors.New("pg: down"))
	if generic.HTTPStatus() != http.StatusInternalServerError {
		t.Errorf("un error genérico debería ser 500, got %d", generic.HTTPStatus())
	}
}

func TestErrorUnwrap(t *testing.T) {
	cause := errors.New("causa raíz")
	e := Wrap(cause, KindInternal, "internal_error", "algo")
	if !errors.Is(e, cause) {
		t.Error("errors.Is debería encontrar la causa envuelta")
	}
}
