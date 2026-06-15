package app

import (
	"context"
	"fmt"

	recordsapp "git.svc-dev.net/board/go-recorder/internal/app/records"
	"git.svc-dev.net/board/go-recorder/internal/config"
	postgresinfra "git.svc-dev.net/board/go-recorder/internal/infrastructure/postgres"
	rabbitmqinfra "git.svc-dev.net/board/go-recorder/internal/infrastructure/rabbitmq"
	redisinfra "git.svc-dev.net/board/go-recorder/internal/infrastructure/redis"
	s3storage "git.svc-dev.net/board/go-recorder/internal/infrastructure/storage/s3"
	workerinfra "git.svc-dev.net/board/go-recorder/internal/infrastructure/worker"
	httptransport "git.svc-dev.net/board/go-recorder/internal/transport/http"
	httpmiddleware "git.svc-dev.net/board/go-recorder/internal/transport/http/middleware"
	"git.svc-dev.net/board/go-recorder/internal/usecase/recorder"
)

// RunAPI запускает HTTP API.
// Параметры: нет.
// Возвращает: ошибку bootstrap или HTTP server-а.
func RunAPI() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	db, err := postgresinfra.Connect(cfg.PostgresDSN)
	if err != nil {
		return err
	}
	if err := postgresinfra.RunMigrations(db, "database/migrations"); err != nil {
		return err
	}
	s3Client, err := s3storage.NewClient(context.Background(), cfg.MinIOEndpoint, cfg.MinIOAccessKey, cfg.MinIOSecretKey, cfg.MinIOBucket, cfg.MinIOUseSSL)
	if err != nil {
		return err
	}
	s3Client.SetPublicEndpoint(cfg.MinIOPublicOrigin)
	redisClient, err := redisinfra.NewClient(context.Background(), cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	if err != nil {
		return err
	}
	defer redisClient.Close()
	conferenceLock := redisinfra.NewConferenceLock(redisClient, cfg.RecordLockTTL)
	rateLimiter := redisinfra.NewRateLimiter(redisClient)

	repository := postgresinfra.NewRecordRepository(db)
	commandPublisher, err := rabbitmqinfra.NewPublisher(context.Background(), rabbitmqinfra.Options{
		URL:        cfg.RabbitMQURL,
		Exchange:   cfg.RabbitMQExchange,
		Queue:      cfg.RabbitMQQueue,
		RoutingKey: cfg.RabbitMQRoutingKey,
	})
	if err != nil {
		return err
	}
	defer commandPublisher.Close()
	workerSignaler := workerinfra.NewClient(cfg.WorkerInternalURL)
	service := recorder.NewService(repository, commandPublisher, s3Client, conferenceLock)
	handler := recordsapp.NewHandlerWithSignaler(service, workerSignaler)
	router := httptransport.NewRouter(handler, cfg.IsLocal(), s3Client, httpmiddleware.RateLimit(rateLimiter, rateLimitConfig(cfg)))

	return router.Run(fmt.Sprintf(":%d", cfg.APIPort))
}

func rateLimitConfig(cfg config.Config) httpmiddleware.RateLimitConfig {
	window := cfg.RateLimitWindow
	defaultLimit := cfg.RateLimit.DefaultRPM
	limit := func(value int) int {
		if value > 0 {
			return value
		}

		return defaultLimit
	}
	path := func(route string) string {
		return httptransport.APIV1Prefix + route
	}
	rules := []httpmiddleware.Rule{
		{
			Method: "POST",
			Path:   path("/records/start"),
			Scope:  "conference",
			Limit:  limit(cfg.RateLimit.RecordStartConferenceRPM),
			Window: window,
			Key:    httpmiddleware.JSONFieldKey("conferenceId"),
		},
		{
			Method: "POST",
			Path:   path("/records/start"),
			Scope:  "ip",
			Limit:  limit(cfg.RateLimit.RecordStartIPRPM),
			Window: window,
			Key:    httpmiddleware.ClientIPKey,
		},
		{
			Method: "POST",
			Path:   path("/records/end"),
			Scope:  "record",
			Limit:  limit(cfg.RateLimit.RecordEndRecordRPM),
			Window: window,
			Key:    httpmiddleware.JSONFieldKey("recordId"),
		},
		{
			Method: "POST",
			Path:   path("/records/end"),
			Scope:  "ip",
			Limit:  limit(cfg.RateLimit.RecordEndIPRPM),
			Window: window,
			Key:    httpmiddleware.ClientIPKey,
		},
		{
			Method: "POST",
			Path:   path("/records/:id/webrtc/offer"),
			Scope:  "record",
			Limit:  limit(cfg.RateLimit.WebRTCOfferRecordRPM),
			Window: window,
			Key:    httpmiddleware.PathParamKey("id"),
		},
		{
			Method: "POST",
			Path:   path("/records/:id/webrtc/offer"),
			Scope:  "ip",
			Limit:  limit(cfg.RateLimit.WebRTCOfferIPRPM),
			Window: window,
			Key:    httpmiddleware.ClientIPKey,
		},
		{
			Method: "GET",
			Path:   path("/records"),
			Scope:  "ip",
			Limit:  limit(cfg.RateLimit.RecordListIPRPM),
			Window: window,
			Key:    httpmiddleware.ClientIPKey,
		},
		{
			Method: "GET",
			Path:   path("/records/count-by-conference"),
			Scope:  "ip",
			Limit:  limit(cfg.RateLimit.RecordListIPRPM),
			Window: window,
			Key:    httpmiddleware.ClientIPKey,
		},
		{
			Method: "GET",
			Path:   path("/records/:id"),
			Scope:  "ip",
			Limit:  limit(cfg.RateLimit.RecordReadIPRPM),
			Window: window,
			Key:    httpmiddleware.ClientIPKey,
		},
	}

	return httpmiddleware.RateLimitConfig{
		Enabled: cfg.RateLimitEnabled,
		Rules:   rules,
	}
}

// RunMigrations применяет миграции без запуска API.
// Параметры: нет.
// Возвращает: ошибку подключения или миграции.
func RunMigrations() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	db, err := postgresinfra.Connect(cfg.PostgresDSN)
	if err != nil {
		return err
	}

	return postgresinfra.RunMigrations(db, "database/migrations")
}
