package http

import (
	"net/http"
	"time"

	"dvtech/qbn/internal/httpx"
	"dvtech/qbn/internal/store/sqlc"
)

type addSuppressionRequest struct {
	Channel   string `json:"channel"`
	Recipient string `json:"recipient"`
	Reason    string `json:"reason"`
}

// ListSuppressions: GET /admin/applications/{appId}/suppressions
func (h *AdminResourceHandler) ListSuppressions(w http.ResponseWriter, r *http.Request) {
	appID, ok := appIDParam(w, r)
	if !ok {
		return
	}
	rows, err := h.svc.ListSuppressions(r.Context(), appID, int32(queryInt(r, "limit", 50)), int32(queryInt(r, "offset", 0)))
	if err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	views := make([]suppressionView, 0, len(rows))
	for _, s := range rows {
		views = append(views, toSuppressionView(s))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": views})
}

// AddSuppression: POST /admin/applications/{appId}/suppressions
func (h *AdminResourceHandler) AddSuppression(w http.ResponseWriter, r *http.Request) {
	appID, ok := appIDParam(w, r)
	if !ok {
		return
	}
	var req addSuppressionRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	row, err := h.svc.AddSuppression(r.Context(), adminActor(r), appID, req.Channel, req.Recipient, req.Reason)
	if err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toSuppressionView(row))
}

// DeleteSuppression: DELETE /admin/applications/{appId}/suppressions/{id}
func (h *AdminResourceHandler) DeleteSuppression(w http.ResponseWriter, r *http.Request) {
	appID, ok := appIDParam(w, r)
	if !ok {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteSuppression(r.Context(), adminActor(r), appID, id); err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type suppressionView struct {
	ID        string `json:"id"`
	Channel   string `json:"channel"`
	Recipient string `json:"recipient"`
	Reason    string `json:"reason"`
	CreatedAt string `json:"createdAt"`
}

func toSuppressionView(s sqlc.SuppressionList) suppressionView {
	return suppressionView{
		ID:        s.ID.String(),
		Channel:   string(s.Channel),
		Recipient: s.Recipient,
		Reason:    s.Reason,
		CreatedAt: s.CreatedAt.Time.Format(time.RFC3339),
	}
}
