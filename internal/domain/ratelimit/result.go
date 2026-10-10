package ratelimit

import (
	"time"
)

// Result содержит решение ограничителя и значения для HTTP-заголовков лимита.
// Limit задаёт число запросов в окне, Remaining — оставшийся запас.
// RetryAfter — ожидание перед повтором; нулевой ResetAt не задаёт время сброса
// и не порождает заголовок X-RateLimit-Reset.
type Result struct {
	Allowed    bool
	Limit      int
	Remaining  int
	RetryAfter time.Duration
	ResetAt    time.Time
}
