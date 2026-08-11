package http

import (
	"net/http"

	"dvtech/qbn/internal/apperr"
	"dvtech/qbn/internal/auth"
	"dvtech/qbn/internal/httpx"
)

type setQuotaRequest struct {
	DailySendQuota   int32 `json:"dailySendQuota"`
	MonthlySendQuota int32 `json:"monthlySendQuota"`
}

// SetQuota: PUT /admin/applications/{appId}/quota — fija las cuotas de envío.
// Exclusivo de super-admin (la plataforma decide los límites del tenant).
func (h *AdminResourceHandler) SetQuota(w http.ResponseWriter, r *http.Request) {
	if admin, _ := auth.AdminFromContext(r.Context()); !admin.IsSuperAdmin() {
		httpx.WriteProblem(w, r, apperr.Forbidden("forbidden", "Requires super-admin role"))
		return
	}
	appID, ok := appIDParam(w, r)
	if !ok {
		return
	}
	var req setQuotaRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	app, err := h.svc.SetQuota(r.Context(), adminActor(r), appID, req.DailySendQuota, req.MonthlySendQuota)
	if err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toAppView(app))
}
