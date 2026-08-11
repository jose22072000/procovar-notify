package http

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"dvtech/qbn/internal/apperr"
	"dvtech/qbn/internal/auth"
	"dvtech/qbn/internal/domain"
	"dvtech/qbn/internal/httpx"
	"dvtech/qbn/internal/notification"
)

// List: GET /v1/notifications — listado paginado por cursor con filtros.
func (h *NotificationHandler) List(w http.ResponseWriter, r *http.Request) {
	caller, ok := requireCaller(w, r)
	if !ok {
		return
	}
	params, err := parseListParams(r, caller.ApplicationID)
	if err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	res, err := h.svc.List(r.Context(), params)
	if err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"data":       toNotificationViews(res.Items),
		"nextCursor": res.NextCursor,
	})
}

// Get: GET /v1/notifications/{id}
func (h *NotificationHandler) Get(w http.ResponseWriter, r *http.Request) {
	caller, ok := requireCaller(w, r)
	if !ok {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	n, err := h.svc.Get(r.Context(), caller.ApplicationID, id)
	if err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toNotificationView(n))
}

// Cancel: POST /v1/notifications/{id}/cancel
func (h *NotificationHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	caller, ok := requireCaller(w, r)
	if !ok {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	n, err := h.svc.Cancel(r.Context(), caller.ApplicationID, id)
	if err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toNotificationView(n))
}

// MarkRead: POST /v1/notifications/{id}/read
func (h *NotificationHandler) MarkRead(w http.ResponseWriter, r *http.Request) {
	caller, ok := requireCaller(w, r)
	if !ok {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	n, err := h.svc.MarkRead(r.Context(), caller.ApplicationID, id)
	if err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toNotificationView(n))
}

// Archive: POST /v1/notifications/{id}/archive — saca la notificación de la
// bandeja sin borrarla ni perder si fue leída. Sin esto la campana acaba siendo
// una lista interminable.
func (h *NotificationHandler) Archive(w http.ResponseWriter, r *http.Request) {
	caller, ok := requireCaller(w, r)
	if !ok {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	n, err := h.svc.Archive(r.Context(), caller.ApplicationID, id)
	if err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toNotificationView(n))
}

// Unarchive: POST /v1/notifications/{id}/unarchive — la devuelve a la bandeja.
func (h *NotificationHandler) Unarchive(w http.ResponseWriter, r *http.Request) {
	caller, ok := requireCaller(w, r)
	if !ok {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	n, err := h.svc.Unarchive(r.Context(), caller.ApplicationID, id)
	if err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toNotificationView(n))
}

// ArchiveAllRead: POST /v1/inbox/archive-read?userId=... — archiva de golpe todas
// las leídas del usuario. Es la acción que de verdad limpia la bandeja.
//
// Va scopeada por destinatario en la propia SQL: aunque el llamante se equivoque,
// nunca puede archivar las de otro usuario.
func (h *NotificationHandler) ArchiveAllRead(w http.ResponseWriter, r *http.Request) {
	caller, ok := requireCaller(w, r)
	if !ok {
		return
	}
	userID := r.URL.Query().Get("userId")
	if userID == "" {
		httpx.WriteProblem(w, r, apperr.Validation("missing_user_id", "userId query parameter is required"))
		return
	}
	if err := h.svc.ArchiveAllRead(r.Context(), caller.ApplicationID, userID); err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Inbox: GET /v1/inbox?userId=... — bandeja in-app de un usuario.
func (h *NotificationHandler) Inbox(w http.ResponseWriter, r *http.Request) {
	caller, ok := requireCaller(w, r)
	if !ok {
		return
	}
	userID := r.URL.Query().Get("userId")
	if userID == "" {
		httpx.WriteProblem(w, r, apperr.Validation("missing_user_id", "userId query parameter is required"))
		return
	}
	params, err := parseListParams(r, caller.ApplicationID)
	if err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	inApp := domain.ChannelInApp
	params.Channel = &inApp
	params.RecipientUserID = &userID

	res, err := h.svc.List(r.Context(), params)
	if err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"data":       toNotificationViews(res.Items),
		"nextCursor": res.NextCursor,
	})
}

type batchRequest struct {
	TemplateKey      string  `json:"templateKey"`
	NotificationType string  `json:"notificationType"`
	Channel          string  `json:"channel"`
	Priority         string  `json:"priority"`
	Locale           *string `json:"locale"`
	Items            []struct {
		Recipient       map[string]any `json:"recipient"`
		RecipientUserID *string        `json:"recipientUserId"`
		Payload         map[string]any `json:"payload"`
		IdempotencyKey  *string        `json:"idempotencyKey"`
	} `json:"items"`
}

// Batch: POST /v1/notifications/batch — envío masivo con éxito parcial.
func (h *NotificationHandler) Batch(w http.ResponseWriter, r *http.Request) {
	caller, ok := requireCaller(w, r)
	if !ok {
		return
	}
	var req batchRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	if req.TemplateKey == "" || req.NotificationType == "" {
		httpx.WriteProblem(w, r, apperr.Validation("missing_fields", "templateKey and notificationType are required"))
		return
	}

	items := make([]notification.BatchItem, 0, len(req.Items))
	for _, it := range req.Items {
		items = append(items, notification.BatchItem{
			Recipient:       it.Recipient,
			RecipientUserID: it.RecipientUserID,
			Payload:         it.Payload,
			IdempotencyKey:  it.IdempotencyKey,
		})
	}

	res, err := h.svc.CreateBatch(r.Context(), notification.BatchInput{
		ApplicationID:    caller.ApplicationID,
		TemplateKey:      req.TemplateKey,
		NotificationType: req.NotificationType,
		Channel:          req.Channel,
		Priority:         req.Priority,
		Locale:           req.Locale,
		Items:            items,
	})
	if err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, res)
}

// GetPreferences: GET /v1/users/{userId}/preferences
func (h *NotificationHandler) GetPreferences(w http.ResponseWriter, r *http.Request) {
	caller, ok := requireCaller(w, r)
	if !ok {
		return
	}
	userID := chi.URLParam(r, "userId")
	prefs, err := h.svc.GetPreferences(r.Context(), caller.ApplicationID, userID)
	if err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, prefs)
}

// SetPreferences: PUT /v1/users/{userId}/preferences
func (h *NotificationHandler) SetPreferences(w http.ResponseWriter, r *http.Request) {
	caller, ok := requireCaller(w, r)
	if !ok {
		return
	}
	userID := chi.URLParam(r, "userId")
	var prefs notification.Preferences
	if err := httpx.DecodeJSON(r, &prefs); err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	updated, err := h.svc.SetPreferences(r.Context(), caller.ApplicationID, userID, prefs)
	if err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, updated)
}

// --- helpers ---

func requireCaller(w http.ResponseWriter, r *http.Request) (auth.Caller, bool) {
	caller, ok := auth.CallerFromContext(r.Context())
	if !ok {
		httpx.WriteProblem(w, r, apperr.Unauthorized("unauthenticated", "Authentication required"))
		return auth.Caller{}, false
	}
	return caller, true
}

func parseID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteProblem(w, r, apperr.Validation("invalid_id", "Invalid notification id"))
		return uuid.Nil, false
	}
	return id, true
}

func parseListParams(r *http.Request, appID uuid.UUID) (notification.ListParams, error) {
	q := r.URL.Query()
	params := notification.ListParams{ApplicationID: appID, Cursor: q.Get("cursor")}

	if v := q.Get("status"); v != "" {
		params.Status = &v
	}
	if v := q.Get("channel"); v != "" {
		params.Channel = &v
	}
	if v := q.Get("recipientUserId"); v != "" {
		params.RecipientUserID = &v
	}
	// archived=true → solo el archivo. Ausente o false → solo las activas, que es
	// lo que debe pedir la campana (si no, mostraría todo el histórico).
	if v := q.Get("archived"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return notification.ListParams{}, apperr.Validation("invalid_archived", "archived must be a boolean")
		}
		params.Archived = &b
	}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return notification.ListParams{}, apperr.Validation("invalid_limit", "limit must be an integer")
		}
		params.Limit = n
	}
	if v := q.Get("startDate"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return notification.ListParams{}, apperr.Validation("invalid_start_date", "startDate must be RFC3339")
		}
		params.StartDate = &t
	}
	if v := q.Get("endDate"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return notification.ListParams{}, apperr.Validation("invalid_end_date", "endDate must be RFC3339")
		}
		params.EndDate = &t
	}
	return params, nil
}
