// Package metrics define las métricas Prometheus del servicio y el handler para
// exponerlas. Cada proceso (api, worker) tiene su propio registro; Prometheus
// raspa /metrics de cada uno.
//
// Los labels son enums de baja cardinalidad (canal, tipo de proveedor, razón):
// nunca ids de tenant ni texto libre de error.
package metrics

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// sent cuenta notificaciones enviadas con éxito, por canal y proveedor.
	sent = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "qbnotify_notifications_sent_total",
		Help: "Notificaciones enviadas con éxito.",
	}, []string{"channel", "provider"})

	// failed cuenta fallos definitivos, con la razón (send_error, route_error,
	// render_error) — sin ella no se puede alertar por causa.
	failed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "qbnotify_notifications_failed_total",
		Help: "Notificaciones fallidas de forma definitiva.",
	}, []string{"channel", "provider", "reason"})

	// retries cuenta reintentos programados, por razón (send_error vs
	// provider_unavailable: el segundo delata una caída del destino).
	retries = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "qbnotify_retries_total",
		Help: "Reintentos de envío programados.",
	}, []string{"reason"})

	// sendDuration mide solo el despacho (SMTP/proveedor), por canal.
	sendDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "qbnotify_send_duration_seconds",
		Help:    "Latencia del despacho por canal.",
		Buckets: prometheus.DefBuckets,
	}, []string{"channel"})

	// deliveryLatency mide extremo a extremo (created_at → SENT), incluida la
	// espera en cola: es la latencia que sufre el usuario. El label priority
	// permite alertar sobre el p95 de los urgentes (SLA del OTP).
	deliveryLatency = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "qbnotify_delivery_latency_seconds",
		Help:    "Latencia extremo a extremo (creación → enviado).",
		Buckets: []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 300, 1800, 3600},
	}, []string{"channel", "priority"})

	// queueDepth expone el tamaño de cada cola asynq por estado; permite
	// alertar "critical crece" o "hay tareas archivadas" sin entrar a Redis.
	queueDepth = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "qbnotify_queue_depth",
		Help: "Tareas en cada cola asynq, por estado.",
	}, []string{"queue", "state"})
)

// Handler devuelve el handler HTTP de /metrics.
func Handler() http.Handler { return promhttp.Handler() }

// RecordSent registra un envío exitoso: despacho, y extremo a extremo desde la
// creación (age) con su prioridad.
func RecordSent(channel, provider string, dispatch, age time.Duration, priority string) {
	sent.WithLabelValues(channel, provider).Inc()
	sendDuration.WithLabelValues(channel).Observe(dispatch.Seconds())
	deliveryLatency.WithLabelValues(channel, priority).Observe(age.Seconds())
}

// RecordFailed registra un fallo definitivo con su razón.
func RecordFailed(channel, provider, reason string) {
	failed.WithLabelValues(channel, provider, reason).Inc()
}

// RecordRetry registra un reintento programado con su razón.
func RecordRetry(reason string) { retries.WithLabelValues(reason).Inc() }

// SetQueueDepth publica el tamaño de una cola asynq en un estado dado.
func SetQueueDepth(queue, state string, n int) {
	queueDepth.WithLabelValues(queue, state).Set(float64(n))
}
