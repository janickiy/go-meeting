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

var acquireLockScript = goredis.NewScript(`
local owner = redis.call("GET", KEYS[1])
if owner == ARGV[1] then
    redis.call("PEXPIRE", KEYS[1], ARGV[2])
    return 1
end
if not owner then
    redis.call("SET", KEYS[1], ARGV[1], "PX", ARGV[2])
    return 1
end
return 0
`)

// ConferenceLock управляет Redis-блокировкой одной активной записи на конференцию.
//   - client: клиент внешнего сервиса или транспорта компонента.
//   - ttl: срок жизни сохраняемого значения или выданного разрешения.
type ConferenceLock struct {
	client *goredis.Client
	ttl    time.Duration
}

// NewConferenceLock создаёт блокировку Redis для conferenceId.
// @args
// - client: клиент Redis.
// - ttl: срок жизни lock-а.
// @return ConferenceLock.
func NewConferenceLock(client *goredis.Client, ttl time.Duration) *ConferenceLock {
	return &ConferenceLock{client: client, ttl: ttl}
}

// Acquire пытается установить блокировку conferenceId со значением recordId.
// @args
// - ctx: контекст операции.
// - conferenceID: UUID конференции.
// - recordID: UUID записи-владельца lock-а.
// @return true, если блокировка успешно установлена.
func (l *ConferenceLock) Acquire(ctx context.Context, conferenceID string, recordID string) (bool, error) {
	if l == nil || l.client == nil {
		return true, nil
	}
	ttl := l.ttl
	if ttl <= 0 {
		ttl = 6 * time.Hour
	}
	result, err := acquireLockScript.Run(ctx, l.client, []string{lockKey(conferenceID)}, recordID, ttl.Milliseconds()).Int()
	if err != nil {
		return false, fmt.Errorf("acquire conference record lock: %w", err)
	}

	return result == 1, nil
}

// Release снимает lock только если им владеет текущая запись.
// @args
// - ctx: контекст операции.
// - conferenceID: UUID конференции.
// - recordID: UUID записи-владельца lock-а.
// @return ошибку Redis.
func (l *ConferenceLock) Release(ctx context.Context, conferenceID string, recordID string) error {
	if l == nil || l.client == nil || strings.TrimSpace(conferenceID) == "" || strings.TrimSpace(recordID) == "" {
		return nil
	}
	if err := releaseLockScript.Run(ctx, l.client, []string{lockKey(conferenceID)}, recordID).Err(); err != nil {
		return fmt.Errorf("release conference record lock: %w", err)
	}

	return nil
}

// lockKey строит Redis-ключ блокировки одной конференции.
//
// @args
//   - conferenceID (string): идентификатор конференции, ограничивающий область операции.
//
// @return:
//   - результат 1 (string): значение, подготовленное операцией для вызывающей стороны.
func lockKey(conferenceID string) string {
	return conferenceLockPrefix + strings.TrimSpace(conferenceID) + ":lock"
}
