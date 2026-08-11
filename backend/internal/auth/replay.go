package auth

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// ReplayGuard registra las firmas ya vistas para impedir reenvíos (replay)
// dentro de la ventana anti-replay. FirstSeen devuelve true la primera vez que
// ve un token y false si ya estaba registrado.
type ReplayGuard interface {
	FirstSeen(ctx context.Context, token string, ttl time.Duration) (bool, error)
}

// RedisReplayGuard implementa ReplayGuard con SETNX + TTL en Redis: la primera
// petición registra la firma con expiración igual a la ventana; un reenvío con
// la misma firma encuentra la clave y se rechaza. La firma HMAC ya es única por
// petición (cubre method, path, query, body y timestamp), así que sirve de
// nonce sin necesidad de un campo aparte.
type RedisReplayGuard struct {
	rdb    redis.UniversalClient
	prefix string
}

// NewRedisReplayGuard crea el guard sobre el cliente Redis dado.
func NewRedisReplayGuard(rdb redis.UniversalClient, prefix string) *RedisReplayGuard {
	return &RedisReplayGuard{rdb: rdb, prefix: prefix}
}

// FirstSeen registra el token (SETNX) y devuelve true si no existía.
func (g *RedisReplayGuard) FirstSeen(ctx context.Context, token string, ttl time.Duration) (bool, error) {
	return g.rdb.SetNX(ctx, g.prefix+":hmacnonce:"+token, 1, ttl).Result()
}
