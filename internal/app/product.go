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
	analyticsusecase "github.com/janickiy/go-recorder/internal/usecase/analytics"
	content "github.com/janickiy/go-recorder/internal/usecase/content"
	integrations "github.com/janickiy/go-recorder/internal/usecase/integrations"
	jobrunner "github.com/janickiy/go-recorder/internal/usecase/jobs"
	searchusecase "github.com/janickiy/go-recorder/internal/usecase/search"
	"gorm.io/gorm"
)

// productServices объединяет продуктовые сценарии, не изменяя жизненный цикл RTP/FFmpeg записи.
type productServices struct {
	content      *content.Service
	integrations *integrations.Service
	search       *searchusecase.Service
}

// newProductServices создаёт одни и те же проверенные зависимости API и отдельного worker.
// API использует только чтение/постановку задач; аудио и провайдеры вызывает worker.
// @args cfg — серверная конфигурация; db — ограниченный SQL-пул; storage — приватный MinIO.
// @return сценарии продукта либо безопасная ошибка конфигурации.
func newProductServices(cfg config.Config, db *gorm.DB, storage *s3.Client) (productServices, error) {
	c := cfg.StageSeven
	local := cfg.IsLocal() || cfg.AppEnv == "test"
	adapter := func(p config.ProviderConfig) providers.AdapterConfig {
		return providers.AdapterConfig{Mode: p.Mode, Endpoint: p.Endpoint, Secret: p.Token, Timeout: c.ProviderTimeout, AllowHTTP: local}
	}
	channels, err := providers.NewIntegrations(providers.IntegrationConfig{Email: adapter(c.Email), Push: adapter(c.Push), Calendar: adapter(c.Calendar), MockConnectAllowed: local,
		SMTP:  providers.SMTPConfig{Host: c.SMTP.Host, Port: c.SMTP.Port, Username: c.SMTP.Username, Password: c.SMTP.Password, From: c.SMTP.From, TLSMode: c.SMTP.TLSMode, Timeout: c.SMTP.Timeout},
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
	deliveryTimeout := c.ProviderTimeout
	if c.Email.Mode == "smtp" && c.SMTP.Timeout > deliveryTimeout {
		deliveryTimeout = c.SMTP.Timeout
	}
	integrationService, err := integrations.NewService(pg.NewIntegrationRepository(db), channels, cipher, integrations.Options{PublicURL: c.PublicURL, ReminderOffsets: c.ReminderOffsets, ProviderTimeout: deliveryTimeout, MockConnectAllowed: local})
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
	if err != nil {
		return productServices{}, err
	}
	embedCfg := cfg.StageEight
	embeddings, err := contentproviders.NewEmbeddingProvider(embedCfg.Embeddings.Mode, embedCfg.Embeddings.Endpoint, embedCfg.Embeddings.Token, embedCfg.EmbeddingTimeout)
	if err != nil {
		return productServices{}, err
	}
	searchService := searchusecase.New(pg.NewSearchRepository(db), embeddings, embedCfg)
	contentService.SetSearch(searchService)
	return productServices{contentService, integrationService, searchService}, nil
}

// RunProductWorker запускает независимый процесс медленных внешних операций с постоянными очередями.
// Отмена останавливает новые захваты, прерывает запросы к провайдеру и ограничивает финальную запись.
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
	if err := pg.RunStartupMigrations(db, "database/migrations"); err != nil {
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
	e := jobrunner.Handler{Handle: services.search.Handle, Fail: services.search.FailJob}
	analyticsRepo := pg.NewAnalyticsRepository(db)
	analyticsService := &analyticsusecase.Service{Repo: analyticsRepo, Enabled: cfg.StageEight.AnalyticsEnabled}
	a := jobrunner.Handler{Handle: analyticsService.Handle}
	s := cfg.StageSeven
	deliveryTimeout := s.ProviderTimeout
	if s.Email.Mode == "smtp" && s.SMTP.Timeout > deliveryTimeout {
		deliveryTimeout = s.SMTP.Timeout
	}
	runner, err := jobrunner.New(repo, []jobrunner.Pool{
		{Kind: "integrations.conference", Concurrency: 1, MaxAttempts: s.MaxAttempts, Timeout: s.ProviderTimeout, Handler: i}, {Kind: "integrations.event", Concurrency: 1, MaxAttempts: s.MaxAttempts, Timeout: s.ProviderTimeout, Handler: i},
		{Kind: "integrations.delivery", Concurrency: s.DeliveryWorkers, MaxAttempts: s.MaxAttempts, Timeout: deliveryTimeout, Handler: i},
		{Kind: "integrations.invitation", Concurrency: s.DeliveryWorkers, MaxAttempts: s.MaxAttempts, Timeout: deliveryTimeout, Handler: i}, {Kind: "integrations.calendar", Concurrency: s.CalendarWorkers, MaxAttempts: s.MaxAttempts, Timeout: s.ProviderTimeout * 3, Handler: i},
		{Kind: "content.transcribe", Concurrency: s.STTWorkers, MaxAttempts: s.MaxAttempts, Timeout: s.STTTimeout, Handler: c}, {Kind: "content.summarize", Concurrency: s.AIWorkers, MaxAttempts: s.MaxAttempts, Timeout: s.AITimeout, Handler: c},
		{Kind: "content.embed", Concurrency: cfg.StageEight.EmbeddingWorkers, MaxAttempts: s.MaxAttempts, Timeout: cfg.StageEight.EmbeddingTimeout, Handler: e},
		{Kind: "analytics.aggregate", Concurrency: 1, MaxAttempts: 3, Timeout: 15 * time.Second, Handler: a},
	}, s.PollInterval, operations.Product)
	if err != nil {
		return err
	}
	workerDone := make(chan struct{})
	ops.ConfigureDrain(runner.BeginDrain, runner.Active)
	go func() { defer close(workerDone); runner.Run(ctx) }()
	tickDone := make(chan struct{})
	go func() {
		defer close(tickDone)
		runProductTicks(ctx, services.integrations, repo, s.PollInterval, func(ctx context.Context) {
			if cfg.StageEight.AnalyticsEnabled {
				if analyticsRepo.Tick(ctx) != nil {
					operations.Event("analytics_failed")
				}
			}
		})
	}()
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

// runProductTicks создаёт наступившие напоминания и измеряет размер очереди одним общим таймером.
// @args ctx — время жизни; service — планировщик интеграций; repo — очередь; interval — период опроса.
func runProductTicks(ctx context.Context, service *integrations.Service, repo *pg.JobRepository, interval time.Duration, extra func(context.Context)) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for ctx.Err() == nil {
		tickCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		if extra != nil {
			extra(tickCtx)
		}
		if err := service.Tick(tickCtx); err != nil && ctx.Err() == nil {
			slog.Warn("product scheduler unavailable", "event_type", "product.scheduler.failed")
		}
		counts, err := repo.Counts(tickCtx)
		cancel()
		if err == nil {
			for _, kind := range []string{"integrations.conference", "integrations.event", "integrations.delivery", "integrations.invitation", "integrations.calendar", "content.transcribe", "content.summarize", "content.embed", "analytics.aggregate"} {
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
