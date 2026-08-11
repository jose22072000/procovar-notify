// Package storetest provee un harness reutilizable para tests de integración:
// levanta un PostgreSQL efímero (testcontainers), le aplica las migraciones
// goose y devuelve un *store.DB listo para usar. Se reutiliza en todas las fases.
//
// Requiere Docker. Para evitar descargas innecesarias se desactiva Ryuk
// (TESTCONTAINERS_RYUK_DISABLED) y la limpieza se hace con t.Cleanup.
package storetest

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // driver database/sql (pgx) para goose
	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"dvtech/qbn/db"
	"dvtech/qbn/internal/store"
)

// NewPostgres levanta un Postgres efímero, migra el esquema y devuelve el *DB.
// El contenedor se destruye automáticamente al terminar el test.
func NewPostgres(t *testing.T) *store.DB {
	t.Helper()
	t.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")
	ctx := context.Background()

	container, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("qbnotify"),
		postgres.WithUsername("qbnotify"),
		postgres.WithPassword("qbnotify"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("levantando contenedor postgres: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(ctx) })

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}

	migrate(t, dsn)

	d, err := store.NewDB(ctx, dsn)
	if err != nil {
		t.Fatalf("abriendo store: %v", err)
	}
	t.Cleanup(d.Close)
	return d
}

// migrate aplica todas las migraciones goose embebidas sobre el contenedor.
func migrate(t *testing.T, dsn string) {
	t.Helper()
	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer func() { _ = sqlDB.Close() }()

	goose.SetBaseFS(db.Migrations)
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatalf("goose dialect: %v", err)
	}
	goose.SetLogger(goose.NopLogger())
	if err := goose.Up(sqlDB, "migrations"); err != nil {
		t.Fatalf("goose up: %v", err)
	}
}
