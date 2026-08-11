// Command seed siembra los datos iniciales (delegando en internal/seed): librería
// de plantillas base, super-admin y, en este comando, también la app demo.
// Es idempotente. Lee DATABASE_URL y las SEED_* del entorno.
package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"dvtech/qbn/internal/crypto"
	"dvtech/qbn/internal/seed"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "seed: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return fmt.Errorf("DATABASE_URL es obligatorio")
	}
	isDev := os.Getenv("APP_ENV") == "development"

	adminEmail := envOr("SEED_ADMIN_EMAIL", "admin@qbnotify.local")
	// Fuera de desarrollo NO se permiten credenciales por defecto conocidas.
	adminPassword := os.Getenv("SEED_ADMIN_PASSWORD")
	if adminPassword == "" {
		if !isDev {
			return fmt.Errorf("SEED_ADMIN_PASSWORD es obligatorio fuera de desarrollo (define APP_ENV=development para usar el valor por defecto)")
		}
		adminPassword = "changeme-admin"
	}

	enc, hasEnc := encryptorFromEnv()
	apiKeySecret := os.Getenv("SEED_API_KEY_SECRET")
	if hasEnc && apiKeySecret == "" {
		if !isDev {
			return fmt.Errorf("SEED_API_KEY_SECRET es obligatorio fuera de desarrollo")
		}
		apiKeySecret = "demo-secret-please-change"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("conectando: %w", err)
	}
	defer pool.Close()

	if err := seed.Run(ctx, pool, seed.Options{
		AdminEmail:    adminEmail,
		AdminPassword: adminPassword,
		IncludeDemo:   true, // el comando seed crea también la app demo
		Encryptor:     enc,  // nil si no hay clave → se omite la mensajería demo
		APIKeySecret:  apiKeySecret,
		SMTPHost:      envOr("SEED_SMTP_HOST", "localhost"),
	}); err != nil {
		return err
	}

	fmt.Println("seed: OK")
	fmt.Printf("  super-admin: %s\n", adminEmail)
	if hasEnc {
		fmt.Printf("  api key demo: demo-key / secret: %s\n", apiKeySecret)
	} else {
		fmt.Println("  (SECRET_ENCRYPTION_KEY ausente: se omiten SMTP/ruta/API key del demo)")
	}
	return nil
}

// encryptorFromEnv construye el cifrador desde SECRET_ENCRYPTION_KEY si es válido.
func encryptorFromEnv() (*crypto.Encryptor, bool) {
	raw := os.Getenv("SECRET_ENCRYPTION_KEY")
	if raw == "" {
		return nil, false
	}
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil || len(key) != 32 {
		return nil, false
	}
	enc, err := crypto.NewEncryptor(key)
	if err != nil {
		return nil, false
	}
	return enc, true
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
