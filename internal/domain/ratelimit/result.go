package ratelimit

import (
	"time"
)

// Result передаёт результат операции и связанные метаданные компонента.
// @params
//   - Allowed: логический признак Allowed, управляющий соответствующей веткой обработки.
//   - Limit: предел количества обрабатываемых элементов.
//   - Remaining: значение Remaining типа int, используемое согласно назначению этой операции.
//   - RetryAfter: значение RetryAfter типа time.Duration, используемое согласно назначению этой операции.
//   - ResetAt: временная отметка ResetAt; указатель допускает отсутствие значения.
type Result struct {
	Allowed    bool
	Limit      int
	Remaining  int
	RetryAfter time.Duration
	ResetAt    time.Time
}
