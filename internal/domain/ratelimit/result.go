package ratelimit

import (
	"time"
)

// Result описывает результат проверки rate limit.
type Result struct {
	Allowed    bool
	Limit      int
	Remaining  int
	RetryAfter time.Duration
	ResetAt    time.Time
}
