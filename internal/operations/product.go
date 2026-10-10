package operations

import (
	"context"
	"errors"
	"time"

	"github.com/janickiy/meet-space/internal/domain/jobs"
)

// productKind допускает только категории, заданные исходным кодом, а не пользовательскими данными.
// @args kind — технический вид задания.
// @return признак принадлежности ограниченному набору меток.
func productKind(kind string) bool {
	switch kind {
	case "integrations.conference", "integrations.event", "integrations.delivery", "integrations.calendar", "content.transcribe", "content.summarize", "content.embed", "analytics.aggregate":
		return true
	}
	return false
}

// Product учитывает исход и продолжительность законченной попытки отдельного worker.
// @args kind — фиксированная категория; outcome — безопасное состояние; duration — время работы.
func Product(kind, outcome string, duration time.Duration) {
	if !productKind(kind) {
		return
	}
	switch outcome {
	case "done", "failed", "skipped", "queued":
	default:
		return
	}
	if r := current.Load(); r != nil {
		r.productJobs.WithLabelValues(kind, outcome).Inc()
		r.productDuration.WithLabelValues(kind).Observe(duration.Seconds())
	}
}

// ProductQueue обновляет снимок очереди; идентификаторы сущностей, пользователей и URL провайдеров не становятся метками.
// @args kind — фиксированная категория; state — queued/processing/failed; count — число заданий.
func ProductQueue(kind, state string, count int64) {
	if !productKind(kind) || (state != "queued" && state != "processing" && state != "failed") {
		return
	}
	if r := current.Load(); r != nil {
		r.productQueue.WithLabelValues(kind, state).Set(float64(count))
	}
}

// ProviderCall учитывает реальную попытку адаптера без содержимого и идентификаторов встречи.
// @args provider — один из пяти фиксированных каналов; duration — длительность;
// err — классифицированная ошибка, текст которой не становится меткой или журналом.
func ProviderCall(provider string, duration time.Duration, err error) {
	switch provider {
	case "email", "push", "calendar", "stt", "ai", "embedding", "live_stt":
	default:
		return
	}
	outcome := "success"
	if err != nil {
		outcome = "retryable"
		var value jobs.Error
		var pointer *jobs.Error
		if errors.As(err, &pointer) && pointer != nil {
			value = *pointer
		} else {
			_ = errors.As(err, &value)
		}
		switch {
		case errors.Is(err, jobs.ErrSkip):
			outcome = "skipped"
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			outcome = "timeout"
		case value.Code == "provider_rate_limited":
			outcome = "rate_limited"
		case value.Code != "" && !value.Retryable:
			outcome = "permanent"
		}
	}
	if r := current.Load(); r != nil {
		r.providerCalls.WithLabelValues(provider, outcome).Inc()
		r.providerTime.WithLabelValues(provider).Observe(duration.Seconds())
	}
}

// Search учитывает время запроса и безопасный класс ответа без поисковой строки.
// @args duration — полное время обработчика; failed — признак HTTP-отказа.
func Search(duration time.Duration, failed bool) {
	outcome := "success"
	if failed {
		outcome = "failed"
	}
	if r := current.Load(); r != nil {
		r.searchRequests.WithLabelValues(outcome).Inc()
		r.searchTime.Observe(duration.Seconds())
	}
}
