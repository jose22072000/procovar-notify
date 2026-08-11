package notification_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// fakeQuota implementa notification.QuotaChecker de forma determinista.
type fakeQuota struct {
	allow bool
	calls int
}

func (f *fakeQuota) Allow(_ context.Context, _ uuid.UUID, _, _ int32) (bool, error) {
	f.calls++
	return f.allow, nil
}

// TestQuotaExceededRejects: con cuota configurada y verificador que niega, Create
// devuelve error (429) y no encola.
func TestQuotaExceededRejects(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	_, err := f.db.Pool.Exec(ctx, `UPDATE applications SET daily_send_quota = 5 WHERE id = $1`, f.appID)
	must(t, err)

	enq := &fakeEnqueuer{}
	fq := &fakeQuota{allow: false}
	_, err = f.service(enq).WithQuota(fq).Create(ctx, baseInput(f.appID))
	if err == nil {
		t.Fatal("se esperaba error por cuota excedida")
	}
	if len(enq.ids) != 0 {
		t.Fatal("no debería encolarse al exceder la cuota")
	}
	if fq.calls != 1 {
		t.Fatalf("el verificador debería llamarse 1 vez, got %d", fq.calls)
	}
}

// TestQuotaAllowed: con cuota y verificador que permite, el envío procede.
func TestQuotaAllowed(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	_, err := f.db.Pool.Exec(ctx, `UPDATE applications SET daily_send_quota = 5 WHERE id = $1`, f.appID)
	must(t, err)

	n, err := f.service(&fakeEnqueuer{}).WithQuota(&fakeQuota{allow: true}).Create(ctx, baseInput(f.appID))
	must(t, err)
	if string(n.Status) != "QUEUED" {
		t.Fatalf("estado esperado QUEUED, got %s", n.Status)
	}
}

// TestQuotaSkippedWhenZero: sin cuota configurada (0), el verificador ni se
// invoca (no cuenta innecesariamente).
func TestQuotaSkippedWhenZero(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	fq := &fakeQuota{allow: false} // negaría si se llamara
	n, err := f.service(&fakeEnqueuer{}).WithQuota(fq).Create(ctx, baseInput(f.appID))
	must(t, err)
	if string(n.Status) != "QUEUED" {
		t.Fatalf("sin cuota debería encolarse; got %s", n.Status)
	}
	if fq.calls != 0 {
		t.Fatalf("sin cuota el verificador no debería llamarse, got %d", fq.calls)
	}
}
