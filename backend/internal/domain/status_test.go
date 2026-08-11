package domain

import "testing"

func TestDeriveQueueState(t *testing.T) {
	cases := []struct {
		status string
		future bool
		want   QueueState
	}{
		{StatusPending, true, QueueScheduled}, // programada a futuro
		{StatusPending, false, QueuePending},  // pendiente ya
		{StatusQueued, false, QueuePending},
		{StatusProcessing, false, QueueActive},
		{StatusRetry, false, QueueRetry},
		{StatusSent, false, QueueCompleted},
		{StatusDelivered, false, QueueCompleted},
		{StatusRead, false, QueueCompleted},
		{StatusFailed, false, QueueFailed},
		{StatusCancelled, false, QueueCancelled},
		{"DESCONOCIDO", false, QueuePending}, // fallback
	}
	for _, c := range cases {
		if got := DeriveQueueState(c.status, c.future); got != c.want {
			t.Errorf("DeriveQueueState(%q, future=%v) = %q, want %q", c.status, c.future, got, c.want)
		}
	}
}
