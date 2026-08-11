// Package store es la capa de acceso a datos sobre PostgreSQL (pgx + sqlc).
//
// El aislamiento por tenant se garantiza en el diseño de las queries: toda
// consulta sobre recursos de un tenant recibe application_id como parámetro
// obligatorio (ver db/queries/*.sql). El test de aislamiento cruzado
// (postgres_test.go) verifica esta propiedad y debe permanecer en verde.
package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	pgxuuid "github.com/vgarvardt/pgx-google-uuid/v5"

	"dvtech/qbn/internal/store/sqlc"
)

// DB envuelve el pool de conexiones pgx y las queries generadas por sqlc.
type DB struct {
	Pool *pgxpool.Pool
	Q    *sqlc.Queries
}

// PoolOption ajusta la configuración del pool antes de abrirlo.
type PoolOption func(*pgxpool.Config)

// WithMaxConns fija el tope de conexiones del pool. Sin él, pgx usa
// max(4, nCPU) — insuficiente para el worker (10 goroutines de envío +
// reaper/retención) y causa serialización silenciosa bajo carga.
func WithMaxConns(n int32) PoolOption {
	return func(c *pgxpool.Config) {
		if n > 0 {
			c.MaxConns = n
		}
	}
}

// WithMinConns mantiene conexiones calientes (evita el coste de abrir en frío
// tras periodos de inactividad).
func WithMinConns(n int32) PoolOption {
	return func(c *pgxpool.Config) {
		if n > 0 {
			c.MinConns = n
		}
	}
}

// NewDB abre el pool contra la DATABASE_URL dada, registra el códec de
// github.com/google/uuid (necesario para que pgx serialice los uuid.UUID que
// usa el código generado) y verifica la conexión.
func NewDB(ctx context.Context, databaseURL string, opts ...PoolOption) (*DB, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parseando DATABASE_URL: %w", err)
	}
	cfg.AfterConnect = func(_ context.Context, conn *pgx.Conn) error {
		pgxuuid.Register(conn.TypeMap())
		return nil
	}
	for _, opt := range opts {
		opt(cfg)
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("creando pool pgx: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping inicial a postgres: %w", err)
	}

	return &DB{Pool: pool, Q: sqlc.New(pool)}, nil
}

// Tx ejecuta fn dentro de una transacción. Si fn devuelve error (o hay panic),
// se hace rollback; si no, commit. Las queries dentro de fn usan el *sqlc.Queries
// ligado a la transacción.
//
// Regla del proyecto: el commit ocurre ANTES de encolar en asynq (nunca al
// revés), para no encolar un job que referencia una fila que podría revertirse.
func (d *DB) Tx(ctx context.Context, fn func(q *sqlc.Queries) error) error {
	tx, err := d.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("iniciando transacción: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op si ya se hizo commit

	if err := fn(d.Q.WithTx(tx)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// Ping implementa la verificación de salud usada por /readyz.
func (d *DB) Ping(ctx context.Context) error {
	return d.Pool.Ping(ctx)
}

// Close libera el pool de conexiones.
func (d *DB) Close() {
	if d.Pool != nil {
		d.Pool.Close()
	}
}
