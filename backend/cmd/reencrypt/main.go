// Command reencrypt re-cifra en sitio todos los secretos en reposo con la clave
// primaria actual del keyring (rotación KMS, Fase 21). Es idempotente: salta las
// filas ya cifradas con la versión primaria. Sin downtime: api/worker pueden
// seguir corriendo con el keyring (descifran cualquier versión) mientras esto
// migra los datos a la versión nueva.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"dvtech/qbn/internal/config"
	"dvtech/qbn/internal/crypto"
	"dvtech/qbn/internal/keyrotation"
	"dvtech/qbn/internal/store"
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
	enc, err := crypto.NewKeyring(cfg.PrimaryKeyVersion, cfg.EncryptionKeyring)
	if err != nil {
		return fmt.Errorf("keyring: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	db, err := store.NewDB(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("postgres: %w", err)
	}
	defer db.Close()

	fmt.Printf("re-cifrando a la versión de clave primaria %d\n", cfg.PrimaryKeyVersion)
	results, err := keyrotation.Reencrypt(ctx, db, enc)
	if err != nil {
		return err
	}
	total := 0
	for _, r := range results {
		fmt.Printf("  %-20s %s: %d fila(s) re-cifradas\n", r.Table, r.Column, r.Updated)
		total += r.Updated
	}
	fmt.Printf("listo: %d secreto(s) migrados a la versión %d\n", total, cfg.PrimaryKeyVersion)
	return nil
}
