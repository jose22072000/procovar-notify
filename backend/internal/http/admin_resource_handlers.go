package http

import (
	"net/http"
	"time"

	"github.com/google/uuid"

	"dvtech/qbn/internal/admin"
	"dvtech/qbn/internal/apperr"
	"dvtech/qbn/internal/auth"
	"dvtech/qbn/internal/httpx"
	"dvtech/qbn/internal/store/sqlc"
)

// AdminResourceHandler expone la gestión de recursos de administración:
// aplicaciones, admins, API keys, SMTP, proveedores y rutas.
type AdminResourceHandler struct {
	svc *admin.Service
}

// NewAdminResourceHandler crea el handler.
func NewAdminResourceHandler(svc *admin.Service) *AdminResourceHandler {
	return &AdminResourceHandler{svc: svc}
}

func adminActor(r *http.Request) uuid.UUID {
	a, _ := auth.AdminFromContext(r.Context())
	return a.ID
}

// --- Aplicaciones ---

type createAppRequest struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

func (h *AdminResourceHandler) ListApplications(w http.ResponseWriter, r *http.Request) {
	apps, err := h.svc.ListApplications(r.Context())
	if err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	views := make([]appView, 0, len(apps))
	for _, a := range apps {
		views = append(views, toAppView(a))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": views})
}

func (h *AdminResourceHandler) CreateApplication(w http.ResponseWriter, r *http.Request) {
	var req createAppRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	if req.Name == "" || req.Slug == "" {
		httpx.WriteProblem(w, r, apperr.Validation("missing_fields", "name and slug are required"))
		return
	}
	app, err := h.svc.CreateApplication(r.Context(), adminActor(r), req.Name, req.Slug)
	if err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toAppView(app))
}

func (h *AdminResourceHandler) GetApplication(w http.ResponseWriter, r *http.Request) {
	appID, ok := appIDParam(w, r)
	if !ok {
		return
	}
	app, err := h.svc.GetApplication(r.Context(), appID)
	if err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toAppView(app))
}

type updateAppRequest struct {
	Name   *string `json:"name"`
	Slug   *string `json:"slug"`
	Status *string `json:"status"`
}

func (h *AdminResourceHandler) UpdateApplication(w http.ResponseWriter, r *http.Request) {
	// Cambiar nombre/estado del tenant es exclusivo de super-admin.
	if admin, _ := auth.AdminFromContext(r.Context()); !admin.IsSuperAdmin() {
		httpx.WriteProblem(w, r, apperr.Forbidden("forbidden", "Requires super-admin role"))
		return
	}
	appID, ok := appIDParam(w, r)
	if !ok {
		return
	}
	var req updateAppRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	app, err := h.svc.UpdateApplication(r.Context(), adminActor(r), appID, req.Name, req.Slug, req.Status)
	if err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toAppView(app))
}

// --- Admin users ---

type createAdminUserRequest struct {
	Email         string  `json:"email"`
	Password      string  `json:"password"`
	Role          string  `json:"role"`
	ApplicationID *string `json:"applicationId"`
}

func (h *AdminResourceHandler) ListAdminUsers(w http.ResponseWriter, r *http.Request) {
	us, err := h.svc.ListAdminUsers(r.Context())
	if err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	views := make([]adminUserView, 0, len(us))
	for _, u := range us {
		views = append(views, toAdminUserView(u))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": views})
}

func (h *AdminResourceHandler) CreateAdminUser(w http.ResponseWriter, r *http.Request) {
	var req createAdminUserRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	if req.Email == "" || req.Password == "" || req.Role == "" {
		httpx.WriteProblem(w, r, apperr.Validation("missing_fields", "email, password and role are required"))
		return
	}
	var appID *uuid.UUID
	if req.ApplicationID != nil && *req.ApplicationID != "" {
		id, err := uuid.Parse(*req.ApplicationID)
		if err != nil {
			httpx.WriteProblem(w, r, apperr.Validation("invalid_app_id", "Invalid applicationId"))
			return
		}
		appID = &id
	}
	u, err := h.svc.CreateAdminUser(r.Context(), adminActor(r), req.Email, req.Password, req.Role, appID)
	if err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toAdminUserView(u))
}

// --- API keys ---

type createAPIKeyRequest struct {
	// Name identifica de qué servicio es la key (qb-back, qb-panel, qb-booking…).
	// Sin él la lista solo mostraba key_id y no había forma de saber cuál es cuál.
	Name   string   `json:"name"`
	Scopes []string `json:"scopes"`
}

func (h *AdminResourceHandler) ListAPIKeys(w http.ResponseWriter, r *http.Request) {
	appID, ok := appIDParam(w, r)
	if !ok {
		return
	}
	keys, err := h.svc.ListAPIKeys(r.Context(), appID)
	if err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	views := make([]apiKeyView, 0, len(keys))
	for _, k := range keys {
		views = append(views, toAPIKeyView(k))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": views})
}

func (h *AdminResourceHandler) CreateAPIKey(w http.ResponseWriter, r *http.Request) {
	appID, ok := appIDParam(w, r)
	if !ok {
		return
	}
	var req createAPIKeyRequest
	// El cuerpo es opcional (scopes por defecto), pero uno presente y malformado
	// (p. ej. campo 'scopes' mal escrito → DisallowUnknownFields) debe dar 400,
	// no descartarse en silencio y crear la key con scopes vacíos.
	if err := httpx.DecodeJSON(r, &req); err != nil && apperr.From(err).Code != "empty_body" {
		httpx.WriteProblem(w, r, err)
		return
	}
	created, err := h.svc.CreateAPIKey(r.Context(), adminActor(r), appID, req.Name, req.Scopes)
	if err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	// El secret se devuelve UNA sola vez.
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"id":     created.ID.String(),
		"keyId":  created.KeyID,
		"secret": created.Secret,
		"scopes": created.Scopes,
		"name":   req.Name,
	})
}

// DeleteAPIKey borra una key ya revocada (limpieza de la lista). El servicio
// rechaza borrar una key ACTIVE: hay que revocarla primero.
func (h *AdminResourceHandler) DeleteAPIKey(w http.ResponseWriter, r *http.Request) {
	appID, ok := appIDParam(w, r)
	if !ok {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteAPIKey(r.Context(), adminActor(r), appID, id); err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AdminResourceHandler) RevokeAPIKey(w http.ResponseWriter, r *http.Request) {
	appID, ok := appIDParam(w, r)
	if !ok {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := h.svc.RevokeAPIKey(r.Context(), adminActor(r), appID, id); err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- SMTP ---

type smtpRequest struct {
	Name      string `json:"name"`
	Host      string `json:"host"`
	Port      int32  `json:"port"`
	Username  string `json:"username"`
	Password  string `json:"password"`
	FromEmail string `json:"fromEmail"`
	FromName  string `json:"fromName"`
	Secure    bool   `json:"secure"`
	Status    string `json:"status"`
}

func (req smtpRequest) toInput() admin.SMTPInput {
	return admin.SMTPInput{
		Name: req.Name, Host: req.Host, Port: req.Port, Username: req.Username,
		Password: req.Password, FromEmail: req.FromEmail, FromName: req.FromName,
		Secure: req.Secure, Status: req.Status,
	}
}

func (h *AdminResourceHandler) ListSMTP(w http.ResponseWriter, r *http.Request) {
	appID, ok := appIDParam(w, r)
	if !ok {
		return
	}
	cs, err := h.svc.ListSMTP(r.Context(), appID)
	if err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	views := make([]smtpView, 0, len(cs))
	for _, c := range cs {
		views = append(views, toSMTPView(c))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": views})
}

func (h *AdminResourceHandler) CreateSMTP(w http.ResponseWriter, r *http.Request) {
	appID, ok := appIDParam(w, r)
	if !ok {
		return
	}
	var req smtpRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	c, err := h.svc.CreateSMTP(r.Context(), adminActor(r), appID, req.toInput())
	if err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toSMTPView(c))
}

func (h *AdminResourceHandler) UpdateSMTP(w http.ResponseWriter, r *http.Request) {
	appID, ok := appIDParam(w, r)
	if !ok {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var req smtpRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	c, err := h.svc.UpdateSMTP(r.Context(), adminActor(r), appID, id, req.toInput())
	if err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toSMTPView(c))
}

func (h *AdminResourceHandler) DeleteSMTP(w http.ResponseWriter, r *http.Request) {
	appID, ok := appIDParam(w, r)
	if !ok {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteSMTP(r.Context(), adminActor(r), appID, id); err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AdminResourceHandler) TestSMTP(w http.ResponseWriter, r *http.Request) {
	appID, ok := appIDParam(w, r)
	if !ok {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := h.svc.TestSMTP(r.Context(), appID, id); err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

// --- Proveedores ---

type providerRequest struct {
	Channel  string         `json:"channel"`
	Provider string         `json:"provider"`
	Name     string         `json:"name"`
	Config   map[string]any `json:"config"`
	Status   string         `json:"status"`
}

func (req providerRequest) toInput() admin.ProviderInput {
	return admin.ProviderInput{Channel: req.Channel, Provider: req.Provider, Name: req.Name, Config: req.Config, Status: req.Status}
}

func (h *AdminResourceHandler) ListProviders(w http.ResponseWriter, r *http.Request) {
	appID, ok := appIDParam(w, r)
	if !ok {
		return
	}
	ps, err := h.svc.ListProviders(r.Context(), appID)
	if err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	views := make([]providerView, 0, len(ps))
	for _, p := range ps {
		views = append(views, toProviderView(p))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": views})
}

func (h *AdminResourceHandler) CreateProvider(w http.ResponseWriter, r *http.Request) {
	appID, ok := appIDParam(w, r)
	if !ok {
		return
	}
	var req providerRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	p, err := h.svc.CreateProvider(r.Context(), adminActor(r), appID, req.toInput())
	if err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toProviderView(p))
}

func (h *AdminResourceHandler) UpdateProvider(w http.ResponseWriter, r *http.Request) {
	appID, ok := appIDParam(w, r)
	if !ok {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var req providerRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	p, err := h.svc.UpdateProvider(r.Context(), adminActor(r), appID, id, req.toInput())
	if err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toProviderView(p))
}

func (h *AdminResourceHandler) DeleteProvider(w http.ResponseWriter, r *http.Request) {
	appID, ok := appIDParam(w, r)
	if !ok {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteProvider(r.Context(), adminActor(r), appID, id); err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Rutas de canal ---

type routeRequest struct {
	NotificationType  string  `json:"notificationType"`
	Channel           string  `json:"channel"`
	TemplateKey       *string `json:"templateKey"`
	SMTPConnectionID  *string `json:"smtpConnectionId"`
	ChannelProviderID *string `json:"channelProviderId"`
	SendPriority      string  `json:"sendPriority"`     // LOW|NORMAL|HIGH|URGENT ("" => NORMAL)
	PIIRetentionDays  *int32  `json:"piiRetentionDays"` // null hereda el de la app
	RetentionDays     *int32  `json:"retentionDays"`    // null = no borrar nunca
}

func (h *AdminResourceHandler) ListRoutes(w http.ResponseWriter, r *http.Request) {
	appID, ok := appIDParam(w, r)
	if !ok {
		return
	}
	rs, err := h.svc.ListRoutes(r.Context(), appID)
	if err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	views := make([]routeView, 0, len(rs))
	for _, rt := range rs {
		views = append(views, toRouteView(rt))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": views})
}

func (h *AdminResourceHandler) CreateRoute(w http.ResponseWriter, r *http.Request) {
	appID, ok := appIDParam(w, r)
	if !ok {
		return
	}
	var req routeRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	smtpID, err := parseOptUUID(req.SMTPConnectionID)
	if err != nil {
		httpx.WriteProblem(w, r, apperr.Validation("invalid_smtp_id", "Invalid smtpConnectionId"))
		return
	}
	provID, err := parseOptUUID(req.ChannelProviderID)
	if err != nil {
		httpx.WriteProblem(w, r, apperr.Validation("invalid_provider_id", "Invalid channelProviderId"))
		return
	}
	route, err := h.svc.CreateRoute(r.Context(), adminActor(r), appID, admin.RouteInput{
		NotificationType: req.NotificationType, Channel: req.Channel,
		TemplateKey:      req.TemplateKey,
		SMTPConnectionID: smtpID, ChannelProviderID: provID,
		SendPriority:     req.SendPriority,
		PIIRetentionDays: req.PIIRetentionDays, RetentionDays: req.RetentionDays,
	})
	if err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toRouteView(route))
}

// UpdateRoute: PATCH /admin/applications/{appId}/routes/{id} — ajustes
// operables del Tipo (prioridad y retención). Envía SIEMPRE los tres campos:
// null en retención es un valor válido (heredar / no borrar), no "sin cambio".
func (h *AdminResourceHandler) UpdateRoute(w http.ResponseWriter, r *http.Request) {
	appID, ok := appIDParam(w, r)
	if !ok {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var req struct {
		SendPriority     string `json:"sendPriority"`
		PIIRetentionDays *int32 `json:"piiRetentionDays"`
		RetentionDays    *int32 `json:"retentionDays"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	route, err := h.svc.UpdateRouteSettings(r.Context(), adminActor(r), appID, id, admin.RouteSettings{
		SendPriority: req.SendPriority, PIIRetentionDays: req.PIIRetentionDays, RetentionDays: req.RetentionDays,
	})
	if err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toRouteView(route))
}

func (h *AdminResourceHandler) DeleteRoute(w http.ResponseWriter, r *http.Request) {
	appID, ok := appIDParam(w, r)
	if !ok {
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteRoute(r.Context(), adminActor(r), appID, id); err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func parseOptUUID(s *string) (*uuid.UUID, error) {
	if s == nil || *s == "" {
		return nil, nil
	}
	id, err := uuid.Parse(*s)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

// --- views (sin secretos) ---

type appView struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Slug             string `json:"slug"`
	Status           string `json:"status"`
	DailySendQuota   int32  `json:"dailySendQuota"`
	MonthlySendQuota int32  `json:"monthlySendQuota"`
	CreatedAt        string `json:"createdAt"`
}

func toAppView(a sqlc.Application) appView {
	return appView{
		ID: a.ID.String(), Name: a.Name, Slug: a.Slug, Status: string(a.Status),
		DailySendQuota: a.DailySendQuota, MonthlySendQuota: a.MonthlySendQuota,
		CreatedAt: a.CreatedAt.Time.Format(time.RFC3339),
	}
}

type adminUserView struct {
	ID            string  `json:"id"`
	Email         string  `json:"email"`
	Role          string  `json:"role"`
	ApplicationID *string `json:"applicationId,omitempty"`
	Status        string  `json:"status"`
}

func toAdminUserView(u sqlc.AdminUser) adminUserView {
	v := adminUserView{ID: u.ID.String(), Email: u.Email, Role: string(u.Role), Status: string(u.Status)}
	if u.ApplicationID.Valid {
		s := uuid.UUID(u.ApplicationID.Bytes).String()
		v.ApplicationID = &s
	}
	return v
}

type apiKeyView struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	KeyID      string   `json:"keyId"`
	Scopes     []string `json:"scopes"`
	Status     string   `json:"status"`
	LastUsedAt *string  `json:"lastUsedAt,omitempty"`
	CreatedAt  string   `json:"createdAt"`
}

func toAPIKeyView(k sqlc.ApiKey) apiKeyView {
	return apiKeyView{
		ID: k.ID.String(), Name: k.Name, KeyID: k.KeyID, Scopes: k.Scopes, Status: string(k.Status),
		LastUsedAt: tsPtr(k.LastUsedAt), CreatedAt: k.CreatedAt.Time.Format(time.RFC3339),
	}
}

type smtpView struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Host      string `json:"host"`
	Port      int32  `json:"port"`
	Username  string `json:"username"`
	FromEmail string `json:"fromEmail"`
	FromName  string `json:"fromName"`
	Secure    bool   `json:"secure"`
	Status    string `json:"status"`
}

func toSMTPView(c sqlc.SmtpConnection) smtpView {
	return smtpView{
		ID: c.ID.String(), Name: c.Name, Host: c.Host, Port: c.Port, Username: c.Username,
		FromEmail: c.FromEmail, FromName: c.FromName, Secure: c.Secure, Status: string(c.Status),
	}
}

type providerView struct {
	ID       string `json:"id"`
	Channel  string `json:"channel"`
	Provider string `json:"provider"`
	Name     string `json:"name"`
	Status   string `json:"status"`
}

func toProviderView(p sqlc.ChannelProvider) providerView {
	return providerView{ID: p.ID.String(), Channel: string(p.Channel), Provider: p.Provider, Name: p.Name, Status: string(p.Status)}
}

type routeView struct {
	ID                string  `json:"id"`
	NotificationType  string  `json:"notificationType"`
	Channel           string  `json:"channel"`
	TemplateKey       *string `json:"templateKey,omitempty"`
	SMTPConnectionID  *string `json:"smtpConnectionId,omitempty"`
	ChannelProviderID *string `json:"channelProviderId,omitempty"`
	SendPriority      string  `json:"sendPriority"`
	PIIRetentionDays  *int32  `json:"piiRetentionDays,omitempty"`
	RetentionDays     *int32  `json:"retentionDays,omitempty"`
}

func toRouteView(rt sqlc.ChannelRoute) routeView {
	v := routeView{
		ID: rt.ID.String(), NotificationType: rt.NotificationType, Channel: string(rt.Channel),
		TemplateKey: rt.TemplateKey, SendPriority: string(rt.SendPriority),
		PIIRetentionDays: rt.PiiRetentionDays, RetentionDays: rt.RetentionDays,
	}
	if rt.SmtpConnectionID.Valid {
		s := uuid.UUID(rt.SmtpConnectionID.Bytes).String()
		v.SMTPConnectionID = &s
	}
	if rt.ChannelProviderID.Valid {
		s := uuid.UUID(rt.ChannelProviderID.Bytes).String()
		v.ChannelProviderID = &s
	}
	return v
}
