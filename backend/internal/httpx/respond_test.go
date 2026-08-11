package httpx

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"dvtech/qbn/internal/apperr"
)

func decodeProblem(t *testing.T, rec *httptest.ResponseRecorder) (status int, code, detail string) {
	t.Helper()
	var p struct {
		Status int    `json:"status"`
		Code   string `json:"code"`
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("respuesta no es JSON: %v (%s)", err, rec.Body)
	}
	return p.Status, p.Code, p.Detail
}

func TestWriteProblem_AppError(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteProblem(rec, httptest.NewRequest("GET", "/", nil), apperr.Validation("missing_fields", "name is required"))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("content-type = %q", ct)
	}
	status, code, detail := decodeProblem(t, rec)
	if status != 422 || code != "missing_fields" || detail != "name is required" {
		t.Fatalf("cuerpo inesperado: status=%d code=%q detail=%q", status, code, detail)
	}
}

func TestWriteProblem_InternalHidesDetail(t *testing.T) {
	rec := httptest.NewRecorder()
	// Un error interno no debe filtrar su causa real al cliente.
	WriteProblem(rec, httptest.NewRequest("GET", "/", nil), errors.New("pg: connection refused at 10.0.0.5"))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	status, _, detail := decodeProblem(t, rec)
	if status != 500 {
		t.Fatalf("status en cuerpo = %d", status)
	}
	if detail == "" || detail != "An unexpected error occurred" {
		t.Fatalf("el detalle no debería filtrar la causa: %q", detail)
	}
}
