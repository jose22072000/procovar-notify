// Package queue gestiona el broker de colas (asynq sobre Redis). En la Fase 3 se
// añaden el cliente/servidor asynq, los tipos de tarea y las colas ponderadas;
// aquí, por ahora, solo construye la conexión Redis (Sentinel o nodo único) y
// expone un chequeo de salud.
package queue

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/redis/go-redis/v9"

	"dvtech/qbn/internal/config"
)

// Redis envuelve el cliente de go-redis usado para salud y, más adelante, para
// rate limiting y locks. asynq mantiene su propia conexión derivada de la misma
// configuración (ver Fase 3).
type Redis struct {
	Client redis.UniversalClient
}

// newUniversalClient construye un cliente go-redis (Sentinel o nodo único) para
// la DB dada, endurecido contra redes que cortan conexiones ociosas en silencio
// (NAT/conntrack en Docker y similares): keepalive TCP frecuente para mantener
// vivas las conexiones y detectar pares muertos, y reciclado de conexiones del
// pool que lleven demasiado tiempo ociosas (antes de que la red las mate).
func newUniversalClient(cfg config.RedisConfig, db int) redis.UniversalClient {
	dialer := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	dial := func(ctx context.Context, network, addr string) (net.Conn, error) {
		return dialer.DialContext(ctx, network, addr)
	}
	const maxIdle = 4 * time.Minute
	if cfg.UseSentinel() {
		return redis.NewFailoverClient(&redis.FailoverOptions{
			MasterName:       cfg.MasterName,
			SentinelAddrs:    cfg.Sentinels,
			Password:         cfg.Password,
			SentinelPassword: cfg.SentinelPass,
			DB:               db,
			Dialer:           dial,
			ConnMaxIdleTime:  maxIdle,
		})
	}
	return redis.NewClient(&redis.Options{
		Addr:            cfg.Addr,
		Password:        cfg.Password,
		DB:              db,
		Dialer:          dial,
		ConnMaxIdleTime: maxIdle,
	})
}

// NewRedis construye el cliente apropiado según la configuración: modo Sentinel
// (failover, alta disponibilidad) si hay sentinels, o nodo único en su defecto.
func NewRedis(ctx context.Context, cfg config.RedisConfig) (*Redis, error) {
	client := newUniversalClient(cfg, cfg.DBDefault)

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("ping inicial a redis: %w", err)
	}
	return &Redis{Client: client}, nil
}

// Ping implementa la verificación de salud usada por /readyz.
func (r *Redis) Ping(ctx context.Context) error {
	return r.Client.Ping(ctx).Err()
}

// Close cierra la conexión.
func (r *Redis) Close() error {
	return r.Client.Close()
}
