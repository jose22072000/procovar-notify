package quota

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

func newTestService(t *testing.T) (*Service, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	s := New(rdb, "qbn")
	s.now = func() time.Time { return time.Date(2026, 6, 26, 10, 0, 0, 0, time.UTC) }
	return s, mr
}

func TestAllow_UnlimitedWhenZero(t *testing.T) {
	s, _ := newTestService(t)
	for i := 0; i < 100; i++ {
		ok, err := s.Allow(context.Background(), uuid.New(), 0, 0)
		if err != nil || !ok {
			t.Fatalf("0/0 debería ser ilimitado: ok=%v err=%v", ok, err)
		}
	}
}

func TestAllow_DailyLimit(t *testing.T) {
	s, _ := newTestService(t)
	app := uuid.New()
	// Límite diario 3: los 3 primeros pasan, el 4º se rechaza.
	for i := 1; i <= 3; i++ {
		ok, err := s.Allow(context.Background(), app, 3, 0)
		if err != nil || !ok {
			t.Fatalf("envío %d debería permitirse: ok=%v err=%v", i, ok, err)
		}
	}
	ok, err := s.Allow(context.Background(), app, 3, 0)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if ok {
		t.Fatal("el 4º envío debería rechazarse por cuota diaria")
	}
}

func TestAllow_SeparatePerApp(t *testing.T) {
	s, _ := newTestService(t)
	a, b := uuid.New(), uuid.New()
	// Agota la cuota de A; B no debe verse afectada.
	_, _ = s.Allow(context.Background(), a, 1, 0)
	if ok, _ := s.Allow(context.Background(), a, 1, 0); ok {
		t.Fatal("A debería estar agotada")
	}
	if ok, _ := s.Allow(context.Background(), b, 1, 0); !ok {
		t.Fatal("B no debería verse afectada por A")
	}
}

func TestAllow_MonthlyLimit(t *testing.T) {
	s, _ := newTestService(t)
	app := uuid.New()
	// Diario alto, mensual 2: el 3º se rechaza por la mensual.
	for i := 1; i <= 2; i++ {
		if ok, _ := s.Allow(context.Background(), app, 1000, 2); !ok {
			t.Fatalf("envío %d debería permitirse", i)
		}
	}
	if ok, _ := s.Allow(context.Background(), app, 1000, 2); ok {
		t.Fatal("el 3º debería rechazarse por cuota mensual")
	}
}

// TestAllow_RejectedMonthlyDoesNotChargeDaily (L5): cuando se rechaza por la
// cuota mensual, el contador diario NO debe incrementarse (no se "cobra" un
// envío que no ocurre).
func TestAllow_RejectedMonthlyDoesNotChargeDaily(t *testing.T) {
	s, mr := newTestService(t)
	app := uuid.New()
	dKey := "qbn:quota:d:" + app.String() + ":20260626"

	// Mensual ya agotada (límite 1, consumida); diario amplio.
	if ok, _ := s.Allow(context.Background(), app, 100, 1); !ok {
		t.Fatal("el 1º debería permitirse")
	}
	dailyAfterFirst, _ := mr.Get(dKey)
	if dailyAfterFirst != "1" {
		t.Fatalf("el diario debería ser 1 tras el 1º envío, got %q", dailyAfterFirst)
	}

	// 2º: rechazado por la mensual → el diario NO debe subir a 2.
	if ok, _ := s.Allow(context.Background(), app, 100, 1); ok {
		t.Fatal("el 2º debería rechazarse por la mensual")
	}
	dailyAfterReject, _ := mr.Get(dKey)
	if dailyAfterReject != "1" {
		t.Fatalf("el diario no debería incrementarse en un rechazo mensual, got %q", dailyAfterReject)
	}
}
