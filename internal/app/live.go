package app

import (
	"context"
	"errors"
	"fmt"
	"github.com/janickiy/go-recorder/internal/config"
	"github.com/janickiy/go-recorder/internal/infrastructure/ffmpeg"
	"github.com/janickiy/go-recorder/internal/infrastructure/liveproviders"
	pg "github.com/janickiy/go-recorder/internal/infrastructure/postgres"
	redisinfra "github.com/janickiy/go-recorder/internal/infrastructure/redis"
	"github.com/janickiy/go-recorder/internal/operations"
	live "github.com/janickiy/go-recorder/internal/usecase/captions"
	"net/http"
	"os/signal"
	"syscall"
	"time"
)

// RunLiveWorker изолирует decoder/STT в отдельном процессе с независимыми ресурсными пределами.
// @return ошибка запуска/завершения без содержимого речи или токенов.
func RunLiveWorker() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	mediaCfg, err := config.LoadMedia()
	if err != nil {
		return err
	}
	rt, err := config.LoadRealtime()
	if err != nil {
		return err
	}
	db, err := pg.Connect(cfg.PostgresDSN)
	if err != nil {
		return err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	defer sqlDB.Close()
	sqlDB.SetMaxOpenConns(cfg.Operations.DBMaxOpen)
	sqlDB.SetMaxIdleConns(cfg.Operations.DBMaxIdle)
	sqlDB.SetConnMaxLifetime(cfg.Operations.DBLifetime)
	if err = pg.RunStartupMigrations(db, "database/migrations"); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	op, cancel := context.WithTimeout(ctx, 5*time.Second)
	client, err := redisinfra.NewClient(op, cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	cancel()
	if err != nil {
		return err
	}
	defer client.Close()
	ops := operations.New("live-worker", hostname(), cfg.Operations, map[string]operations.Check{"postgres": sqlDB.PingContext, "redis": func(ctx context.Context) error { return client.Ping(ctx).Err() }})
	ops.Run(ctx)
	go runProfiling(ctx, ops)
	go sampleDatabase(ctx, sqlDB)
	c := cfg.StageEight
	worker := &live.Worker{Repo: pg.NewCaptionsRepository(db), Tap: liveproviders.NewTap(redisinfra.NewMediaRegistry(client, mediaCfg.Namespace), mediaCfg.InternalSecret), Decoder: ffmpeg.LiveAudio{Binary: cfg.FFmpegPath}, Provider: liveproviders.Provider{Mode: c.Live.Mode, Endpoint: c.Live.Endpoint, Token: c.Live.Token, Timeout: c.ProviderTimeout}, Events: redisinfra.NewRealtimeStore(client, rt.Namespace), Config: c}
	done := make(chan struct{})
	ops.ConfigureDrain(worker.BeginDrain, worker.Active)
	go func() { defer close(done); worker.Run(ctx) }()
	mux := http.NewServeMux()
	ops.Register(mux)
	server := &http.Server{Addr: fmt.Sprintf(":%d", c.LivePort), Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second}
	failed := make(chan error, 1)
	go func() { failed <- server.ListenAndServe() }()
	var serveErr error
	select {
	case <-ctx.Done():
	case serveErr = <-failed:
	}
	ops.Drain()
	stop()
	shutdown, finish := context.WithTimeout(context.Background(), cfg.Operations.ShutdownTimeout)
	defer finish()
	err = server.Shutdown(shutdown)
	select {
	case <-done:
	case <-shutdown.Done():
		return errors.Join(err, shutdown.Err())
	}
	if errors.Is(serveErr, http.ErrServerClosed) {
		serveErr = nil
	}
	return errors.Join(err, serveErr)
}
