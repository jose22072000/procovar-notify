package notification

import (
	"sync"
	"time"
)

// TTLs de la caché de la ruta caliente del worker. La invalidación es por
// expiración (el worker no ve las ediciones del admin en la api): un cambio de
// ruta/SMTP tarda como mucho routeCacheTTL en aplicarse a los envíos.
const (
	// routeCacheTTL: rutas + credenciales descifradas. Corto: es lo que puede
	// editar el admin en caliente.
	routeCacheTTL = 30 * time.Second
	// templatePinnedTTL: una versión concreta de plantilla es inmutable; el
	// TTL solo acota memoria.
	templatePinnedTTL = 10 * time.Minute
	// templateLatestTTL: "última versión activa" cambia al editar la plantilla.
	templateLatestTTL = 30 * time.Second
)

// ttlCache es una caché en memoria con expiración por entrada, segura para
// uso concurrente. Evita re-consultar (y re-descifrar) datos casi estáticos
// en cada envío: la ruta caliente pasaba ~8-11 viajes a Postgres por
// notificación y la mayoría eran siempre los mismos datos.
type ttlCache[V any] struct {
	mu sync.RWMutex
	m  map[string]ttlEntry[V]
}

type ttlEntry[V any] struct {
	val     V
	expires time.Time
}

func newTTLCache[V any]() *ttlCache[V] {
	return &ttlCache[V]{m: make(map[string]ttlEntry[V])}
}

// get devuelve el valor si existe y no ha expirado.
func (c *ttlCache[V]) get(key string) (V, bool) {
	c.mu.RLock()
	e, ok := c.m[key]
	c.mu.RUnlock()
	if !ok || time.Now().After(e.expires) {
		var zero V
		return zero, false
	}
	return e.val, true
}

// set guarda el valor con su TTL y aprovecha para expulsar entradas vencidas
// (mantiene la memoria acotada sin un goroutine de limpieza).
func (c *ttlCache[V]) set(key string, val V, ttl time.Duration) {
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, e := range c.m {
		if now.After(e.expires) {
			delete(c.m, k)
		}
	}
	c.m[key] = ttlEntry[V]{val: val, expires: now.Add(ttl)}
}
