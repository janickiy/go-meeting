package redis

import (
	"context"
	"fmt"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

const conferenceLockPrefix = "record:conference:"

var releaseLockScript = goredis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("DEL", KEYS[1])
end
return 0
`)

// ConferenceLock запрещает параллельный record.start для одного conferenceId.
type ConferenceLock struct {
	client *goredis.Client
	ttl    time.Duration
}

// NewConferenceLock создает Redis lock для conferenceId.
// Параметры:
// - client: Redis client.
// - ttl: срок жизни lock-а.
// Возвращает: ConferenceLock.
func NewConferenceLock(client *goredis.Client, ttl time.Duration) *ConferenceLock {
	return &ConferenceLock{client: client, ttl: ttl}
}

// Acquire пытается поставить lock conferenceId со значением recordId.
// Параметры:
// - ctx: контекст операции.
// - conferenceID: UUID конференции.
// - recordID: UUID записи-владельца lock-а.
// Возвращает: true, если lock успешно установлен.
func (l *ConferenceLock) Acquire(ctx context.Context, conferenceID string, recordID string) (bool, error) {
	if l == nil || l.client == nil {
		return true, nil
	}
	ttl := l.ttl
	if ttl <= 0 {
		ttl = 6 * time.Hour
	}
	ok, err := l.client.SetNX(ctx, lockKey(conferenceID), recordID, ttl).Result()
	if err != nil {
		return false, fmt.Errorf("acquire conference record lock: %w", err)
	}

	return ok, nil
}

// Release снимает lock только если им владеет текущая запись.
// Параметры:
// - ctx: контекст операции.
// - conferenceID: UUID конференции.
// - recordID: UUID записи-владельца lock-а.
// Возвращает: ошибку Redis.
func (l *ConferenceLock) Release(ctx context.Context, conferenceID string, recordID string) error {
	if l == nil || l.client == nil || strings.TrimSpace(conferenceID) == "" || strings.TrimSpace(recordID) == "" {
		return nil
	}
	if err := releaseLockScript.Run(ctx, l.client, []string{lockKey(conferenceID)}, recordID).Err(); err != nil {
		return fmt.Errorf("release conference record lock: %w", err)
	}

	return nil
}

func lockKey(conferenceID string) string {
	return conferenceLockPrefix + strings.TrimSpace(conferenceID) + ":lock"
}
