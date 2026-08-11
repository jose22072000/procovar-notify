// Package quota cuenta los envíos por aplicación en Redis y decide si se supera
// el límite diario/mensual configurado por el tenant.
package quota

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// Service cuenta envíos por ventana (día/mes) usando contadores Redis con TTL.
type Service struct {
	rdb    redis.UniversalClient
	prefix string
	now    func() time.Time
}

// New crea el servicio de cuotas.
func New(rdb redis.UniversalClient, prefix string) *Service {
	return &Service{rdb: rdb, prefix: prefix, now: time.Now}
}

// allowScript decide atómicamente (read-then-incr) si se permite un envío sin
// "cobrar" cuotas que no se consumen: lee ambos contadores, rechaza si alguno
// está ya en su límite, y solo entonces incrementa los que apliquen. Antes el
// contador diario se incrementaba aun cuando el mensual rechazaba (L5).
// KEYS[1]=día, KEYS[2]=mes; ARGV: dLimit, mLimit, dTTLms, mTTLms.
var allowScript = redis.NewScript(`
local dlimit = tonumber(ARGV[1])
local mlimit = tonumber(ARGV[2])
if dlimit > 0 then
  local d = tonumber(redis.call('GET', KEYS[1]) or '0')
  if d >= dlimit then return 0 end
end
if mlimit > 0 then
  local m = tonumber(redis.call('GET', KEYS[2]) or '0')
  if m >= mlimit then return 0 end
end
if dlimit > 0 then
  if redis.call('INCR', KEYS[1]) == 1 then redis.call('PEXPIRE', KEYS[1], tonumber(ARGV[3])) end
end
if mlimit > 0 then
  if redis.call('INCR', KEYS[2]) == 1 then redis.call('PEXPIRE', KEYS[2], tonumber(ARGV[4])) end
end
return 1
`)

// Allow decide si un envío cabe en las cuotas diaria y mensual de la aplicación
// y, si cabe, las consume. Devuelve false si alguna cuota (>0) está agotada; un
// límite 0 significa ilimitado. Atómico vía Lua. Las ventanas se cuentan en UTC.
func (s *Service) Allow(ctx context.Context, appID uuid.UUID, daily, monthly int32) (bool, error) {
	if daily <= 0 && monthly <= 0 {
		return true, nil
	}
	now := s.now().UTC()
	dKey := fmt.Sprintf("%s:quota:d:%s:%s", s.prefix, appID, now.Format("20060102"))
	mKey := fmt.Sprintf("%s:quota:m:%s:%s", s.prefix, appID, now.Format("200601"))

	res, err := allowScript.Run(ctx, s.rdb, []string{dKey, mKey},
		daily, monthly,
		(48 * time.Hour).Milliseconds(),
		(40 * 24 * time.Hour).Milliseconds(),
	).Int64()
	if err != nil {
		return false, err
	}
	return res == 1, nil
}
