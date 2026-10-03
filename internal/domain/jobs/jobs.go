// Пакет jobs задаёт независимый от транспорта контракт постоянных фоновых заданий.
package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

var (
	// ErrSkip означает осознанный пропуск отключённой или устаревшей операции.
	ErrSkip = errors.New("job skipped")
	// ErrLeaseLost запрещает завершение задания после смены его владельца.
	ErrLeaseLost = errors.New("job lease lost")
)

// Job содержит ссылки на сущности и ограниченную нагрузку, но не токены или текст расшифровки.
// Version связывает обработку с поколением сущности; LeaseToken защищает запись результата
// от опоздавшего работника. Attempts учитывает захваты, включая восстановление после сбоя.
type Job struct {
	ID           string          `json:"id"`
	Kind         string          `json:"kind"`
	EntityID     string          `json:"entityId"`
	ConferenceID string          `json:"conferenceId"`
	UserID       *string         `json:"userId,omitempty"`
	Version      int64           `json:"version"`
	Payload      json.RawMessage `json:"payload" gorm:"type:jsonb"`
	DedupKey     string          `json:"dedupKey"`
	Attempts     int             `json:"attempts"`
	MaxAttempts  int             `json:"maxAttempts"`
	LeaseToken   string          `json:"-"`
	LeaseUntil   time.Time       `json:"-"`
	CreatedAt    time.Time       `json:"createdAt"`
}

// Count представляет число заданий одной технической категории без пользовательских меток.
type Count struct {
	Kind  string
	State string
	Count int64
}

// Repository атомарно выдаёт задания и принимает завершение только от владельца аренды.
type Repository interface {
	// Claim захватывает одно доступное задание категории kind на срок lease; bool показывает наличие работы.
	Claim(ctx context.Context, kind string, lease time.Duration) (Job, bool, error)
	// Finish меняет состояние арендованного задания; retryAt повторно ставит его в очередь.
	Finish(ctx context.Context, job Job, state, code string, retryAt *time.Time) error
	// Counts возвращает ограниченные агрегаты очереди для служебных метрик.
	Counts(ctx context.Context) ([]Count, error)
}

// Error описывает безопасный код отказа, возможность повтора и ограниченную паузу провайдера.
// Code не должен содержать токен, ответ внешнего API или пользовательский текст.
type Error struct {
	Code       string
	Retryable  bool
	RetryAfter time.Duration
}

// Error возвращает только технический код без исходного ответа провайдера.
// @return безопасная строка ошибки.
func (e Error) Error() string { return e.Code }
