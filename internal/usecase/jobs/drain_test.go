package jobs

import (
	"context"
	domain "github.com/janickiy/go-recorder/internal/domain/jobs"
	"sync/atomic"
	"testing"
	"time"
)

// drainQueue удерживает SQL-захват, чтобы проверить гонку между остановкой и получением задания.
type drainQueue struct {
	testQueue
	claimed chan struct{}
	release chan struct{}
	calls   atomic.Int64
}

// Claim имитирует единственный уже начавшийся захват.
// @args ctx — отмена теста; остальные параметры не влияют на имитацию.
// @return одно задание после разрешения тестом.
func (q *drainQueue) Claim(ctx context.Context, _ string, _ time.Duration) (domain.Job, bool, error) {
	q.calls.Add(1)
	close(q.claimed)
	select {
	case <-ctx.Done():
		return domain.Job{}, false, ctx.Err()
	case <-q.release:
	}
	return domain.Job{Attempts: 1, MaxAttempts: 2}, true, nil
}

// TestDrainWaitsForPendingClaim не допускает ложного active=0 и новых захватов после завершения.
// @args t — контекст проверки конкурентной остановки.
func TestDrainWaitsForPendingClaim(t *testing.T) {
	q := &drainQueue{claimed: make(chan struct{}), release: make(chan struct{})}
	r, err := New(q, []Pool{{Kind: "integrations.delivery", Concurrency: 1, Timeout: time.Second, Handler: Handler{Handle: func(context.Context, domain.Job) error { return nil }}}}, time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() { r.Run(ctx); close(done) }()
	select {
	case <-q.claimed:
	case <-ctx.Done():
		t.Fatal("claim not started")
	}
	r.BeginDrain()
	if r.Active() != 1 {
		t.Fatal("pending claim missing from active work")
	}
	close(q.release)
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("drain failed")
	}
	if r.Active() != 0 || q.calls.Load() != 1 {
		t.Fatal("additional work after drain")
	}
}
