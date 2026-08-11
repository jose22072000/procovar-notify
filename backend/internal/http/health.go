package http

import (
	"context"
	"net/http"
	"time"

	"dvtech/qbn/internal/httpx"
)

// Pinger es cualquier dependencia que sabe reportar si está sana (DB, Redis...).
// Se define aquí (en el consumidor) para no acoplar el paquete http a pgx/redis.
type Pinger interface {
	Ping(ctx context.Context) error
}

// namedPinger asocia un nombre legible a un Pinger para el reporte de /readyz.
type namedPinger struct {
	name   string
	pinger Pinger
}

// HealthHandler agrupa los chequeos de salud del proceso.
type HealthHandler struct {
	deps    []namedPinger
	timeout time.Duration
}

// NewHealthHandler crea el handler de salud. Las dependencias se registran con
// AddDependency antes de montar las rutas.
func NewHealthHandler() *HealthHandler {
	return &HealthHandler{timeout: 2 * time.Second}
}

// AddDependency registra una dependencia a verificar en /readyz.
func (h *HealthHandler) AddDependency(name string, p Pinger) {
	h.deps = append(h.deps, namedPinger{name: name, pinger: p})
}

// Live responde a /healthz: el proceso está vivo (no comprueba dependencias).
func (h *HealthHandler) Live(w http.ResponseWriter, _ *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Ready responde a /readyz: comprueba cada dependencia y devuelve 503 si alguna
// falla, con el detalle por dependencia.
func (h *HealthHandler) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	services := make(map[string]string, len(h.deps))
	ready := true
	for _, d := range h.deps {
		if err := d.pinger.Ping(ctx); err != nil {
			services[d.name] = "unhealthy"
			ready = false
		} else {
			services[d.name] = "healthy"
		}
	}

	status := http.StatusOK
	overall := "ok"
	if !ready {
		status = http.StatusServiceUnavailable
		overall = "degraded"
	}

	httpx.WriteJSON(w, status, map[string]any{
		"status":   overall,
		"services": services,
	})
}
