package http

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"dvtech/qbn/internal/httpx"
	"dvtech/qbn/internal/store/sqlc"
)

// ListAudit: GET /admin/applications/{appId}/audit — registro de acciones de
// administración (AdminAuditLog) de la aplicación, paginado.
func (h *AdminResourceHandler) ListAudit(w http.ResponseWriter, r *http.Request) {
	appID, ok := appIDParam(w, r)
	if !ok {
		return
	}
	limit := queryInt(r, "limit", 50)
	offset := queryInt(r, "offset", 0)

	logs, err := h.svc.ListAuditLogs(r.Context(), appID, int32(limit), int32(offset))
	if err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	views := make([]auditView, 0, len(logs))
	for _, l := range logs {
		views = append(views, toAuditView(l))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": views})
}

func queryInt(r *http.Request, key string, def int) int {
	if v := r.URL.Query().Get(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

type auditView struct {
	ID         string          `json:"id"`
	ActorID    string          `json:"actorAdminId"`
	Action     string          `json:"action"`
	TargetType string          `json:"targetType"`
	TargetID   *string         `json:"targetId,omitempty"`
	Details    json.RawMessage `json:"details,omitempty"`
	IP         *string         `json:"ip,omitempty"`
	UserAgent  *string         `json:"userAgent,omitempty"`
	CreatedAt  string          `json:"createdAt"`
}

func toAuditView(l sqlc.AdminAuditLog) auditView {
	v := auditView{
		ID:         l.ID.String(),
		ActorID:    uuid.UUID(l.ActorAdminID).String(),
		Action:     l.Action,
		TargetType: l.TargetType,
		TargetID:   l.TargetID,
		IP:         l.Ip,
		UserAgent:  l.UserAgent,
		CreatedAt:  l.CreatedAt.Time.Format(time.RFC3339),
	}
	if len(l.Details) > 0 {
		v.Details = json.RawMessage(l.Details)
	}
	return v
}
