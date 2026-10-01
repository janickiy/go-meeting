package redis

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/ratelimit"
	goredis "github.com/redis/go-redis/v9"
)

var rateLimitScript = goredis.NewScript(`
local key = KEYS[1]
local limit = tonumber(ARGV[1])
local window_ms = tonumber(ARGV[2])
local now_ms = tonumber(ARGV[3])
local member = ARGV[4]

redis.call("ZREMRANGEBYSCORE", key, 0, now_ms - window_ms)

local count = redis.call("ZCARD", key)
if count >= limit then
	local oldest = redis.call("ZRANGE", key, 0, 0, "WITHSCORES")
	local retry_ms = window_ms
	if oldest[2] then
		retry_ms = tonumber(oldest[2]) + window_ms - now_ms
	end
	if retry_ms < 0 then
		retry_ms = 0
	end
	redis.call("PEXPIRE", key, window_ms)
	return {0, 0, math.ceil(retry_ms / 1000), math.floor((now_ms + retry_ms) / 1000)}
end

redis.call("ZADD", key, now_ms, member)
redis.call("PEXPIRE", key, window_ms)

local remaining = limit - count - 1
return {1, remaining, 0, math.floor((now_ms + window_ms) / 1000)}
`)

// RateLimiter применяет Redis-ограничения частоты независимо для разных видов операций.
//   - client: клиент внешнего сервиса или транспорта компонента.
type RateLimiter struct {
	client *goredis.Client
}

// NewRateLimiter создает Redis rate limiter.
// @parameters:
// - client: Redis client.
// @return RateLimiter.
func NewRateLimiter(client *goredis.Client) *RateLimiter {
	return &RateLimiter{client: client}
}

// Allow проверяет, можно ли выполнить запрос по ключу лимита.
// @parameters:
// - ctx: контекст операции.
// - key: Redis key лимита.
// - limit: максимум запросов за окно.
// - window: длительность окна.
// @return результат проверки или ошибку Redis.
func (l *RateLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (ratelimit.Result, error) {
	if l == nil || l.client == nil || limit <= 0 {
		return ratelimit.Result{Allowed: true, Limit: limit}, nil
	}
	if window <= 0 {
		window = time.Minute
	}
	now := time.Now().UTC()
	member := strconv.FormatInt(now.UnixNano(), 10) + ":" + uuid.NewString()

	raw, err := rateLimitScript.Run(ctx, l.client, []string{key}, limit, window.Milliseconds(), now.UnixMilli(), member).Result()
	if err != nil {
		return ratelimit.Result{}, fmt.Errorf("check redis rate limit: %w", err)
	}
	values, ok := raw.([]interface{})
	if !ok || len(values) != 4 {
		return ratelimit.Result{}, fmt.Errorf("unexpected redis rate limit response %T", raw)
	}

	allowed, err := redisInt(values[0])
	if err != nil {
		return ratelimit.Result{}, err
	}
	remaining, err := redisInt(values[1])
	if err != nil {
		return ratelimit.Result{}, err
	}
	retryAfterSec, err := redisInt(values[2])
	if err != nil {
		return ratelimit.Result{}, err
	}
	resetAtSec, err := redisInt(values[3])
	if err != nil {
		return ratelimit.Result{}, err
	}

	return ratelimit.Result{
		Allowed:    allowed == 1,
		Limit:      limit,
		Remaining:  int(remaining),
		RetryAfter: time.Duration(retryAfterSec) * time.Second,
		ResetAt:    time.Unix(resetAtSec, 0).UTC(),
	}, nil
}

// redisInt приводит числовой ответ Redis к ожидаемому целочисленному виду.
//
// @parameters:
//   - value (interface{}): значение для проверки, нормализации или преобразования.
//
// @return:
//   - результат 1 (int64): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func redisInt(value interface{}) (int64, error) {
	switch typed := value.(type) {
	case int:
		return int64(typed), nil
	case int64:
		return typed, nil
	case string:
		parsed, err := strconv.ParseInt(typed, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("parse redis integer %q: %w", typed, err)
		}

		return parsed, nil
	default:
		return 0, fmt.Errorf("unexpected redis integer type %T", value)
	}
}
