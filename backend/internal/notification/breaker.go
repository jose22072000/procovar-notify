package notification

import (
	"sync"
	"time"

	"github.com/sony/gobreaker"

	"dvtech/qbn/internal/channels"
)

// breakerRegistry mantiene un circuit breaker por destino (conexión SMTP o
// proveedor). Tras varios fallos consecutivos el breaker se abre y deja de
// intentar ese destino durante un tiempo, derivando al fallback (§7.1).
type breakerRegistry struct {
	mu sync.Mutex
	m  map[string]*gobreaker.CircuitBreaker
}

func newBreakerRegistry() *breakerRegistry {
	return &breakerRegistry{m: make(map[string]*gobreaker.CircuitBreaker)}
}

// get devuelve (creando si hace falta) el breaker para la clave de destino.
func (b *breakerRegistry) get(key string) *gobreaker.CircuitBreaker {
	b.mu.Lock()
	defer b.mu.Unlock()
	if cb, ok := b.m[key]; ok {
		return cb
	}
	cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:    key,
		Timeout: 30 * time.Second, // tiempo abierto antes de probar de nuevo
		ReadyToTrip: func(c gobreaker.Counts) bool {
			return c.ConsecutiveFailures >= 3
		},
		// Los errores permanentes (validación de destinatario/payload) no son
		// fallos de infraestructura: no deben contar para abrir el breaker (L1).
		IsSuccessful: func(err error) bool {
			return err == nil || channels.IsPermanent(err)
		},
	})
	b.m[key] = cb
	return cb
}
