package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/janickiy/go-recorder/internal/config"
	"github.com/janickiy/go-recorder/internal/infrastructure/contentproviders"
	"github.com/janickiy/go-recorder/internal/infrastructure/ffmpeg"
	pg "github.com/janickiy/go-recorder/internal/infrastructure/postgres"
	"github.com/janickiy/go-recorder/internal/infrastructure/providers"
	"github.com/janickiy/go-recorder/internal/infrastructure/security"
	s3 "github.com/janickiy/go-recorder/internal/infrastructure/storage/s3"
	"github.com/janickiy/go-recorder/internal/operations"
	content "github.com/janickiy/go-recorder/internal/usecase/content"
	integrations "github.com/janickiy/go-recorder/internal/usecase/integrations"
	jobrunner "github.com/janickiy/go-recorder/internal/usecase/jobs"
	"gorm.io/gorm"
)

// productServices объединяет продуктовые сценарии, не изменяя жизненный цикл RTP/FFmpeg записи.
type productServices struct {
	content      *content.Service
	integrations *integrations.Service
}

// newProductServices создаёт одни и те же проверенные зависимости API и отдельного worker.
// API использует только чтение/постановку задач; аудио и провайдеры вызывает worker.
// @parameters cfg — серверная конфигурация; db — ограниченный SQL-пул; storage — приватный MinIO.
// @return сценарии продукта либо безопасная ошибка конфигурации.
func newProductServices(cfg config.Config, db *gorm.DB, storage *s3.Client) (productServices, error) {
	c := cfg.StageSeven
	local := cfg.IsLocal() || cfg.AppEnv == "test"
	adapter := func(p config.ProviderConfig) providers.AdapterConfig {
		return providers.AdapterConfig{Mode: p.Mode, Endpoint: p.Endpoint, Secret: p.Token, Timeout: c.ProviderTimeout, AllowHTTP: local}
	}
	channels, err := providers.NewIntegrations(providers.IntegrationConfig{Email: adapter(c.Email), Push: adapter(c.Push), Calendar: adapter(c.Calendar), MockConnectAllowed: local,
		OAuth: providers.OAuthConfig{AuthorizationURL: c.OAuthAuthURL, TokenURL: c.OAuthTokenURL, RevokeURL: c.OAuthRevokeURL, ClientID: c.OAuthClientID, ClientSecret: c.OAuthClientSecret, RedirectURL: c.OAuthRedirectURL, Scopes: c.OAuthScopes, Timeout: c.ProviderTimeout, AllowHTTP: local}})
	if err != nil {
		return productServices{}, err
	}
	channels.Email = measuredEmail{channels.Email}
	channels.Push = measuredPush{channels.Push}
	channels.Calendar = measuredCalendar{channels.Calendar}
	tokenCipher, err := security.NewProviderTokens(c.EncryptionKey)
	if err != nil {
		return productServices{}, err
	}
	var cipher integrations.TokenCipher
	if tokenCipher != nil {
		cipher = tokenCipher
	}
	integrationService, err := integrations.NewService(pg.NewIntegrationRepository(db), channels, cipher, integrations.Options{PublicURL: c.PublicURL, ReminderOffsets: c.ReminderOffsets, ProviderTimeout: c.ProviderTimeout, MockConnectAllowed: local})
	if err != nil {
		return productServices{}, err
	}
	stt, err := contentproviders.NewTranscriptionProvider(c.STT.Mode, c.STT.Endpoint, c.STT.Token, c.STTTimeout)
	if err != nil {
		return productServices{}, err
	}
	ai, err := contentproviders.NewAIProvider(c.AI.Mode, c.AI.Endpoint, c.AI.Token, c.AIModel, c.AITimeout)
	if err != nil {
		return productServices{}, err
	}
	audio := ffmpeg.NewTranscriptionAudio(storage, cfg.FFmpegPath, c.TempRoot, c.MaxVideoBytes, c.MaxAudioBytes, c.MaxDurationSec)
	contentService, err := content.NewService(pg.NewContentRepository(db), audio, measuredSTT{stt}, measuredAI{ai}, content.Config{TranscriptionEnabled: c.STTEnabled, AIEnabled: c.AIEnabled, MaxDurationSec: c.MaxDurationSec, MaxSegments: c.MaxSegments, MaxTranscriptBytes: c.MaxTranscriptBytes,
		ChunkRunes: c.ChunkRunes, MaxChunks: c.MaxChunks, AIConcurrency: c.AIConcurrency, STTTimeout: c.STTTimeout, AITimeout: c.AITimeout, ReprocessCooldown: c.ReprocessCooldown, MaxReprocess: c.MaxReprocess, MaxAttempts: c.MaxAttempts})
	return productServices{contentService, integrationService}, err
}

// RunProductWorker запускает независимый процесс медленных внешних операций с постоянными очередями.
// Отмена останавливает новые захваты, прерывает provider requests и ограничивает финальную запись.
// @return ошибка конфигурации/старта либо nil после корректного завершения.
func RunProductWorker() error {
	cfg, err := config.Load()
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
	if err := pg.RunMigrations(db, "database/migrations"); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	storage, err := s3.NewClient(ctx, cfg.MinIOEndpoint, cfg.MinIOAccessKey, cfg.MinIOSecretKey, cfg.MinIOBucket, cfg.MinIOUseSSL)
	if err != nil {
		return err
	}
	if cfg.StageSeven.STTEnabled {
		privacyCtx, cancel := context.WithTimeout(ctx, cfg.Operations.ProbeTimeout)
		err = storage.CheckAttachmentPrivacy(privacyCtx)
		cancel()
		if err != nil {
			return errors.New("product worker requires a private recording bucket")
		}
	}
	services, err := newProductServices(cfg, db, storage)
	if err != nil {
		return err
	}
	checks := map[string]operations.Check{"postgres": sqlDB.PingContext}
	if cfg.StageSeven.STTEnabled {
		checks["minio"] = func(ctx context.Context) error {
			if err := storage.Check(ctx); err != nil {
				return err
			}
			return storage.CheckAttachmentPrivacy(ctx)
		}
	}
	ops := operations.New("product-worker", hostname(), cfg.Operations, checks)
	ops.Run(ctx)
	go runProfiling(ctx, ops)
	go sampleDatabase(ctx, sqlDB)
	repo := pg.NewJobRepository(db)
	i := jobrunner.Handler{Handle: services.integrations.Handle, Fail: services.integrations.FailJob}
	c := jobrunner.Handler{Handle: services.content.Handle, Fail: services.content.FailJob}
	s := cfg.StageSeven
	runner, err := jobrunner.New(repo, []jobrunner.Pool{
		{Kind: "integrations.conference", Concurrency: 1, MaxAttempts: s.MaxAttempts, Timeout: s.ProviderTimeout, Handler: i}, {Kind: "integrations.event", Concurrency: 1, MaxAttempts: s.MaxAttempts, Timeout: s.ProviderTimeout, Handler: i},
		{Kind: "integrations.delivery", Concurrency: s.DeliveryWorkers, MaxAttempts: s.MaxAttempts, Timeout: s.ProviderTimeout, Handler: i}, {Kind: "integrations.calendar", Concurrency: s.CalendarWorkers, MaxAttempts: s.MaxAttempts, Timeout: s.ProviderTimeout * 3, Handler: i},
		{Kind: "content.transcribe", Concurrency: s.STTWorkers, MaxAttempts: s.MaxAttempts, Timeout: s.STTTimeout, Handler: c}, {Kind: "content.summarize", Concurrency: s.AIWorkers, MaxAttempts: s.MaxAttempts, Timeout: s.AITimeout, Handler: c},
	}, s.PollInterval, operations.Product)
	if err != nil {
		return err
	}
	workerDone := make(chan struct{})
	go func() { defer close(workerDone); runner.Run(ctx) }()
	tickDone := make(chan struct{})
	go func() { defer close(tickDone); runProductTicks(ctx, services.integrations, repo, s.PollInterval) }()
	mux := http.NewServeMux()
	ops.Register(mux)
	server := &http.Server{Addr: fmt.Sprintf(":%d", s.Port), Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 * 1024}
	failed := make(chan error, 1)
	go func() { failed <- server.ListenAndServe() }()
	var serveErr error
	select {
	case <-ctx.Done():
	case serveErr = <-failed:
	}
	ops.Drain()
	stop()
	shutdown, cancel := context.WithTimeout(context.Background(), cfg.Operations.ShutdownTimeout)
	defer cancel()
	serverErr := server.Shutdown(shutdown)
	for _, done := range []<-chan struct{}{workerDone, tickDone} {
		select {
		case <-done:
		case <-shutdown.Done():
			return errors.Join(serverErr, shutdown.Err())
		}
	}
	if errors.Is(serveErr, http.ErrServerClosed) {
		serveErr = nil
	}
	return errors.Join(serveErr, serverErr)
}

// runProductTicks создаёт due-reminders и снимает размер очереди одним общим таймером.
// @parameters ctx — время жизни; service — scheduler интеграций; repo — очередь; interval — период опроса.
func runProductTicks(ctx context.Context, service *integrations.Service, repo *pg.JobRepository, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for ctx.Err() == nil {
		tickCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		if err := service.Tick(tickCtx); err != nil && ctx.Err() == nil {
			slog.Warn("product scheduler unavailable", "event_type", "product.scheduler.failed")
		}
		counts, err := repo.Counts(tickCtx)
		cancel()
		if err == nil {
			for _, kind := range []string{"integrations.conference", "integrations.event", "integrations.delivery", "integrations.calendar", "content.transcribe", "content.summarize"} {
				for _, state := range []string{"queued", "processing", "failed"} {
					operations.ProductQueue(kind, state, 0)
				}
			}
			for _, count := range counts {
				operations.ProductQueue(count.Kind, count.State, count.Count)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
