// Command api es el servidor HTTP: expone la API pública (notificaciones) y la
// API de administración (consumida por la SPA). En la Fase 0 solo sirve las
// rutas de salud; las de negocio se añaden en fases posteriores.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	dbpkg "dvtech/qbn/db"
	"dvtech/qbn/internal/admin"
	"dvtech/qbn/internal/auth"
	"dvtech/qbn/internal/config"
	"dvtech/qbn/internal/crypto"
	xhttp "dvtech/qbn/internal/http"
	"dvtech/qbn/internal/monitor"
	"dvtech/qbn/internal/notification"
	"dvtech/qbn/internal/observability"
	"dvtech/qbn/internal/queue"
	"dvtech/qbn/internal/quota"
	"dvtech/qbn/internal/seed"
	"dvtech/qbn/internal/store"
	"dvtech/qbn/internal/template"
)

// Límites de rate por API key (sostenido / ráfaga). Valores por defecto
// razonables para tráfico backend; se pueden externalizar a config más adelante.
const (
	rateLimitRPS   = 20
	rateLimitBurst = 40
	// Login admin: por IP, mucho más estricto (anti fuerza bruta). ~10 intentos
	// rápidos y luego 1 cada 5 s sostenido.
	authRateLimitRPS   = 0.2
	authRateLimitBurst = 10
)

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
	logger.Info("starting api", "env", cfg.AppEnv, "port", cfg.APIPort)

	// Contexto de arranque para abrir dependencias.
	bootCtx, cancelBoot := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelBoot()

	db, err := store.NewDB(bootCtx, cfg.DatabaseURL, store.WithMaxConns(int32(cfg.DBMaxConns)), store.WithMinConns(int32(cfg.DBMinConns)))
	if err != nil {
		return fmt.Errorf("conectando a postgres: %w", err)
	}
	defer db.Close()
	logger.Info("postgres connected")

	// Bootstrap: aplica migraciones al arrancar (salvo AUTO_MIGRATE=false) y, si la
	// BD está limpia, siembra los datos iniciales (salvo AUTO_SEED=false).
	if os.Getenv("AUTO_MIGRATE") != "false" {
		if err := dbpkg.Migrate(bootCtx, cfg.DatabaseURL, "up"); err != nil {
			return fmt.Errorf("migraciones: %w", err)
		}
		logger.Info("migraciones aplicadas")
	}
	if os.Getenv("AUTO_SEED") != "false" {
		if err := seedIfEmpty(bootCtx, db, cfg.AppEnv == "development", logger); err != nil {
			return fmt.Errorf("seed inicial: %w", err)
		}
	}

	rds, err := queue.NewRedis(bootCtx, cfg.Redis)
	if err != nil {
		return fmt.Errorf("conectando a redis: %w", err)
	}
	defer func() { _ = rds.Close() }()
	logger.Info("redis connected", "sentinel", cfg.Redis.UseSentinel())

	// Confianza en X-Forwarded-For para la IP de auditoría: solo tras un proxy de
	// confianza (TRUST_PROXY_HEADERS=true); por defecto se ignora (anti-spoofing, L9).
	if os.Getenv("TRUST_PROXY_HEADERS") == "true" {
		xhttp.SetTrustProxy(true)
		logger.Warn("trusting X-Forwarded-For for client IP (TRUST_PROXY_HEADERS=true)")
	}

	// Salud: registra las dependencias que verifica /readyz.
	health := xhttp.NewHealthHandler()
	health.AddDependency("postgres", db)
	health.AddDependency("redis", rds)

	// Componentes de autenticación.
	encryptor, err := crypto.NewKeyring(cfg.PrimaryKeyVersion, cfg.EncryptionKeyring)
	if err != nil {
		return fmt.Errorf("inicializando cifrado: %w", err)
	}
	hmacAuth := auth.NewHMACAuthenticator(db.Q, encryptor, cfg.HMACTimestampSkewSecs).
		WithReplayGuard(auth.NewRedisReplayGuard(rds.Client, cfg.Redis.Prefix))
	rateLimiter := auth.NewRateLimiter(rds.Client, cfg.Redis.Prefix, rateLimitRPS, rateLimitBurst)
	// Limitador por IP para el login admin (estricto, anti fuerza bruta).
	authRateLimiter := auth.NewRateLimiter(rds.Client, cfg.Redis.Prefix, authRateLimitRPS, authRateLimitBurst)
	adminAuth := auth.NewAdminAuthenticator(db.Q, auth.NewTokenIssuer(cfg.AdminJWTSecret))

	// Sesión única de Procovar. Si no está configurado, hub es nil y el
	// middleware se comporta igual que siempre (login propio y nada más).
	hub, err := auth.NewProcovarAuth(
		cfg.ProcovarAuthURL, cfg.ProcovarAuthClientID, cfg.ProcovarAuthSigningKey,
		cfg.ProcovarAuthKeyVersion, cfg.ProcovarSessionCookieName,
	)
	if err != nil {
		// Configuración a medias: se avisa y se sigue con el login propio. Caerse
		// al arrancar dejaría Avisos inaccesible por una variable mal puesta.
		slog.Error("procovar-auth mal configurado: el SSO queda apagado", "error", err)
	}
	if hub != nil {
		adminAuth = adminAuth.ConHub(hub)
		slog.Info("SSO con procovar-auth activo", "url", cfg.ProcovarAuthURL, "cliente", cfg.ProcovarAuthClientID)
	}

	// Cliente de cola + servicio de notificaciones (con cuotas por tenant).
	queueClient := queue.NewClient(cfg.Redis)
	defer func() { _ = queueClient.Close() }()
	quotaSvc := quota.New(rds.Client, cfg.Redis.Prefix)
	notifSvc := notification.NewService(db, queueClient, logger).
		WithQuota(quotaSvc).WithMaxRetries(cfg.QueueRetryMax).WithWebhooks(queueClient)
	templateSvc := template.NewService(db, logger)
	adminSvc := admin.NewService(db, encryptor, logger)
	monitorSvc := monitor.NewService(db, queueClient, logger)

	router := xhttp.NewRouter(xhttp.RouterDeps{
		Logger:             logger,
		Health:             health,
		HMAC:               hmacAuth,
		RateLimiter:        rateLimiter,
		AuthRateLimiter:    authRateLimiter,
		AdminAuth:          adminAuth,
		Notification:       xhttp.NewNotificationHandler(notifSvc),
		AdminTemplate:      xhttp.NewAdminTemplateHandler(templateSvc),
		AdminResource:      xhttp.NewAdminResourceHandler(adminSvc),
		AdminMonitor:       xhttp.NewAdminMonitorHandler(monitorSvc, notifSvc),
		CORSAllowedOrigins: cfg.CORSAllowedOrigins,
		RefreshCookie: xhttp.CookieConfig{
			Name:     cfg.CookieRefreshName,
			Domain:   cfg.CookieDomain,
			Secure:   cfg.CookieSecure,
			SameSite: cfg.CookieSameSite,
		},
	})

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.APIPort),
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Arranca el servidor en segundo plano.
	serverErr := make(chan error, 1)
	go func() {
		logger.Info("http server listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	// Espera señal de apagado o un error del servidor.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		return fmt.Errorf("servidor http: %w", err)
	case sig := <-stop:
		logger.Info("shutdown signal received", "signal", sig.String())
	}

	// Apagado ordenado: deja de aceptar peticiones y drena las en curso.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("apagado del servidor http: %w", err)
	}
	logger.Info("api stopped cleanly")
	return nil
}

// seedIfEmpty siembra los datos iniciales solo si la BD está "limpia" (sin admins).
// Fuera de desarrollo requiere SEED_ADMIN_PASSWORD; si falta, avisa y no siembra
// (la app arranca igual, pero no habrá con qué entrar hasta sembrar).
func seedIfEmpty(ctx context.Context, db *store.DB, isDev bool, logger *slog.Logger) error {
	var admins int
	if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM admin_users`).Scan(&admins); err != nil {
		return err
	}
	if admins > 0 {
		return nil // ya inicializada
	}

	adminEmail := envOr("SEED_ADMIN_EMAIL", "admin@qbnotify.local")
	adminPassword := os.Getenv("SEED_ADMIN_PASSWORD")
	if adminPassword == "" {
		if !isDev {
			logger.Warn("BD sin inicializar pero SEED_ADMIN_PASSWORD ausente: no se siembra (define SEED_ADMIN_PASSWORD y reinicia)")
			return nil
		}
		adminPassword = "changeme-admin"
	}

	logger.Info("BD limpia: sembrando datos iniciales", "admin", adminEmail)
	return seed.Run(ctx, db.Pool, seed.Options{
		AdminEmail:    adminEmail,
		AdminPassword: adminPassword,
		IncludeDemo:   isDev, // la app demo solo en desarrollo
	})
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
