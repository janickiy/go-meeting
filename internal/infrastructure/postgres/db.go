package postgres

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Connect открывает подключение к PostgreSQL через GORM.
// @args
// - dsn: строка подключения PostgreSQL.
// @return *gorm.DB или ошибку подключения.
func Connect(dsn string) (*gorm.DB, error) {
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("invalid postgres connection configuration")
	}
	cfg.ConnectTimeout = 5 * time.Second
	queryTimeout := 10 * time.Second
	if raw := os.Getenv("DB_QUERY_TIMEOUT"); raw != "" {
		queryTimeout, err = time.ParseDuration(raw)
		if err != nil || queryTimeout < time.Second || queryTimeout > time.Minute {
			return nil, fmt.Errorf("DB_QUERY_TIMEOUT must be 1s..1m")
		}
	}
	cfg.RuntimeParams["statement_timeout"] = strconv.FormatInt(queryTimeout.Milliseconds(), 10)
	cfg.RuntimeParams["lock_timeout"] = "5000"
	connection := stdlib.OpenDB(*cfg)
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: connection}), &gorm.Config{
		Logger: logger.Discard,
	})
	if err != nil {
		_ = connection.Close()
		return nil, fmt.Errorf("connect postgres: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(20)
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)

	return db, nil
}
