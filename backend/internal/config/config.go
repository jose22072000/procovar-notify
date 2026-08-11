// Package config carga y valida la configuración del servicio siguiendo el
// principio 12-factor (todo por variables de entorno). La validación ocurre al
// arranque: si algo crítico falta o es inválido, el proceso falla rápido y con
// un mensaje claro en lugar de romperse más tarde en runtime.
package config

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/spf13/viper"
)

// Config es la configuración completa del proceso (compartida por api y worker;
// cada binario usa el subconjunto que necesita).
type Config struct {
	// Servidor
	AppEnv            string // "production" | "development" | ...
	Debug             bool
	APIPort           int
	MetricsPort       int
	WorkerConcurrency int

	// PostgreSQL
	DatabaseURL string

	// Redis / colas
	Redis RedisConfig

	// Cripto / auth
	SecretEncryptionKey []byte          // clave primaria (32 bytes), decodificada de base64
	PrimaryKeyVersion   byte            // versión de la clave primaria (cifrado nuevo)
	EncryptionKeyring   map[byte][]byte // todas las versiones de clave para descifrar (incluye la primaria)
	AdminJWTSecret      string

	// Defaults de cola y auth
	QueueRetryMax         int
	HMACTimestampSkewSecs int
	// QueueStrictPriority (default true): critical se atiende siempre antes
	// que default/low; false = reparto ponderado 6/3/1.
	QueueStrictPriority bool
	// QueueShutdownTimeoutSecs (default 30): drenado máximo del worker al
	// apagar; debe superar al canal más lento (SMTP 15 s) para no cortar
	// envíos en vuelo en cada deploy.
	QueueShutdownTimeoutSecs int

	// Pool de Postgres. Sin fijarlo, pgx usa max(4, nCPU): pocas conexiones
	// para WORKER_CONCURRENCY goroutines y serializa los envíos bajo carga.
	DBMaxConns int // default 20 (>= concurrencia del worker + tareas de fondo)
	DBMinConns int // default 2 (conexiones calientes)

	// CORS (admin API) — orígenes permitidos para que el SPA, servido en un
	// origen distinto (sin proxy delante), pueda llamar a /admin desde el
	// navegador. Vacío = CORS deshabilitado (comportamiento previo).
	CORSAllowedOrigins []string

	// Cookie del refresh token (HttpOnly). Sus atributos dependen de la topología
	// de despliegue: mismo dominio → SameSite=Lax basta; dominios distintos →
	// SameSite=None + Secure (y HTTPS). En dev (proxy de Vite, mismo origen sobre
	// http) los defaults Lax + Secure=false funcionan.
	CookieRefreshName string
	CookieDomain      string
	CookieSecure      bool
	CookieSameSite    http.SameSite
}

// RedisConfig describe la conexión a Redis. Si Sentinels tiene entradas se usa
// modo Sentinel (alta disponibilidad, §6.3 del diseño); en caso contrario se
// usa Addr como nodo único (útil en local/tests).
type RedisConfig struct {
	Sentinels    []string // ["host:26379", ...]
	MasterName   string
	Addr         string // nodo único (fallback cuando no hay sentinels)
	Password     string
	SentinelPass string
	DBDefault    int
	DBLocks      int
	DBAsynq      int
	Prefix       string
}

// UseSentinel indica si la configuración debe usar el cliente de failover.
func (r RedisConfig) UseSentinel() bool { return len(r.Sentinels) > 0 }

// IsProduction es un atajo usado para decidir el formato de logs, etc.
func (c Config) IsProduction() bool { return c.AppEnv == "production" }

// Load lee la configuración del entorno, aplica defaults y la valida.
func Load() (*Config, error) {
	v := viper.New()
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Defaults (§10 del diseño).
	v.SetDefault("APP_ENV", "development")
	v.SetDefault("DEBUG", false)
	v.SetDefault("API_PORT", 8080)
	v.SetDefault("METRICS_PORT", 9091)
	v.SetDefault("WORKER_CONCURRENCY", 10)
	v.SetDefault("REDIS_MASTER_NAME", "master")
	v.SetDefault("REDIS_DB_DEFAULT", 4)
	v.SetDefault("REDIS_DB_LOCKS", 5)
	v.SetDefault("REDIS_DB_ASYNQ", 10)
	v.SetDefault("REDIS_PREFIX", "procovar-notify")
	v.SetDefault("QUEUE_RETRY_MAX", 3)
	v.SetDefault("QUEUE_STRICT_PRIORITY", true)
	v.SetDefault("QUEUE_SHUTDOWN_TIMEOUT_SECONDS", 30)
	v.SetDefault("DB_MAX_CONNS", 20)
	v.SetDefault("DB_MIN_CONNS", 2)
	v.SetDefault("HMAC_TIMESTAMP_SKEW_SECONDS", 300)
	v.SetDefault("COOKIE_REFRESH_NAME", "qbn_refresh")
	v.SetDefault("COOKIE_SAMESITE", "lax")
	v.SetDefault("COOKIE_SECURE", false)

	cfg := &Config{
		AppEnv:                   v.GetString("APP_ENV"),
		Debug:                    v.GetBool("DEBUG"),
		APIPort:                  v.GetInt("API_PORT"),
		MetricsPort:              v.GetInt("METRICS_PORT"),
		WorkerConcurrency:        v.GetInt("WORKER_CONCURRENCY"),
		DatabaseURL:              v.GetString("DATABASE_URL"),
		AdminJWTSecret:           v.GetString("ADMIN_JWT_SECRET"),
		QueueRetryMax:            v.GetInt("QUEUE_RETRY_MAX"),
		QueueStrictPriority:      v.GetBool("QUEUE_STRICT_PRIORITY"),
		QueueShutdownTimeoutSecs: v.GetInt("QUEUE_SHUTDOWN_TIMEOUT_SECONDS"),
		DBMaxConns:               v.GetInt("DB_MAX_CONNS"),
		DBMinConns:               v.GetInt("DB_MIN_CONNS"),
		HMACTimestampSkewSecs:    v.GetInt("HMAC_TIMESTAMP_SKEW_SECONDS"),
		CORSAllowedOrigins:       splitAndTrim(v.GetString("CORS_ALLOWED_ORIGINS")),
		CookieRefreshName:        v.GetString("COOKIE_REFRESH_NAME"),
		CookieDomain:             v.GetString("COOKIE_DOMAIN"),
		CookieSecure:             v.GetBool("COOKIE_SECURE"),
		CookieSameSite:           parseSameSite(v.GetString("COOKIE_SAMESITE")),
		Redis: RedisConfig{
			Sentinels:    splitAndTrim(v.GetString("REDIS_SENTINELS")),
			MasterName:   v.GetString("REDIS_MASTER_NAME"),
			Addr:         v.GetString("REDIS_ADDR"),
			Password:     v.GetString("REDIS_PASSWORD"),
			SentinelPass: v.GetString("REDIS_SENTINEL_PASSWORD"),
			DBDefault:    v.GetInt("REDIS_DB_DEFAULT"),
			DBLocks:      v.GetInt("REDIS_DB_LOCKS"),
			DBAsynq:      v.GetInt("REDIS_DB_ASYNQ"),
			Prefix:       v.GetString("REDIS_PREFIX"),
		},
	}

	key, err := decodeEncryptionKey(v.GetString("SECRET_ENCRYPTION_KEY"))
	if err != nil {
		return nil, err
	}
	cfg.SecretEncryptionKey = key

	// Keyring de cifrado (rotación KMS). La clave primaria cifra; las versiones
	// antiguas (SECRET_ENCRYPTION_KEYS) solo se usan para descifrar datos previos.
	verInt := v.GetInt("SECRET_ENCRYPTION_KEY_VERSION")
	if verInt < 0 || verInt > 255 {
		// Sin esto, byte(256) truncaría a 0 y caería al default 1, descifrando
		// con la clave equivocada en silencio.
		return nil, fmt.Errorf("SECRET_ENCRYPTION_KEY_VERSION debe estar en 1..255, got %d", verInt)
	}
	primary := byte(verInt)
	if primary == 0 {
		primary = 1
	}
	cfg.PrimaryKeyVersion = primary
	cfg.EncryptionKeyring = map[byte][]byte{primary: key}
	for _, entry := range splitAndTrim(v.GetString("SECRET_ENCRYPTION_KEYS")) {
		ver, oldKey, err := parseVersionedKey(entry)
		if err != nil {
			return nil, err
		}
		cfg.EncryptionKeyring[ver] = oldKey
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// parseSameSite mapea el valor de COOKIE_SAMESITE al enum de net/http. Un valor
// desconocido cae a Lax (seguro por defecto); la combinación None+!Secure la
// rechaza validate() (los navegadores descartan SameSite=None sin Secure).
func parseSameSite(s string) http.SameSite {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "strict":
		return http.SameSiteStrictMode
	case "none":
		return http.SameSiteNoneMode
	default:
		return http.SameSiteLaxMode
	}
}

// parseVersionedKey parsea "<versión>:<clave-base64>" del keyring de rotación.
func parseVersionedKey(entry string) (byte, []byte, error) {
	parts := strings.SplitN(entry, ":", 2)
	if len(parts) != 2 {
		return 0, nil, fmt.Errorf("SECRET_ENCRYPTION_KEYS: formato inválido %q (use version:base64)", entry)
	}
	ver, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil || ver <= 0 || ver > 255 {
		return 0, nil, fmt.Errorf("SECRET_ENCRYPTION_KEYS: versión inválida en %q", entry)
	}
	rawKey, err := base64.StdEncoding.DecodeString(strings.TrimSpace(parts[1]))
	if err != nil || len(rawKey) != 32 {
		return 0, nil, fmt.Errorf("SECRET_ENCRYPTION_KEYS: clave inválida en %q (32 bytes base64)", entry)
	}
	return byte(ver), rawKey, nil
}

// validate acumula y reporta todos los problemas de configuración a la vez.
func (c *Config) validate() error {
	var problems []string

	if c.DatabaseURL == "" {
		problems = append(problems, "DATABASE_URL es obligatorio")
	}
	if len(c.AdminJWTSecret) < 32 {
		problems = append(problems, "ADMIN_JWT_SECRET es obligatorio y debe tener al menos 32 caracteres (256 bits)")
	}
	if !c.Redis.UseSentinel() && c.Redis.Addr == "" {
		problems = append(problems, "define REDIS_SENTINELS (modo HA) o REDIS_ADDR (nodo único)")
	}
	if c.Redis.UseSentinel() && c.Redis.MasterName == "" {
		problems = append(problems, "REDIS_MASTER_NAME es obligatorio en modo Sentinel")
	}
	if c.APIPort <= 0 || c.APIPort > 65535 {
		problems = append(problems, fmt.Sprintf("API_PORT inválido: %d", c.APIPort))
	}
	// Los navegadores descartan una cookie SameSite=None si no es Secure: en ese
	// caso el refresh no viajaría y la sesión se caería en silencio. Falla rápido.
	if c.CookieSameSite == http.SameSiteNoneMode && !c.CookieSecure {
		problems = append(problems, "COOKIE_SAMESITE=none requiere COOKIE_SECURE=true (y HTTPS)")
	}

	if len(problems) > 0 {
		return fmt.Errorf("configuración inválida:\n  - %s", strings.Join(problems, "\n  - "))
	}
	return nil
}

// decodeEncryptionKey valida que la clave de cifrado sea base64 de 32 bytes
// (AES-256). Es obligatoria porque los secretos SMTP/proveedor/API key se cifran
// en reposo con ella.
func decodeEncryptionKey(raw string) ([]byte, error) {
	if raw == "" {
		return nil, fmt.Errorf("SECRET_ENCRYPTION_KEY es obligatorio (32 bytes en base64)")
	}
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("SECRET_ENCRYPTION_KEY no es base64 válido: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("SECRET_ENCRYPTION_KEY debe decodificar a 32 bytes, son %d", len(key))
	}
	return key, nil
}

func splitAndTrim(csv string) []string {
	if csv == "" {
		return nil
	}
	parts := strings.Split(csv, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
