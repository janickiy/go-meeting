package jobs

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	domain "github.com/janickiy/go-recorder/internal/domain/jobs"
)

// testQueue фиксирует безопасные изменения задания без реального SQL и внешних вызовов.
type testQueue struct {
	mu          sync.Mutex
	state, code string
	retryAt     *time.Time
}

// Claim сообщает отсутствие новых заданий для проверки остановки фиксированного пула.
// @args ctx/kind/lease — параметры тестового захвата.
// @return отсутствие задания без ошибки.
func (q *testQueue) Claim(context.Context, string, time.Duration) (domain.Job, bool, error) {
	return domain.Job{}, false, nil
}

// Finish сохраняет исход обработчика для последующих утверждений теста.
// @args job — тестовая задача; state/code/retryAt — решение механизма повторов.
// @return nil: тестовый репозиторий не отказывает.
func (q *testQueue) Finish(_ context.Context, _ domain.Job, state, code string, retryAt *time.Time) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.state, q.code, q.retryAt = state, code, retryAt
	return nil
}

// Counts возвращает пустые технические агрегаты тестовой очереди.
// @args ctx — неиспользуемый контекст теста.
// @return пустая очередь без ошибки.
func (q *testQueue) Counts(context.Context) ([]domain.Count, error) { return nil, nil }

// TestRetryClassification проверяет pointer/value ошибки, secrets, timeout и явно пропущенную работу.
// @args t — контекст утверждений.
func TestRetryClassification(t *testing.T) {
	for _, tc := range []struct {
		name        string
		err         error
		state, code string
		retry       bool
	}{
		{"success", nil, "done", "", false}, {"skip", domain.ErrSkip, "skipped", "", false},
		{"retry", domain.Error{Code: "provider_rate_limit", Retryable: true}, "failed", "provider_rate_limit", true},
		{"pointer", &domain.Error{Code: "invalid_audio"}, "failed", "invalid_audio", false},
		{"secret", domain.Error{Code: "Bearer secret", Retryable: false}, "failed", "provider_failed", false},
		{"timeout", context.DeadlineExceeded, "failed", "operation_timeout", true},
		{"db", errors.New("password in raw database error"), "failed", "internal_unavailable", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state, code, retry, _ := classify(tc.err)
			if state != tc.state || code != tc.code || retry != tc.retry {
				t.Fatalf("unexpected safe classification: %s %s %v", state, code, retry)
			}
		})
	}
}

// TestBoundedRetriesAndExpiredFinalLease исключает дополнительный дорогой вызов после исчерпания попыток.
// @args t — контекст теста политики повторов и terminal callback.
func TestBoundedRetriesAndExpiredFinalLease(t *testing.T) {
	q := &testQueue{}
	called, failed := 0, 0
	pool := Pool{Kind: "content.transcribe", Concurrency: 1, Timeout: time.Second, Handler: Handler{
		Handle: func(context.Context, domain.Job) error {
			called++
			return domain.Error{Code: "provider_busy", Retryable: true, RetryAfter: 2 * time.Hour}
		},
		Fail: func(context.Context, domain.Job, string) error { failed++; return nil },
	}}
	r, err := New(q, []Pool{pool}, time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}
	r.execute(context.Background(), pool, domain.Job{Attempts: 1, MaxAttempts: 3})
	if called != 1 || failed != 0 || q.state != "queued" || q.retryAt == nil || time.Until(*q.retryAt) > time.Hour {
		t.Fatal("retry not bounded")
	}
	r.execute(context.Background(), pool, domain.Job{Attempts: 3, MaxAttempts: 3})
	if called != 2 || failed != 1 || q.state != "failed" || q.retryAt != nil {
		t.Fatal("final failure not recorded")
	}
	r.execute(context.Background(), pool, domain.Job{Attempts: 4, MaxAttempts: 3})
	if called != 2 || failed != 2 || q.code != "attempts_exhausted" {
		t.Fatal("expired final lease called provider again")
	}
}

// TestBackoffAndPanic проверяет конечные задержки и отсутствие panic payload в технической ошибке.
// @args t — контекст проверки границ.
func TestBackoffAndPanic(t *testing.T) {
	for i := -1; i < 30; i++ {
		d := Backoff(i, 0)
		if d < time.Second || d > time.Hour {
			t.Fatal("unbounded backoff", d)
		}
	}
	if Backoff(1, 24*time.Hour) != time.Hour {
		t.Fatal("Retry-After is not capped")
	}
	err := callHandler(context.Background(), func(context.Context, domain.Job) error { panic("private transcript and secret") }, domain.Job{})
	if err == nil || err.Error() != "handler_panic" {
		t.Fatal("panic data leaked")
	}
}

// TestRunnerShutdown подтверждает завершение фиксированных работников без задания и таймеров встреч.
// @args t — контекст проверки остановки.
func TestRunnerShutdown(t *testing.T) {
	r, err := New(&testQueue{}, []Pool{{Kind: "integrations.delivery", Concurrency: 2, Timeout: time.Second, Handler: Handler{Handle: func(context.Context, domain.Job) error { return nil }}}}, time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { r.Run(ctx); close(done) }()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("runner did not stop")
	}
}

// TestOperatorAttemptCapAppliesToOldJobs проверяет снижение расходов для уже сохранённых задач.
// @args t — контекст проверки общего операторского предела поверх SQL defaults.
func TestOperatorAttemptCapAppliesToOldJobs(t *testing.T) {
	q := &testQueue{}
	called := 0
	pool := Pool{Kind: "content.summarize", Concurrency: 1, MaxAttempts: 1, Timeout: time.Second, Handler: Handler{
		Handle: func(_ context.Context, job domain.Job) error {
			called++
			if job.MaxAttempts != 1 {
				t.Fatal("handler did not receive effective budget")
			}
			return domain.Error{Code: "provider_busy", Retryable: true}
		},
	}}
	r, err := New(q, []Pool{pool}, time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}
	r.execute(context.Background(), pool, domain.Job{Attempts: 1, MaxAttempts: 5})
	if q.state != "failed" || q.retryAt != nil || called != 1 {
		t.Fatal("operator cap ignored")
	}
	r.execute(context.Background(), pool, domain.Job{Attempts: 2, MaxAttempts: 5})
	if called != 1 || q.code != "attempts_exhausted" {
		t.Fatal("recovered old job spent above operator cap")
	}
}
