package http

import (
	"net/http"
	"time"

	"dvtech/qbn/internal/httpx"
	"dvtech/qbn/internal/store/sqlc"
)

type createWebhookRequest struct {
	URL    string   `json:"url"`
	Events []string `json:"events"`
}

// ListWebhooks: GET /admin/applications/{appId}/webhooks
func (h *AdminResourceHandler) ListWebhooks(w http.ResponseWriter, r *http.Request) {
	appID, ok := appIDParam(w, r)
	if !ok {
		return
	}
	eps, err := h.svc.ListWebhooks(r.Context(), appID)
	if err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	views := make([]webhookView, 0, len(eps))
	for _, e := range eps {
		views = append(views, toWebhookView(e))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": views})
}

// CreateWebhook: POST /admin/applications/{appId}/webhooks → devuelve el secreto una vez.
func (h *AdminResourceHandler) CreateWebhook(w http.ResponseWriter, r *http.Request) {
	appID, ok := appIDParam(w, r)
	if !ok {
		return
	}
	var req createWebhookRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	created, err := h.svc.CreateWebhook(r.Context(), adminActor(r), appID, req.URL, req.Events)
	if err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	v := toWebhookView(created.Endpoint)
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"id":        v.ID,
		"url":       v.URL,
		"events":    v.Events,
		"status":    v.Status,
		"createdAt": v.CreatedAt,
		"secret":    created.Secret, // se muestra una sola vez
	})
}

// DeleteWebhook: DELETE /admin/applications/{appId}/webhooks/{id}
func (h *AdminResourceHandler) DeleteWebhook(w http.ResponseWriter, r *http.Request) {
	appID, ok := appIDParam(w, r)
	if !ok {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteWebhook(r.Context(), adminActor(r), appID, id); err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type webhookView struct {
	ID        string   `json:"id"`
	URL       string   `json:"url"`
	Events    []string `json:"events"`
	Status    string   `json:"status"`
	CreatedAt string   `json:"createdAt"`
}

func toWebhookView(e sqlc.WebhookEndpoint) webhookView {
	return webhookView{
		ID:        e.ID.String(),
		URL:       e.Url,
		Events:    e.Events,
		Status:    string(e.Status),
		CreatedAt: e.CreatedAt.Time.Format(time.RFC3339),
	}
}
