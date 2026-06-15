package redis

import (
	"context"
	"fmt"

	goredis "github.com/redis/go-redis/v9"
)

// NewClient создает Redis-клиент и проверяет соединение.
// Параметры:
// - ctx: контекст подключения.
// - addr: host:port Redis.
// - password: пароль Redis, если задан.
// - db: номер Redis database.
// Возвращает: Redis client или ошибку подключения.
func NewClient(ctx context.Context, addr string, password string, db int) (*goredis.Client, error) {
	client := goredis.NewClient(&goredis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("connect redis: %w", err)
	}

	return client, nil
}
