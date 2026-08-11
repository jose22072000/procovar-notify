package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// serveRL ejecuta una petición con un Caller inyectado a través del middleware
// del rate limiter y devuelve el recorder.
func serveRL(rl *RateLimiter, keyID string) *httptest.ResponseRecorder {
	h := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	req := httptest.NewRequest("GET", "/", nil).WithContext(ContextWithCaller(context.Background(), Caller{KeyID: keyID}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestRateLimiterRefill(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	rl := NewRateLimiter(rdb, "t", 1, 2) // rate 1/s, burst 2
	clock := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	rl.now = func() time.Time { return clock }

	if c := serveRL(rl, "k").Code; c != 200 {
		t.Fatalf("1ª debería pasar, got %d", c)
	}
	if c := serveRL(rl, "k").Code; c != 200 {
		t.Fatalf("2ª (burst) debería pasar, got %d", c)
	}
	if c := serveRL(rl, "k").Code; c != http.StatusTooManyRequests {
		t.Fatalf("3ª debería ser 429, got %d", c)
	}

	// Pasan 2s: el bucket se rellena (rate 1/s) y vuelve a permitir.
	clock = clock.Add(2 * time.Second)
	if c := serveRL(rl, "k").Code; c != 200 {
		t.Fatalf("tras el refill debería pasar, got %d", c)
	}
}

func TestRateLimiterHeaders(t *testing.T) {
	mr, _ := miniredis.Run()
	defer mr.Close()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	rl := NewRateLimiter(rdb, "t", 1, 2)

	rec := serveRL(rl, "h")
	if rec.Header().Get("X-RateLimit-Limit") != "2" {
		t.Errorf("X-RateLimit-Limit = %q, want 2", rec.Header().Get("X-RateLimit-Limit"))
	}
	if rec.Header().Get("X-RateLimit-Remaining") == "" {
		t.Error("falta X-RateLimit-Remaining")
	}

	// Agota el burst y comprueba Retry-After en el 429.
	serveRL(rl, "h")
	denied := serveRL(rl, "h")
	if denied.Code != http.StatusTooManyRequests {
		t.Fatalf("debería ser 429, got %d", denied.Code)
	}
	if denied.Header().Get("Retry-After") == "" {
		t.Error("falta Retry-After en el 429")
	}
}

func TestRateLimiterFailOpen(t *testing.T) {
	mr, _ := miniredis.Run()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	rl := NewRateLimiter(rdb, "t", 1, 1)
	mr.Close() // Redis caído: el limiter debe fallar abierto (no bloquear tráfico).

	if c := serveRL(rl, "k").Code; c != 200 {
		t.Fatalf("con Redis caído el limiter debería fallar abierto (200), got %d", c)
	}
}
