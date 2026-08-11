package http

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"

	"dvtech/qbn/internal/apperr"
	"dvtech/qbn/internal/httpx"
	"dvtech/qbn/internal/store/sqlc"
	"dvtech/qbn/internal/template"
)

// AdminTemplateHandler expone la gestión de templates por aplicación y la
// librería global de BaseTemplate (solo lectura).
type AdminTemplateHandler struct {
	svc *template.Service
}

// NewAdminTemplateHandler crea el handler.
func NewAdminTemplateHandler(svc *template.Service) *AdminTemplateHandler {
	return &AdminTemplateHandler{svc: svc}
}

type templateRequest struct {
	Key            string                           `json:"key"`
	Channel        string                           `json:"channel"`
	Locale         string                           `json:"locale"`
	Name           string                           `json:"name"`
	Description    string                           `json:"description"`
	Subject        string                           `json:"subject"`
	Structure      json.RawMessage                  `json:"structure"`
	BaseTemplateID *string                          `json:"baseTemplateId"`
	Variables      map[string]template.VariableSpec `json:"variables"`
	Kind           string                           `json:"kind"` // "builder" (def.) | "html"
	Body           string                           `json:"body"` // HTML crudo si kind == "html"
}

func (req templateRequest) toInput() (template.Input, error) {
	in := template.Input{
		Key:         req.Key,
		Channel:     req.Channel,
		Locale:      req.Locale,
		Name:        req.Name,
		Description: req.Description,
		Subject:     req.Subject,
		Structure:   req.Structure,
		Variables:   req.Variables,
		Kind:        req.Kind,
		Body:        req.Body,
	}
	if req.BaseTemplateID != nil && *req.BaseTemplateID != "" {
		id, err := uuid.Parse(*req.BaseTemplateID)
		if err != nil {
			return template.Input{}, apperr.Validation("invalid_base_template_id", "Invalid baseTemplateId")
		}
		in.BaseTemplateID = &id
	}
	return in, nil
}

// Create: POST /admin/applications/{appId}/templates
func (h *AdminTemplateHandler) Create(w http.ResponseWriter, r *http.Request) {
	appID, ok := appIDParam(w, r)
	if !ok {
		return
	}
	var req templateRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	if req.Key == "" || req.Name == "" {
		httpx.WriteProblem(w, r, apperr.Validation("missing_fields", "key and name are required"))
		return
	}
	in, err := req.toInput()
	if err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	t, err := h.svc.Create(r.Context(), adminActor(r), appID, in)
	if err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toTemplateView(t))
}

// Update: PATCH /admin/applications/{appId}/templates/{id} → nueva versión.
func (h *AdminTemplateHandler) Update(w http.ResponseWriter, r *http.Request) {
	appID, ok := appIDParam(w, r)
	if !ok {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var req templateRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	in, err := req.toInput()
	if err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	t, err := h.svc.Update(r.Context(), adminActor(r), appID, id, in)
	if err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toTemplateView(t))
}

// Get: GET /admin/applications/{appId}/templates/{id}
func (h *AdminTemplateHandler) Get(w http.ResponseWriter, r *http.Request) {
	appID, ok := appIDParam(w, r)
	if !ok {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	t, err := h.svc.Get(r.Context(), appID, id)
	if err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toTemplateView(t))
}

// List: GET /admin/applications/{appId}/templates
func (h *AdminTemplateHandler) List(w http.ResponseWriter, r *http.Request) {
	appID, ok := appIDParam(w, r)
	if !ok {
		return
	}
	ts, err := h.svc.List(r.Context(), appID)
	if err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	// Filtro opcional por canal (?channel=EMAIL|PUSH|SMS|IN_APP) para las tabs y
	// el select de "Tipos de notificación".
	channel := r.URL.Query().Get("channel")
	views := make([]templateView, 0, len(ts))
	for _, t := range ts {
		if channel != "" && string(t.Channel) != channel {
			continue
		}
		views = append(views, toTemplateView(t))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": views})
}

// Delete: DELETE /admin/applications/{appId}/templates/{id} — elimina la
// plantilla completa (todas sus versiones).
func (h *AdminTemplateHandler) Delete(w http.ResponseWriter, r *http.Request) {
	appID, ok := appIDParam(w, r)
	if !ok {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := h.svc.Delete(r.Context(), adminActor(r), appID, id); err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type previewRequest struct {
	Payload map[string]any `json:"payload"`
}

// Preview: POST /admin/applications/{appId}/templates/{id}/preview
func (h *AdminTemplateHandler) Preview(w http.ResponseWriter, r *http.Request) {
	appID, ok := appIDParam(w, r)
	if !ok {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var req previewRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	res, err := h.svc.Preview(r.Context(), appID, id, req.Payload)
	if err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, res)
}

type previewDraftRequest struct {
	Channel   string                           `json:"channel"`
	Subject   string                           `json:"subject"`
	Structure json.RawMessage                  `json:"structure"`
	Variables map[string]template.VariableSpec `json:"variables"`
	Payload   map[string]any                   `json:"payload"`
	Kind      string                           `json:"kind"` // "builder" (def.) | "html"
	Body      string                           `json:"body"` // HTML crudo si kind == "html"
}

// PreviewDraft: POST /admin/applications/{appId}/templates/preview — renderiza
// una estructura sin guardar (editor en vivo, antes de crear/actualizar).
func (h *AdminTemplateHandler) PreviewDraft(w http.ResponseWriter, r *http.Request) {
	if _, ok := appIDParam(w, r); !ok {
		return
	}
	var req previewDraftRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	res, err := h.svc.PreviewDraft(req.Channel, req.Kind, req.Structure, req.Subject, req.Body, req.Variables, req.Payload)
	if err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, res)
}

// ListBase: GET /admin/base-templates — librería global (solo lectura).
func (h *AdminTemplateHandler) ListBase(w http.ResponseWriter, r *http.Request) {
	bs, err := h.svc.ListBaseTemplates(r.Context())
	if err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	views := make([]baseTemplateView, 0, len(bs))
	for _, b := range bs {
		views = append(views, baseTemplateView{
			ID:          b.ID.String(),
			Key:         b.Key,
			Channel:     string(b.Channel),
			Category:    b.Category,
			Name:        b.Name,
			Description: derefStr(b.Description),
			Kind:        b.Kind,
			Structure:   json.RawMessage(b.Structure),
			Body:        derefStr(b.Body),
			Subject:     derefStr(b.Subject),
		})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": views})
}

// --- views ---

type templateView struct {
	ID                string          `json:"id"`
	Key               string          `json:"key"`
	Channel           string          `json:"channel"`
	Locale            string          `json:"locale"`
	Version           int32           `json:"version"`
	Name              string          `json:"name"`
	Description       string          `json:"description,omitempty"`
	Subject           string          `json:"subject,omitempty"`
	IsActive          bool            `json:"isActive"`
	Kind              string          `json:"kind"`
	Structure         json.RawMessage `json:"structure,omitempty"`
	Body              string          `json:"body,omitempty"` // HTML (útil para editar/revertir en modo html)
	RequiredVariables json.RawMessage `json:"requiredVariables,omitempty"`
	CreatedAt         string          `json:"createdAt"`
}

func toTemplateView(t sqlc.Template) templateView {
	return templateView{
		ID:                t.ID.String(),
		Key:               t.Key,
		Channel:           string(t.Channel),
		Locale:            t.Locale,
		Version:           t.Version,
		Name:              t.Name,
		Description:       derefStr(t.Description),
		Subject:           derefStr(t.Subject),
		IsActive:          t.IsActive,
		Kind:              t.Kind,
		Structure:         json.RawMessage(t.Structure),
		Body:              t.Body,
		RequiredVariables: json.RawMessage(t.RequiredVariables),
		CreatedAt:         t.CreatedAt.Time.Format(time.RFC3339),
	}
}

type baseTemplateView struct {
	ID          string          `json:"id"`
	Key         string          `json:"key"`
	Channel     string          `json:"channel"`
	Category    string          `json:"category"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Kind        string          `json:"kind"`
	Structure   json.RawMessage `json:"structure,omitempty"`
	Body        string          `json:"body,omitempty"`
	Subject     string          `json:"subject,omitempty"`
}

// derefStr local (los *string de sqlc) → string.
func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
