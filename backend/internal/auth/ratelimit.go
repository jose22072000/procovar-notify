package auth

import (
	"context"
	"math"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"dvtech/qbn/internal/apperr"
	"dvtech/qbn/internal/httpx"
)

// tokenBucketScript implementa un token bucket atómico en Redis.
// KEYS[1] = clave del bucket
// ARGV   = rate (tokens/s), burst (máx), now (ms), requested
// Devuelve {allowed(0|1), tokens_restantes}.
var tokenBucketScript = redis.NewScript(`
local key = KEYS[1]
local rate = tonumber(ARGV[1])
local burst = tonumber(ARGV[2])
local now = tonumber(ARGV[3])
local requested = tonumber(ARGV[4])

local data = redis.call('HMGET', key, 'tokens', 'ts')
local tokens = tonumber(data[1])
local ts = tonumber(data[2])
if tokens == nil then
  tokens = burst
  ts = now
end

local delta = math.max(0, now - ts) / 1000.0
tokens = math.min(burst, tokens + delta * rate)

local allowed = 0
if tokens >= requested then
  tokens = tokens - requested
  allowed = 1
end

redis.call('HSET', key, 'tokens', tokens, 'ts', now)
redis.call('PEXPIRE', key, math.ceil(burst / rate * 1000) + 1000)

return {allowed, math.floor(tokens)}
`)

// RateLimiter aplica un token bucket por API key (key_id) usando Redis.
type RateLimiter struct {
	rdb    redis.UniversalClient
	prefix string
	rate   float64 // tokens por segundo (régimen sostenido)
	burst  int     // capacidad del bucket (ráfaga máxima)
	now    func() time.Time
}

// NewRateLimiter crea el limitador. rate = peticiones/s sostenidas; burst = pico.
func NewRateLimiter(rdb redis.UniversalClient, prefix string, rate float64, burst int) *RateLimiter {
	return &RateLimiter{rdb: rdb, prefix: prefix, rate: rate, burst: burst, now: time.Now}
}

// Middleware limita por la API key autenticada. Debe montarse DESPUÉS del
// middleware HMAC (necesita el Caller en el contexto).
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		caller, ok := CallerFromContext(r.Context())
		if !ok {
			httpx.WriteProblem(w, r, apperr.Unauthorized("unauthenticated", "Authentication required"))
			return
		}

		allowed, remaining, err := rl.allow(r.Context(), caller.KeyID)
		if err != nil {
			// Falla abierta: si Redis no responde, no bloqueamos el tráfico
			// legítimo (se registra arriba). Preferible a caer por el limitador.
			next.ServeHTTP(w, r)
			return
		}

		w.Header().Set("X-RateLimit-Limit", strconv.Itoa(rl.burst))
		w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
		if !allowed {
			retryAfter := int(math.Ceil(1.0 / rl.rate))
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
			httpx.WriteProblem(w, r, apperr.New(apperr.KindRateLimited, "rate_limited", "Too many requests"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// IPMiddleware limita por IP de origen (RemoteAddr — NO X-Forwarded-For, para
// que no se pueda eludir spoofeando la cabecera). Pensado para endpoints no
// autenticados como el login admin (anti fuerza bruta). Tras un proxy de
// confianza habría que resolver la IP real del cliente (ver L9 en REVISION.md).
func (rl *RateLimiter) IPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			ip = r.RemoteAddr
		}
		allowed, remaining, lerr := rl.allow(r.Context(), "ip:"+ip)
		if lerr != nil {
			next.ServeHTTP(w, r) // falla abierta si Redis no responde
			return
		}
		w.Header().Set("X-RateLimit-Limit", strconv.Itoa(rl.burst))
		w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
		if !allowed {
			retryAfter := int(math.Ceil(1.0 / rl.rate))
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
			httpx.WriteProblem(w, r, apperr.New(apperr.KindRateLimited, "rate_limited", "Too many requests"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// allow ejecuta el token bucket y devuelve si se permite y cuántos tokens quedan.
func (rl *RateLimiter) allow(ctx context.Context, keyID string) (bool, int, error) {
	key := rl.prefix + ":ratelimit:" + keyID
	res, err := tokenBucketScript.Run(ctx, rl.rdb,
		[]string{key},
		rl.rate, rl.burst, rl.now().UnixMilli(), 1,
	).Int64Slice()
	if err != nil {
		return false, 0, err
	}
	if len(res) != 2 {
		return true, rl.burst, nil
	}
	return res[0] == 1, int(res[1]), nil
}
