package db

import (
	"context"
	"database/sql"

	_ "github.com/jackc/pgx/v5/stdlib" // driver "pgx" para database/sql (goose)
	"github.com/pressly/goose/v3"
)

const migrationsDir = "migrations"

// Migrate ejecuta un comando de goose (up|down|status|version|reset|…) sobre la
// base de datos indicada, usando las migraciones embebidas. Abre y cierra su
// propia conexión database/sql (goose la necesita), separada del pool pgx.
// Lo usan tanto cmd/migrate como el arranque del api (migración automática).
func Migrate(ctx context.Context, databaseURL, command string, args ...string) error {
	sqlDB, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return err
	}
	defer func() { _ = sqlDB.Close() }()

	goose.SetBaseFS(Migrations)
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	return goose.RunContext(ctx, command, sqlDB, migrationsDir, args...)
}
