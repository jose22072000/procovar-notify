// Command migrate ejecuta las migraciones de base de datos (goose) embebidas en
// el binario. Lee DATABASE_URL del entorno directamente (no necesita la config
// completa del servicio) para poder correr en arranque/CI de forma aislada.
//
// Uso:
//
//	migrate up            aplica todas las migraciones pendientes
//	migrate down          revierte la última migración
//	migrate status        muestra el estado de las migraciones
//	migrate version       muestra la versión actual
//	migrate reset         revierte todas las migraciones
package main

import (
	"context"
	"fmt"
	"os"

	"dvtech/qbn/db"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "migrate: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	args := os.Args[1:]
	if len(args) == 0 {
		return fmt.Errorf("falta el comando (up|down|status|version|reset)")
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return fmt.Errorf("DATABASE_URL es obligatorio")
	}

	if err := db.Migrate(context.Background(), databaseURL, args[0], args[1:]...); err != nil {
		return fmt.Errorf("goose %s: %w", args[0], err)
	}
	return nil
}
