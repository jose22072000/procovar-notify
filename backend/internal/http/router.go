package http

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"dvtech/qbn/internal/auth"
	"dvtech/qbn/internal/domain"
	"dvtech/qbn/internal/metrics"
	"dvtech/qbn/internal/observability"
)

// RouterDeps son las dependencias necesarias para construir el router.
type RouterDeps struct {
	Logger          *slog.Logger
	Health          *HealthHandler
	HMAC            *auth.HMACAuthenticator
	RateLimiter     *auth.RateLimiter
	AuthRateLimiter *auth.RateLimiter
	AdminAuth       *auth.AdminAuthenticator
	Notification    *NotificationHandler
	AdminTemplate   *AdminTemplateHandler
	AdminResource   *AdminResourceHandler
	AdminMonitor    *AdminMonitorHandler

	// CORSAllowedOrigins: orígenes permitidos para llamar a /admin desde el
	// navegador (el SPA vive en otro origen cuando no hay proxy delante).
	CORSAllowedOrigins []string

	// RefreshCookie: config de la cookie HttpOnly del refresh token.
	RefreshCookie CookieConfig
}

// maxBytes limita el tamaño del cuerpo de la petición para evitar DoS por memoria
// (importante en endpoints no autenticados como el login). Al exceder el límite,
// la lectura del body falla y DecodeJSON devuelve 413.
func maxBytes(n int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, n)
			next.ServeHTTP(w, r)
		})
	}
}

// NewRouter construye el router HTTP con los middlewares transversales, las
// rutas de salud, la API pública /v1 (HMAC) y la API de administración /admin.
func NewRouter(deps RouterDeps) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.Recoverer)
	r.Use(observability.RequestID(deps.Logger))
	r.Use(observability.AccessLog)

	// Salud / readiness / métricas (públicas, sin auth).
	r.Get("/healthz", deps.Health.Live)
	r.Get("/health", deps.Health.Live) // alias k8s liveness (DevOps probes /health)
	r.Get("/readyz", deps.Health.Ready)
	r.Handle("/metrics", metrics.Handler())

	// API pública (tenants) — autenticada por HMAC + rate limit por API key.
	r.Route("/v1", func(v1 chi.Router) {
		v1.Use(deps.HMAC.Middleware)
		v1.Use(deps.RateLimiter.Middleware)
		v1.Get("/whoami", WhoAmI)

		n := deps.Notification

		// Lectura / bandeja (scope notifications:read).
		v1.Group(func(rd chi.Router) {
			rd.Use(auth.RequireScope(domain.ScopeNotificationsRead))
			rd.Get("/notifications", n.List)
			rd.Get("/notifications/{id}", n.Get)
			rd.Post("/notifications/{id}/read", n.MarkRead)
			// Archivar es una acción del DESTINATARIO (como marcar leída), no un
			// envío: por eso vive en el grupo de lectura, que es el scope que
			// llevan los frontends con campana.
			rd.Post("/notifications/{id}/archive", n.Archive)
			rd.Post("/notifications/{id}/unarchive", n.Unarchive)
			rd.Post("/inbox/archive-read", n.ArchiveAllRead)
			rd.Get("/inbox", n.Inbox)
			rd.Get("/users/{userId}/preferences", n.GetPreferences)
		})

		// Envío / gestión (scope notifications:send).
		v1.Group(func(sd chi.Router) {
			sd.Use(auth.RequireScope(domain.ScopeNotificationsSend))
			sd.Post("/notifications", n.Create)
			sd.Post("/notifications/batch", n.Batch)
			sd.Post("/notifications/{id}/cancel", n.Cancel)
			sd.Post("/events", n.IngestEvent)
			sd.Put("/users/{userId}/preferences", n.SetPreferences)
		})
	})

	// API de administración (SPA) — login público + recursos protegidos por JWT.
	adminHandler := NewAdminAuthHandler(deps.AdminAuth, deps.RefreshCookie)
	r.Route("/admin", func(adm chi.Router) {
		// Primero CORS: responde el preflight OPTIONS antes de llegar a la auth
		// (que rechazaría una petición sin Authorization).
		adm.Use(CORSMiddleware(deps.CORSAllowedOrigins))
		adm.Use(maxBytes(1 << 20)) // 1 MiB: los cuerpos admin son pequeños (anti DoS)
		// Login/refresh: públicos pero con rate-limit por IP (anti fuerza bruta).
		adm.Group(func(authRoutes chi.Router) {
			authRoutes.Use(deps.AuthRateLimiter.IPMiddleware)
			authRoutes.Post("/auth/login", adminHandler.Login)
			authRoutes.Post("/auth/refresh", adminHandler.Refresh)
			authRoutes.Post("/auth/logout", adminHandler.Logout)
		})

		adm.Group(func(protected chi.Router) {
			protected.Use(deps.AdminAuth.Middleware)
			protected.Use(auditMeta)
			protected.Get("/me", adminHandler.Me)
			protected.Get("/base-templates", deps.AdminTemplate.ListBase)

			res := deps.AdminResource

			// Colecciones globales (solo super-admin).
			protected.Group(func(sa chi.Router) {
				sa.Use(auth.RequireSuperAdmin)
				sa.Get("/applications", res.ListApplications)
				sa.Post("/applications", res.CreateApplication)
				sa.Get("/admin-users", res.ListAdminUsers)
				sa.Post("/admin-users", res.CreateAdminUser)
			})

			// Recursos por aplicación (requieren acceso del admin a :appId).
			protected.Route("/applications/{appId}", func(a chi.Router) {
				a.Use(requireAppAccess)
				a.Get("/", res.GetApplication)
				a.Patch("/", res.UpdateApplication) // el handler exige super-admin

				a.Route("/api-keys", func(k chi.Router) {
					k.Get("/", res.ListAPIKeys)
					k.Post("/", res.CreateAPIKey)
					k.Delete("/{id}", res.RevokeAPIKey)
					// Borrado real (limpiar la lista). Exige que ya esté revocada.
					k.Delete("/{id}/purge", res.DeleteAPIKey)
				})
				a.Route("/smtp", func(m chi.Router) {
					m.Get("/", res.ListSMTP)
					m.Post("/", res.CreateSMTP)
					m.Patch("/{id}", res.UpdateSMTP)
					m.Delete("/{id}", res.DeleteSMTP)
					m.Post("/{id}/test", res.TestSMTP)
				})
				a.Route("/providers", func(pv chi.Router) {
					pv.Get("/", res.ListProviders)
					pv.Post("/", res.CreateProvider)
					pv.Patch("/{id}", res.UpdateProvider)
					pv.Delete("/{id}", res.DeleteProvider)
				})
				a.Route("/routes", func(rt chi.Router) {
					rt.Get("/", res.ListRoutes)
					rt.Post("/", res.CreateRoute)
					rt.Patch("/{id}", res.UpdateRoute)
					rt.Delete("/{id}", res.DeleteRoute)
				})
				a.Route("/templates", func(t chi.Router) {
					t.Get("/", deps.AdminTemplate.List)
					t.Post("/", deps.AdminTemplate.Create)
					t.Post("/preview", deps.AdminTemplate.PreviewDraft)
					t.Get("/{id}", deps.AdminTemplate.Get)
					t.Patch("/{id}", deps.AdminTemplate.Update)
					t.Delete("/{id}", deps.AdminTemplate.Delete)
					t.Post("/{id}/preview", deps.AdminTemplate.Preview)
				})

				// Notificaciones (tabla + detalle) y métricas.
				m := deps.AdminMonitor
				a.Get("/notifications", m.ListNotifications)
				a.Get("/notifications/{id}", m.NotificationDetail)
				a.Get("/metrics", m.Metrics)
				a.Get("/audit", res.ListAudit)

				a.Route("/webhooks", func(wh chi.Router) {
					wh.Get("/", res.ListWebhooks)
					wh.Post("/", res.CreateWebhook)
					wh.Delete("/{id}", res.DeleteWebhook)
				})
				a.Route("/suppressions", func(sp chi.Router) {
					sp.Get("/", res.ListSuppressions)
					sp.Post("/", res.AddSuppression)
					sp.Delete("/{id}", res.DeleteSuppression)
				})

				// Horarios recurrentes y privacidad (PII).
				a.Route("/recurring", func(rc chi.Router) {
					rc.Get("/", res.ListRecurring)
					rc.Post("/", res.CreateRecurring)
					rc.Delete("/{id}", res.DeleteRecurring)
				})
				a.Put("/retention", res.SetRetention)
				a.Put("/quota", res.SetQuota)
				a.Post("/users/{userId}/forget", res.ForgetUser)

				// Monitor de cola por aplicación (§8.3).
				a.Route("/tasks", func(tk chi.Router) {
					tk.Get("/", m.ListNotifications)
					tk.Get("/summary", m.TasksSummary)
					tk.Get("/stream", m.TasksStream)
					tk.Post("/{id}/retry", m.RetryTask)
					tk.Post("/{id}/cancel", m.CancelTask)
				})
			})
		})
	})

	return r
}
