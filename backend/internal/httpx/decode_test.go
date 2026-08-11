package httpx

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"dvtech/qbn/internal/apperr"
)

func decodeKind(t *testing.T, r *http.Request) apperr.Kind {
	t.Helper()
	var v map[string]any
	err := DecodeJSON(r, &v)
	var ae *apperr.Error
	if !errors.As(err, &ae) {
		t.Fatalf("se esperaba *apperr.Error, got %T: %v", err, err)
	}
	return ae.Kind
}

func TestDecodeJSON_BodyTooLarge(t *testing.T) {
	big := `{"a":"` + strings.Repeat("x", 2000) + `"}`
	r := httptest.NewRequest("POST", "/", strings.NewReader(big))
	r.Body = http.MaxBytesReader(httptest.NewRecorder(), r.Body, 100) // límite 100 bytes
	if k := decodeKind(t, r); k != apperr.KindPayloadTooLarge {
		t.Fatalf("se esperaba KindPayloadTooLarge (413), got %v", k)
	}
}

func TestDecodeJSON_EmptyAndInvalid(t *testing.T) {
	if k := decodeKind(t, httptest.NewRequest("POST", "/", strings.NewReader(""))); k != apperr.KindValidation {
		t.Errorf("body vacío debería ser Validation, got %v", k)
	}
	if k := decodeKind(t, httptest.NewRequest("POST", "/", strings.NewReader("{not json"))); k != apperr.KindValidation {
		t.Errorf("JSON inválido debería ser Validation, got %v", k)
	}
	// Campo desconocido en un struct (DisallowUnknownFields) → Validation.
	r := httptest.NewRequest("POST", "/", strings.NewReader(`{"unknown":1}`))
	var s struct {
		Known string `json:"known"`
	}
	err := DecodeJSON(r, &s)
	var ae *apperr.Error
	if !errors.As(err, &ae) || ae.Kind != apperr.KindValidation {
		t.Errorf("campo desconocido debería ser Validation, got %v", err)
	}
}
