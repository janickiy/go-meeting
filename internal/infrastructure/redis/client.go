package redis

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// NewClient создает Redis-клиент и проверяет соединение.
// @args
// - ctx: контекст подключения.
// - addr: адрес Redis в формате хост:порт.
// - password: пароль Redis, если задан.
// - db: номер базы Redis.
// @return Redis client или ошибку подключения.
func NewClient(ctx context.Context, addr string, password string, db int) (*goredis.Client, error) {
	client := goredis.NewClient(&goredis.Options{
		Addr:                  addr,
		Password:              password,
		DB:                    db,
		DialTimeout:           3 * time.Second,
		ReadTimeout:           3 * time.Second,
		WriteTimeout:          3 * time.Second,
		PoolTimeout:           3 * time.Second,
		MaxRetries:            2,
		ContextTimeoutEnabled: true,
	})
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("connect redis: %w", err)
	}

	return client, nil
}
