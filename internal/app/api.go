package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	authapp "github.com/janickiy/go-recorder/internal/app/auth"
	conferencesapp "github.com/janickiy/go-recorder/internal/app/conferences"
	recordsapp "github.com/janickiy/go-recorder/internal/app/records"
	"github.com/janickiy/go-recorder/internal/config"
	postgresinfra "github.com/janickiy/go-recorder/internal/infrastructure/postgres"
	rabbitmqinfra "github.com/janickiy/go-recorder/internal/infrastructure/rabbitmq"
	redisinfra "github.com/janickiy/go-recorder/internal/infrastructure/redis"
	"github.com/janickiy/go-recorder/internal/infrastructure/security"
	s3storage "github.com/janickiy/go-recorder/internal/infrastructure/storage/s3"
	workerinfra "github.com/janickiy/go-recorder/internal/infrastructure/worker"
	httptransport "github.com/janickiy/go-recorder/internal/transport/http"
	httpmiddleware "github.com/janickiy/go-recorder/internal/transport/http/middleware"
	wstransport "github.com/janickiy/go-recorder/internal/transport/websocket"
	authusecase "github.com/janickiy/go-recorder/internal/usecase/auth"
	conferenceusecase "github.com/janickiy/go-recorder/internal/usecase/conferences"
	realtimeusecase "github.com/janickiy/go-recorder/internal/usecase/realtime"
	"github.com/janickiy/go-recorder/internal/usecase/recorder"
)

// RunAPI запускает HTTP API.
// Параметры: нет.
// Возвращает: ошибку bootstrap или HTTP server-а.
func RunAPI() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	// Validate only in API bootstrap: recorder-worker/migrate do not issue JWTs.
	tokens, err := security.NewTokenService(cfg.JWTSecret)
	if err != nil {
		return err
	}
	realtimeConfig, err := config.LoadRealtime()
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)
	db, err := postgresinfra.Connect(cfg.PostgresDSN)
	if err != nil {
		return err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	defer sqlDB.Close()
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
	if err := router.SetTrustedProxies(cfg.TrustedProxies); err != nil {
		return fmt.Errorf("HTTP_TRUSTED_PROXIES: %w", err)
	}
	users := postgresinfra.NewUserRepository(db)
	authService, err := authusecase.NewService(users, security.PasswordHasher{}, tokens)
	if err != nil {
		return err
	}
	conferenceService := conferenceusecase.NewService(postgresinfra.NewConferenceRepository(db), users, security.GenerateInviteCode)
	store := redisinfra.NewRealtimeStore(redisClient, realtimeConfig.Namespace)
	hub, err := realtimeusecase.NewHub(postgresinfra.NewSessionRepository(db), store, realtimeConfig.SessionTTL, logger)
	if err != nil {
		return err
	}
	defer hub.Shutdown()
	conferenceService.SetObserver(hub)
	httptransport.RegisterPlatformRoutes(router, authapp.NewHandler(authService), conferencesapp.NewHandler(conferenceService), httpmiddleware.Authenticate(tokens))
	wstransport.NewHandler(hub, tokens, store, rateLimiter, realtimeConfig).RegisterRoutes(router)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.APIPort))
	if err != nil {
		return err
	}
	server := &http.Server{Handler: router, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 * 1024}
	failed := make(chan error, 1)
	go func() { failed <- server.Serve(listener) }()
	select {
	case err := <-failed:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
	}
	// Hijacked WebSockets are not closed by http.Server.Shutdown.
	_ = listener.Close()
	hub.Shutdown()
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err = server.Shutdown(shutdown)
	if errors.Is(err, net.ErrClosed) || errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
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
		{Method: "POST", Path: path("/auth/login"), Scope: "ip", Limit: limit(cfg.RateLimit.AuthLoginIPRPM), Window: window, Key: httpmiddleware.ClientIPKey},
		{Method: "POST", Path: path("/auth/register"), Scope: "ip", Limit: limit(cfg.RateLimit.AuthRegisterIPRPM), Window: window, Key: httpmiddleware.ClientIPKey},
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
