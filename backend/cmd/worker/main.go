// Command worker consume tareas de la cola (asynq) y ejecuta el envío por canal.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"dvtech/qbn/internal/channels"
	"dvtech/qbn/internal/config"
	"dvtech/qbn/internal/crypto"
	"dvtech/qbn/internal/metrics"
	"dvtech/qbn/internal/notification"
	"dvtech/qbn/internal/observability"
	"dvtech/qbn/internal/queue"
	"dvtech/qbn/internal/recurring"
	"dvtech/qbn/internal/retention"
	"dvtech/qbn/internal/safedial"
	"dvtech/qbn/internal/store"
	"dvtech/qbn/internal/webhook"
)

// retentionInterval es cada cuánto el worker anonimiza PII vencida.
const retentionInterval = time.Hour

// retentionTimeout acota cada pasada de purga para que no quede colgada.
const retentionTimeout = 5 * time.Minute

// reaperInterval: cadencia del barrido de notificaciones huérfanas (corto:
// un OTP atascado debe rescatarse en minutos).
const reaperInterval = time.Minute

// reaperTimeout acota cada pasada del reaper.
const reaperTimeout = 30 * time.Second

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger := observability.NewLogger(cfg.IsProduction(), cfg.Debug)
	logger.Info("starting worker", "env", cfg.AppEnv, "concurrency", cfg.WorkerConcurrency)

	bootCtx, cancelBoot := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelBoot()

	db, err := store.NewDB(bootCtx, cfg.DatabaseURL, store.WithMaxConns(int32(cfg.DBMaxConns)), store.WithMinConns(int32(cfg.DBMinConns)))
	if err != nil {
		return fmt.Errorf("conectando a postgres: %w", err)
	}
	defer db.Close()
	logger.Info("postgres connected")

	encryptor, err := crypto.NewKeyring(cfg.PrimaryKeyVersion, cfg.EncryptionKeyring)
	if err != nil {
		return fmt.Errorf("inicializando cifrado: %w", err)
	}

	// Guard anti-SSRF en adjuntos por URL: por defecto se bloquean IPs
	// privadas/loopback. ATTACHMENT_ALLOW_PRIVATE=true lo desactiva (entornos
	// cerrados donde los adjuntos vivan en hosts internos de confianza).
	if os.Getenv("ATTACHMENT_ALLOW_PRIVATE") == "true" {
		notification.SetAllowPrivateHosts(true)
		logger.Warn("attachment SSRF guard disabled (ATTACHMENT_ALLOW_PRIVATE=true)")
	}

	// Allowlist opcional de hosts para adjuntos por URL (defensa en profundidad
	// sobre el guard por IP). ATTACHMENT_HOST_ALLOWLIST="cdn.foo.com,assets.bar.com".
	if hosts := os.Getenv("ATTACHMENT_HOST_ALLOWLIST"); strings.TrimSpace(hosts) != "" {
		list := strings.Split(hosts, ",")
		notification.SetHostAllowlist(list)
		logger.Info("attachment host allowlist enabled", "count", len(list))
	}

	// Guard anti-SSRF en la entrega de webhooks: por defecto se bloquean destinos
	// internos. WEBHOOK_ALLOW_PRIVATE=true lo desactiva (self-hosted con el backend
	// del cliente en la misma red privada).
	if os.Getenv("WEBHOOK_ALLOW_PRIVATE") == "true" {
		safedial.SetAllowPrivate(true)
		logger.Warn("webhook SSRF guard disabled (WEBHOOK_ALLOW_PRIVATE=true)")
	}

	// Dispatcher con todos los canales: Email (SMTP), In-App, Push (FCM) y SMS (Twilio).
	dispatcher := channels.NewDispatcher(
		channels.NewEmailSender(),
		channels.NewInAppSender(),
		channels.NewPushSender(),
		channels.NewSMSSender(),
	)
	// Cliente de cola + servicio de notificaciones (lo usan las recurrentes para
	// crear y encolar una notificación en cada disparo).
	queueClient := queue.NewClient(cfg.Redis)
	defer func() { _ = queueClient.Close() }()
	notifSvc := notification.NewService(db, queueClient, logger).WithMaxRetries(cfg.QueueRetryMax)
	recurringSvc := recurring.NewService(db, notifSvc, logger)

	// El processor emite webhooks de estado (sent/failed) encolándolos.
	processor := notification.NewProcessor(db, dispatcher, encryptor, logger).WithWebhooks(queueClient)
	webhookSvc := webhook.NewService(db, encryptor, logger)

	srv := queue.NewServer(cfg, logger)
	mux := queue.NewMux(processor)
	queue.RegisterRecurring(mux, recurringSvc.Process)
	queue.RegisterWebhook(mux, webhookSvc.Deliver)

	if err := srv.Start(mux); err != nil {
		return fmt.Errorf("arrancando asynq server: %w", err)
	}
	logger.Info("worker ready, consuming tasks")

	// PeriodicTaskManager: registra las entradas cron de los horarios recurrentes.
	periodicMgr, err := queue.NewPeriodicManager(cfg.Redis, recurring.NewConfigProvider(db))
	if err != nil {
		return fmt.Errorf("creando periodic manager: %w", err)
	}
	go func() {
		if err := periodicMgr.Run(); err != nil {
			logger.Error("periodic manager error", "error", err)
		}
	}()

	// Anonimización de PII vencida (retención §11.1) en un ticker.
	retentionSvc := retention.NewService(db, logger)
	stopRetention := make(chan struct{})
	// retentionCtx se cancela al apagar, abortando una purga en curso; cada pasada
	// además se acota con retentionTimeout.
	retentionCtx, cancelRetention := context.WithCancel(context.Background())
	go func() {
		ticker := time.NewTicker(retentionInterval)
		defer ticker.Stop()
		purge := func() {
			ctx, cancel := context.WithTimeout(retentionCtx, retentionTimeout)
			defer cancel()
			if _, err := retentionSvc.PurgeExpired(ctx); err != nil {
				logger.Error("pii purge error", "error", err)
			}
		}
		purge()
		for {
			select {
			case <-stopRetention:
				return
			case <-ticker.C:
				purge()
			}
		}
	}()

	// Reaper de notificaciones huérfanas (commit sin encolar): reencolar es
	// idempotente por TaskID, así que barrer de más nunca duplica.
	stopReaper := make(chan struct{})
	reaperCtx, cancelReaper := context.WithCancel(context.Background())
	go func() {
		ticker := time.NewTicker(reaperInterval)
		defer ticker.Stop()
		sweep := func() {
			ctx, cancel := context.WithTimeout(reaperCtx, reaperTimeout)
			defer cancel()
			if _, err := notifSvc.RequeueStale(ctx, notification.DefaultReapThreshold, notification.DefaultReapBatch); err != nil {
				logger.Error("reaper sweep error", "error", err)
			}
			if _, _, err := notifSvc.ReconcileStuckProcessing(ctx, notification.DefaultReapThreshold, notification.DefaultReapBatch); err != nil {
				logger.Error("reaper reconcile error", "error", err)
			}
		}
		sweep()
		for {
			select {
			case <-stopReaper:
				return
			case <-ticker.C:
				sweep()
			}
		}
	}()

	// Profundidad de colas → gauges Prometheus (cada 30 s).
	inspector := queue.NewInspector(cfg.Redis)
	stopDepth := make(chan struct{})
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		poll := func() { queue.PollQueueDepth(inspector, metrics.SetQueueDepth) }
		poll()
		for {
			select {
			case <-stopDepth:
				return
			case <-ticker.C:
				poll()
			}
		}
	}()

	// Servidor de métricas (Prometheus raspa este proceso aparte de la api).
	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", metrics.Handler())
	metricsMux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	metricsSrv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.MetricsPort),
		Handler:           metricsMux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		if err := metricsSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("metrics server error", "error", err)
		}
	}()
	logger.Info("metrics server listening", "port", cfg.MetricsPort)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	sig := <-stop
	logger.Info("shutdown signal received", "signal", sig.String())

	// Shutdown ordenado: parar loops de fondo y drenar tareas en curso.
	// (periodicMgr.Run() gestiona su propio apagado al recibir la señal, así que
	// no se le llama Shutdown() aquí para no cerrar dos veces su canal.)
	close(stopRetention)
	cancelRetention() // aborta una purga de retención en curso
	close(stopReaper)
	cancelReaper() // aborta una pasada del reaper en curso
	close(stopDepth)
	srv.Shutdown()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = metricsSrv.Shutdown(shutdownCtx)
	logger.Info("worker stopped cleanly")
	return nil
}
