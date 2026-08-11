package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"dvtech/qbn/internal/apperr"
	"dvtech/qbn/internal/httpx"
	"dvtech/qbn/internal/monitor"
	"dvtech/qbn/internal/notification"
	"dvtech/qbn/internal/store/sqlc"
)

// AdminMonitorHandler expone la vista admin de notificaciones y el monitor de
// cola por aplicación (§8.3).
type AdminMonitorHandler struct {
	monitor *monitor.Service
	notif   *notification.Service
}

// NewAdminMonitorHandler crea el handler.
func NewAdminMonitorHandler(m *monitor.Service, n *notification.Service) *AdminMonitorHandler {
	return &AdminMonitorHandler{monitor: m, notif: n}
}

// ListNotifications: GET /admin/applications/{appId}/notifications (y /tasks).
func (h *AdminMonitorHandler) ListNotifications(w http.ResponseWriter, r *http.Request) {
	appID, ok := appIDParam(w, r)
	if !ok {
		return
	}
	params, err := parseListParams(r, appID)
	if err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	res, err := h.notif.List(r.Context(), params)
	if err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"data":       toNotificationViews(res.Items),
		"nextCursor": res.NextCursor,
	})
}

// NotificationDetail: GET /admin/applications/{appId}/notifications/{id}
func (h *AdminMonitorHandler) NotificationDetail(w http.ResponseWriter, r *http.Request) {
	appID, ok := appIDParam(w, r)
	if !ok {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	d, err := h.monitor.Detail(r.Context(), appID, id)
	if err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"notification": toNotificationView(d.Notification),
		"attempts":     toAttemptViews(d.Attempts),
		"logs":         toLogViews(d.Logs),
	})
}

// Metrics: GET /admin/applications/{appId}/metrics
func (h *AdminMonitorHandler) Metrics(w http.ResponseWriter, r *http.Request) {
	appID, ok := appIDParam(w, r)
	if !ok {
		return
	}
	m, err := h.monitor.Metrics(r.Context(), appID)
	if err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, m)
}

// TasksSummary: GET /admin/applications/{appId}/tasks/summary
func (h *AdminMonitorHandler) TasksSummary(w http.ResponseWriter, r *http.Request) {
	appID, ok := appIDParam(w, r)
	if !ok {
		return
	}
	s, err := h.monitor.Summary(r.Context(), appID)
	if err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, s)
}

// sseMaxDuration acota la conexión SSE por debajo del WriteTimeout del servidor;
// el navegador (EventSource) reconecta automáticamente.
const sseMaxDuration = 25 * time.Second

// TasksStream: GET /admin/applications/{appId}/tasks/stream — SSE del summary.
func (h *AdminMonitorHandler) TasksStream(w http.ResponseWriter, r *http.Request) {
	appID, ok := appIDParam(w, r)
	if !ok {
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		httpx.WriteProblem(w, r, apperr.Internal(fmt.Errorf("streaming unsupported")))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	send := func() bool {
		s, err := h.monitor.Summary(r.Context(), appID)
		if err != nil {
			return false
		}
		data, _ := json.Marshal(s)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
		return true
	}

	send() // primer envío inmediato
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	deadline := time.After(sseMaxDuration)

	for {
		select {
		case <-r.Context().Done():
			return
		case <-deadline:
			return
		case <-ticker.C:
			if !send() {
				return
			}
		}
	}
}

// RetryTask: POST /admin/applications/{appId}/tasks/{id}/retry
func (h *AdminMonitorHandler) RetryTask(w http.ResponseWriter, r *http.Request) {
	appID, ok := appIDParam(w, r)
	if !ok {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := h.monitor.Retry(r.Context(), appID, id); err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "requeued"})
}

// CancelTask: POST /admin/applications/{appId}/tasks/{id}/cancel
func (h *AdminMonitorHandler) CancelTask(w http.ResponseWriter, r *http.Request) {
	appID, ok := appIDParam(w, r)
	if !ok {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := h.monitor.Cancel(r.Context(), appID, id); err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "cancelled"})
}

// --- views ---

type attemptView struct {
	AttemptNumber int32   `json:"attemptNumber"`
	Status        string  `json:"status"`
	ProviderRef   *string `json:"providerRef,omitempty"`
	ErrorCode     *string `json:"errorCode,omitempty"`
	ErrorMessage  *string `json:"errorMessage,omitempty"`
	StartedAt     string  `json:"startedAt"`
	FinishedAt    *string `json:"finishedAt,omitempty"`
}

func toAttemptViews(as []sqlc.DeliveryAttempt) []attemptView {
	out := make([]attemptView, 0, len(as))
	for _, a := range as {
		out = append(out, attemptView{
			AttemptNumber: a.AttemptNumber,
			Status:        string(a.Status),
			ProviderRef:   a.ProviderRef,
			ErrorCode:     a.ErrorCode,
			ErrorMessage:  a.ErrorMessage,
			StartedAt:     a.StartedAt.Time.Format(time.RFC3339),
			FinishedAt:    tsPtr(a.FinishedAt),
		})
	}
	return out
}

type logView struct {
	Event     string          `json:"event"`
	Message   *string         `json:"message,omitempty"`
	Details   json.RawMessage `json:"details,omitempty"`
	CreatedAt string          `json:"createdAt"`
}

func toLogViews(ls []sqlc.NotificationLog) []logView {
	out := make([]logView, 0, len(ls))
	for _, l := range ls {
		out = append(out, logView{
			Event:     l.Event,
			Message:   l.Message,
			Details:   json.RawMessage(l.Details),
			CreatedAt: l.CreatedAt.Time.Format(time.RFC3339),
		})
	}
	return out
}
