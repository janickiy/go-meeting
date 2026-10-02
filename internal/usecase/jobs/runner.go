// Package jobs исполняет постоянную очередь ограниченным набором независимых работников.
package jobs

import (
	"context"
	"errors"
	"log/slog"
	"math/rand/v2"
	"regexp"
	"sync"
	"time"

	domain "github.com/janickiy/go-recorder/internal/domain/jobs"
)

// Handler исполняет операцию; Fail сохраняет безопасный terminal failure связанной сущности.
type Handler struct {
	Handle func(context.Context, domain.Job) error
	Fail   func(context.Context, domain.Job, string) error
}

// Pool резервирует собственную конкуренцию и таймаут для одного фиксированного вида работ.
type Pool struct {
	Kind        string
	Concurrency int
	MaxAttempts int // Ноль сохраняет бюджет строки; 1..10 дополнительно ограничивают старые/trigger jobs.
	Timeout     time.Duration
	Handler     Handler
}

// Runner опрашивает PostgreSQL с постоянным числом горутин, не создавая таймер на встречу.
type Runner struct {
	repo     domain.Repository
	pools    []Pool
	interval time.Duration
	observe  func(string, string, time.Duration)
}

// New проверяет бюджет работников, чтобы неверный конфиг не вызвал неограниченный fanout.
// @args repo — очередь; pools — категории и бюджеты; interval — период опроса;
// observe — необязательный сборщик метрик с ограниченными метками.
// @return готовый исполнитель или ошибка конфигурации.
func New(repo domain.Repository, pools []Pool, interval time.Duration, observe func(string, string, time.Duration)) (*Runner, error) {
	if repo == nil || interval < 100*time.Millisecond || interval > time.Minute || len(pools) > 10 {
		return nil, errors.New("invalid job runner configuration")
	}
	seen := map[string]bool{}
	for _, pool := range pools {
		if pool.Kind == "" || seen[pool.Kind] || pool.Concurrency < 1 || pool.Concurrency > 16 || pool.MaxAttempts < 0 || pool.MaxAttempts > 10 || pool.Timeout < time.Second || pool.Timeout > 30*time.Minute || pool.Handler.Handle == nil {
			return nil, errors.New("invalid job pool configuration")
		}
		seen[pool.Kind] = true
	}
	return &Runner{repo, pools, interval, observe}, nil
}

// Run запускает выделенные пулы и ждёт их освобождения при отмене общего контекста.
// @args ctx — срок жизни процесса; новые задания после отмены не захватываются.
func (r *Runner) Run(ctx context.Context) {
	var workers sync.WaitGroup
	for _, pool := range r.pools {
		for range pool.Concurrency {
			workers.Go(func() { r.work(ctx, pool) })
		}
	}
	workers.Wait()
}

// work последовательно обрабатывает задания категории с отдельным конечным сроком аренды.
// @args ctx — остановка процесса; pool — тип, таймаут и обработчики категории.
func (r *Runner) work(ctx context.Context, pool Pool) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for ctx.Err() == nil {
		claimCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		job, found, err := r.repo.Claim(claimCtx, pool.Kind, pool.Timeout+30*time.Second)
		cancel()
		if err == nil && found {
			r.execute(ctx, pool, job)
			continue
		}
		if err != nil && ctx.Err() == nil {
			slog.Warn("product job queue unavailable", "event_type", "product.queue.failed", "kind", pool.Kind)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// execute отделяет вызов провайдера от короткой записи результата и ограниченных повторов.
// @args ctx — остановка; pool — политика исполнения; job — арендованное задание.
func (r *Runner) execute(ctx context.Context, pool Pool, job domain.Job) {
	start := time.Now()
	if pool.MaxAttempts > 0 && pool.MaxAttempts < job.MaxAttempts {
		job.MaxAttempts = pool.MaxAttempts
	}
	var err error
	if job.Attempts > job.MaxAttempts {
		err = &domain.Error{Code: "attempts_exhausted"}
	} else {
		callCtx, cancel := context.WithTimeout(ctx, pool.Timeout)
		err = callHandler(callCtx, pool.Handler.Handle, job)
		cancel()
	}
	state, code, retryable, wait := classify(err)
	var retryAt *time.Time
	if retryable && job.Attempts < job.MaxAttempts {
		delay := Backoff(job.Attempts, wait)
		next := time.Now().Add(delay)
		retryAt, state = &next, "queued"
	}
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if state == "failed" && pool.Handler.Fail != nil {
		if failure := pool.Handler.Fail(finishCtx, job, code); failure != nil {
			slog.Warn("product failure state not saved", "event_type", "product.failure.pending", "kind", pool.Kind, "job_id", job.ID)
			return // аренда истечёт, повторный захват сохранит terminal state без вызова провайдера.
		}
	}
	if finishErr := r.repo.Finish(finishCtx, job, state, code, retryAt); finishErr != nil {
		slog.Warn("product job completion deferred", "event_type", "product.completion.pending", "kind", pool.Kind, "job_id", job.ID)
		return
	}
	if r.observe != nil {
		r.observe(pool.Kind, state, time.Since(start))
	}
	slog.Info("product job completed", "event_type", "product.job.completed", "kind", pool.Kind, "state", state, "job_id", job.ID, "entity_id", job.EntityID, "conference_id", job.ConferenceID, "attempt", job.Attempts, "code", code)
}

var safeCode = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

// callHandler изолирует панику адаптера и не выводит её содержимое, которое может быть секретным.
// @args ctx — срок вызова; handle — обработчик; job — ограниченная нагрузка.
// @return безопасная permanent ошибка при панике либо исходный результат.
func callHandler(ctx context.Context, handle func(context.Context, domain.Job) error, job domain.Job) (err error) {
	defer func() {
		if recover() != nil {
			err = domain.Error{Code: "handler_panic"}
		}
	}()
	return handle(ctx, job)
}

// classify преобразует произвольный отказ в безопасные метаданные, не сохраняя vendor body.
// @args err — результат обработчика.
// @return состояние, безопасный код, возможность повтора и пожелание провайдера к паузе.
func classify(err error) (string, string, bool, time.Duration) {
	if err == nil {
		return "done", "", false, 0
	}
	if errors.Is(err, domain.ErrSkip) {
		return "skipped", "", false, 0
	}
	if errors.Is(err, domain.ErrLeaseLost) {
		return "skipped", "lease_lost", false, 0
	}
	var failure *domain.Error
	var valueFailure domain.Error
	if errors.As(err, &valueFailure) {
		failure = &valueFailure
	} else {
		_ = errors.As(err, &failure)
	}
	if failure != nil {
		code := failure.Code
		if !safeCode.MatchString(code) {
			code = "provider_failed"
		}
		return "failed", code, failure.Retryable, failure.RetryAfter
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return "failed", "operation_timeout", true, 0
	}
	return "failed", "internal_unavailable", true, 0
}

// Backoff вычисляет ограниченную экспоненциальную задержку с jitter без общего генератора состояния.
// @args attempt — номер попытки; requested — Retry-After внешнего сервиса.
// @return пауза от секунды до часа.
func Backoff(attempt int, requested time.Duration) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 10 {
		attempt = 10
	}
	base := time.Second * time.Duration(1<<uint(attempt))
	delay := base/2 + time.Duration(rand.Int64N(int64(base/2)+1))
	if requested > delay {
		delay = requested
	}
	if delay > time.Hour {
		delay = time.Hour
	}
	return delay
}
