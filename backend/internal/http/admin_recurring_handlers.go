package http

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"dvtech/qbn/internal/admin"
	"dvtech/qbn/internal/apperr"
	"dvtech/qbn/internal/httpx"
	"dvtech/qbn/internal/store/sqlc"
)

type recurringRequest struct {
	Name             string         `json:"name"`
	TemplateKey      string         `json:"templateKey"`
	NotificationType string         `json:"notificationType"`
	Channel          string         `json:"channel"`
	Recipient        map[string]any `json:"recipient"`
	RecipientUserID  *string        `json:"recipientUserId"`
	Payload          map[string]any `json:"payload"`
	Locale           *string        `json:"locale"`
	Priority         string         `json:"priority"`
	Cron             string         `json:"cron"`
}

// ListRecurring: GET /admin/applications/{appId}/recurring
func (h *AdminResourceHandler) ListRecurring(w http.ResponseWriter, r *http.Request) {
	appID, ok := appIDParam(w, r)
	if !ok {
		return
	}
	rs, err := h.svc.ListRecurring(r.Context(), appID)
	if err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	views := make([]recurringView, 0, len(rs))
	for _, s := range rs {
		views = append(views, toRecurringView(s))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": views})
}

// CreateRecurring: POST /admin/applications/{appId}/recurring
func (h *AdminResourceHandler) CreateRecurring(w http.ResponseWriter, r *http.Request) {
	appID, ok := appIDParam(w, r)
	if !ok {
		return
	}
	var req recurringRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	s, err := h.svc.CreateRecurring(r.Context(), adminActor(r), appID, admin.RecurringInput{
		Name: req.Name, TemplateKey: req.TemplateKey, NotificationType: req.NotificationType,
		Channel: req.Channel, Recipient: req.Recipient, RecipientUserID: req.RecipientUserID,
		Payload: req.Payload, Locale: req.Locale, Priority: req.Priority, Cron: req.Cron,
	})
	if err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toRecurringView(s))
}

// DeleteRecurring: DELETE /admin/applications/{appId}/recurring/{id}
func (h *AdminResourceHandler) DeleteRecurring(w http.ResponseWriter, r *http.Request) {
	appID, ok := appIDParam(w, r)
	if !ok {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteRecurring(r.Context(), adminActor(r), appID, id); err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type retentionRequest struct {
	RetentionDays int32 `json:"retentionDays"`
}

// SetRetention: PUT /admin/applications/{appId}/retention
func (h *AdminResourceHandler) SetRetention(w http.ResponseWriter, r *http.Request) {
	appID, ok := appIDParam(w, r)
	if !ok {
		return
	}
	var req retentionRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	app, err := h.svc.SetRetention(r.Context(), adminActor(r), appID, req.RetentionDays)
	if err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toAppView(app))
}

// ForgetUser: POST /admin/applications/{appId}/users/{userId}/forget
func (h *AdminResourceHandler) ForgetUser(w http.ResponseWriter, r *http.Request) {
	appID, ok := appIDParam(w, r)
	if !ok {
		return
	}
	userID := chi.URLParam(r, "userId")
	if userID == "" {
		httpx.WriteProblem(w, r, apperr.Validation("missing_user_id", "userId is required"))
		return
	}
	affected, err := h.svc.ForgetUser(r.Context(), adminActor(r), appID, userID)
	if err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"anonymized": affected})
}

type recurringView struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	TemplateKey      string `json:"templateKey"`
	NotificationType string `json:"notificationType"`
	Channel          string `json:"channel"`
	Cron             string `json:"cron"`
	Enabled          bool   `json:"enabled"`
	CreatedAt        string `json:"createdAt"`
}

func toRecurringView(s sqlc.RecurringSchedule) recurringView {
	return recurringView{
		ID: s.ID.String(), Name: s.Name, TemplateKey: s.TemplateKey,
		NotificationType: s.NotificationType, Channel: string(s.Channel),
		Cron: s.Cron, Enabled: s.Enabled, CreatedAt: s.CreatedAt.Time.Format(time.RFC3339),
	}
}
