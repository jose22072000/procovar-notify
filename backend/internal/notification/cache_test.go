package notification

import (
	"testing"
	"time"
)

func TestTTLCacheExpiry(t *testing.T) {
	c := newTTLCache[int]()
	c.set("a", 1, 50*time.Millisecond)
	if v, ok := c.get("a"); !ok || v != 1 {
		t.Fatalf("esperaba hit con 1, got %v %v", v, ok)
	}
	time.Sleep(60 * time.Millisecond)
	if _, ok := c.get("a"); ok {
		t.Fatal("la entrada debería haber expirado")
	}
	if _, ok := c.get("nunca"); ok {
		t.Fatal("una clave inexistente no debe dar hit")
	}
}
