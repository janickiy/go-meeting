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

	authapp "github.com/janickiy/meet-space/internal/app/auth"
	captionsapp "github.com/janickiy/meet-space/internal/app/captions"
	chatapp "github.com/janickiy/meet-space/internal/app/chat"
	conferencesapp "github.com/janickiy/meet-space/internal/app/conferences"
	contentapp "github.com/janickiy/meet-space/internal/app/content"
	engagementapp "github.com/janickiy/meet-space/internal/app/engagement"
	foldersapp "github.com/janickiy/meet-space/internal/app/folders"
	integrationsapp "github.com/janickiy/meet-space/internal/app/integrations"
	notificationsapp "github.com/janickiy/meet-space/internal/app/notifications"
	personalapp "github.com/janickiy/meet-space/internal/app/personal"
	platformapp "github.com/janickiy/meet-space/internal/app/platform"
	recordingsapp "github.com/janickiy/meet-space/internal/app/recordings"
	telemetryapp "github.com/janickiy/meet-space/internal/app/telemetry"
	"github.com/janickiy/meet-space/internal/config"
	healthinfra "github.com/janickiy/meet-space/internal/infrastructure/health"
	postgresinfra "github.com/janickiy/meet-space/internal/infrastructure/postgres"
	rabbitmqinfra "github.com/janickiy/meet-space/internal/infrastructure/rabbitmq"
	redisinfra "github.com/janickiy/meet-space/internal/infrastructure/redis"
	"github.com/janickiy/meet-space/internal/infrastructure/security"
	s3storage "github.com/janickiy/meet-space/internal/infrastructure/storage/s3"
	"github.com/janickiy/meet-space/internal/operations"
	httptransport "github.com/janickiy/meet-space/internal/transport/http"
	httpmiddleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
	wstransport "github.com/janickiy/meet-space/internal/transport/websocket"
	authusecase "github.com/janickiy/meet-space/internal/usecase/auth"
	chatusecase "github.com/janickiy/meet-space/internal/usecase/chat"
	conferenceusecase "github.com/janickiy/meet-space/internal/usecase/conferences"
	foldersusecase "github.com/janickiy/meet-space/internal/usecase/folders"
	mediausecase "github.com/janickiy/meet-space/internal/usecase/media"
	notificationsusecase "github.com/janickiy/meet-space/internal/usecase/notifications"
	personalusecase "github.com/janickiy/meet-space/internal/usecase/personal"
	platformusecase "github.com/janickiy/meet-space/internal/usecase/platform"
	realtimeusecase "github.com/janickiy/meet-space/internal/usecase/realtime"
	"github.com/janickiy/meet-space/internal/usecase/recorder"
	recordingsusecase "github.com/janickiy/meet-space/internal/usecase/recordings"
)

// RunAPI запускает HTTP API.
// @return ошибку bootstrap или HTTP server-а.
func RunAPI() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	// Проверяем только при запуске API: recorder-worker и команда миграций не выпускают JWT.
	tokens, err := security.NewTokenService(cfg.JWTSecret)
	if err != nil {
		return err
	}
	realtimeConfig, err := config.LoadRealtime()
	if err != nil {
		return err
	}
	mediaConfig, err := config.LoadMedia()
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
	if err := postgresinfra.RunStartupMigrations(db, "database/migrations"); err != nil {
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
	service := recorder.NewService(repository, commandPublisher, s3Client, conferenceLock)
	sqlDB.SetMaxOpenConns(cfg.Operations.DBMaxOpen)
	sqlDB.SetMaxIdleConns(cfg.Operations.DBMaxIdle)
	sqlDB.SetConnMaxLifetime(cfg.Operations.DBLifetime)
	ops := operations.New("api", hostname(), cfg.Operations, map[string]operations.Check{
		"postgres": sqlDB.PingContext,
		"redis":    func(ctx context.Context) error { return redisClient.Ping(ctx).Err() },
		"minio":    s3Client.Check,
		"rabbitmq": commandPublisher.Check,
	})
	router := httptransport.NewRouter(ops.Middleware(), httpmiddleware.RateLimit(rateLimiter, rateLimitConfig(cfg)))
	ops.RegisterGin(router)
	httptransport.RegisterClientErrorRoutes(router, telemetryapp.NewHandler(os.Getenv("CLIENT_TELEMETRY_ENABLED") == "true", slog.Default(), ops.Registry), rateLimiter)
	if err := router.SetTrustedProxies(cfg.TrustedProxies); err != nil {
		return fmt.Errorf("HTTP_TRUSTED_PROXIES: %w", err)
	}
	users := postgresinfra.NewUserRepository(db)
	authService, err := authusecase.NewService(users, security.PasswordHasher{}, tokens)
	if err != nil {
		return err
	}
	authService.WithSessions(postgresinfra.NewAuthSessionRepository(db))
	conferenceRepository := postgresinfra.NewConferenceRepository(db)
	conferenceService := conferenceusecase.NewService(conferenceRepository, users, security.GenerateInviteCode)
	store := redisinfra.NewRealtimeStore(redisClient, realtimeConfig.Namespace)
	hub, err := realtimeusecase.NewHub(postgresinfra.NewSessionRepository(db), store, realtimeConfig.SessionTTL, logger)
	if err != nil {
		return err
	}
	defer hub.Shutdown()
	ops.Checks["realtime"] = hub.Check
	engagementService := realtimeusecase.NewEngagement(postgresinfra.NewSessionRepository(db), hub)
	notificationBus := redisinfra.NewNotificationBus(redisClient, realtimeConfig.Namespace)
	notificationService := notificationsusecase.NewService(postgresinfra.NewNotificationRepository(db).DisableLegacyReminders(), notificationBus)
	conferenceChatRepository := postgresinfra.NewChatRepository(db)
	chatService, err := chatusecase.NewService(context.Background(), conferenceChatRepository, s3Client, hub)
	if err != nil {
		return fmt.Errorf("chat initialization: %w", err)
	}
	personalRepository := postgresinfra.NewPersonalRepository(db)
	personalEvents := &personalusecase.Events{Members: personalRepository, Bus: notificationBus}
	personalAssets, err := personalusecase.NewAssetService(context.Background(), postgresinfra.NewPersonalAssetRepository(db), s3Client, personalEvents)
	if err != nil {
		return fmt.Errorf("personal assets initialization: %w", err)
	}
	personalChat, err := chatusecase.NewService(context.Background(), postgresinfra.NewDirectChatRepository(db), s3Client, personalEvents)
	if err != nil {
		return fmt.Errorf("personal chat initialization: %w", err)
	}
	userPresence := redisinfra.NewUserPresence(redisClient, realtimeConfig.Namespace, realtimeConfig.SessionTTL)
	globalWS := wstransport.NewUserHandler(authService, redisinfra.NewRealtimeStore(redisClient, realtimeConfig.Namespace+":user-ws"), rateLimiter, realtimeConfig, notificationBus, userPresence, personalRepository)
	defer globalWS.Shutdown()
	globalWS.RegisterRoutes(router)
	personalHandler := &personalapp.Handler{Repo: personalRepository, Events: personalEvents, Presence: userPresence}
	httptransport.RegisterPersonalRoutes(router, personalHandler, chatapp.NewHandler(personalChat).ForConversations().WithGroupDownloads(personalAssets), httpmiddleware.Authenticate(authService), rateLimiter)
	httptransport.RegisterPersonalAssetRoutes(router, personalapp.NewAssetHandler(personalAssets), httpmiddleware.Authenticate(authService), personalHandler.AccountOnly, rateLimiter)
	folderHandler := foldersapp.NewHandler(postgresinfra.NewFolderRepository(db), &foldersusecase.Events{Bus: notificationBus})
	httptransport.RegisterFolderRoutes(router, folderHandler, httpmiddleware.Authenticate(authService), rateLimiter)
	mediaTickets, err := security.NewMediaTickets(mediaConfig.TicketSecret, mediaConfig.TicketTTL)
	if err != nil {
		return err
	}
	mediaClient := mediausecase.NewHTTPClient(mediaConfig.InternalSecret, mediaConfig.OperationTimeout)
	defer mediaClient.Close()
	mediaController := mediausecase.NewController(redisinfra.NewMediaRegistry(redisClient, mediaConfig.Namespace), mediaTickets, mediaClient, hub, store, mediaConfig, realtimeConfig)
	mediaController.SetPolicyProvider(conferenceRepository)
	controlService := conferenceusecase.NewControlService(conferenceRepository, mediaController, hub)
	recordingRepository := postgresinfra.NewConferenceRecordingRepository(db)
	recordingService := recordingsusecase.NewConferenceService(recordingRepository, service, hub)
	recordingCommands := recordingsusecase.NewCommandDispatcher(recordingRepository, commandPublisher, conferenceLock, hub)
	hub.SetDisconnectObserver(realtimeusecase.DisconnectObservers{controlService, mediaController})
	conferenceService.SetObserver(hub)
	guestService := &conferenceusecase.GuestService{Repository: postgresinfra.NewGuestRepository(db), Tokens: tokens, Observer: hub}
	httptransport.RegisterGuestRoutes(router, &conferencesapp.GuestHandler{Service: guestService, Tokens: authService}, rateLimiter)
	httptransport.RegisterPlatformRoutes(router, authapp.NewHandler(authService).WithSessionCookies(cfg.StageSeven.PublicURL, !cfg.IsLocal()), conferencesapp.NewHandler(conferenceService), httpmiddleware.Authenticate(authService))
	httptransport.RegisterControlRoutes(router, conferencesapp.NewControlHandler(controlService), httpmiddleware.Authenticate(authService))
	httptransport.RegisterConferenceRecordingRoutes(router, recordingsapp.NewHandler(recordingService), httpmiddleware.Authenticate(authService))
	httptransport.RegisterChatRoutes(router, chatapp.NewHandler(chatService), httpmiddleware.Authenticate(authService), rateLimiter)
	httptransport.RegisterChatActionRoutes(router, &chatapp.ActionHandler{Service: chatusecase.NewActionService(conferenceChatRepository, hub).WithPresence(userPresence, hub)}, httpmiddleware.Authenticate(authService), rateLimiter)
	notificationHandler := notificationsapp.NewHandler(notificationService, notificationBus, authService, rateLimiter, realtimeConfig.Namespace)
	ops.ConfigureDrain(func() { notificationHandler.BeginDrain(); globalWS.BeginDrain(true) }, func() int { return hub.LocalCount() + globalWS.LocalCount() })
	httptransport.RegisterNotificationRoutes(router, notificationHandler, httpmiddleware.Authenticate(authService))
	httptransport.RegisterEngagementRoutes(router, engagementapp.NewHandler(engagementService, rateLimiter, realtimeConfig.Namespace), httpmiddleware.Authenticate(authService))
	product, err := newProductServices(cfg, db, s3Client)
	if err != nil {
		return fmt.Errorf("product initialization failed: %w", err)
	}
	httptransport.RegisterContentRoutes(router, contentapp.NewHandler(product.content), httpmiddleware.Authenticate(authService), rateLimiter)
	httptransport.RegisterCaptionRoutes(router, &captionsapp.Handler{Repo: postgresinfra.NewCaptionsRepository(db), Enabled: cfg.StageEight.LiveEnabled, Config: cfg.StageEight, Analytics: postgresinfra.NewAnalyticsRepository(db), Search: postgresinfra.NewSearchRepository(db)}, httpmiddleware.Authenticate(authService), rateLimiter)
	httptransport.RegisterIntegrationRoutes(router, integrationsapp.NewHandler(product.integrations), httpmiddleware.Authenticate(authService), rateLimiter)
	httptransport.RegisterInvitationRoutes(router, &conferencesapp.InvitationHandler{Service: &conferenceusecase.InvitationService{Repository: postgresinfra.NewConferenceInvitationRepository(db)}}, httpmiddleware.Authenticate(authService), rateLimiter)
	mediaReady := healthinfra.NewHTTPReady(mediaConfig.WorkerInternalURL)
	defer mediaReady.Close()
	platformRepository := postgresinfra.NewPlatformRepository(db)
	platformService := &platformusecase.Service{Repo: platformRepository, Vector: postgresinfra.NewSearchRepository(db), MediaWorker: mediaReady, APIReady: ops.Ready, DependencyStatuses: ops.DependencyStatuses, StageSeven: cfg.StageSeven, StageEight: cfg.StageEight}
	httptransport.RegisterPlatformStatusRoutes(router, &platformapp.Handler{Service: platformService, BuildVersion: platformusecase.BuildVersion()}, httpmiddleware.Authenticate(authService), platformRepository)
	wstransport.NewHandler(hub, authService, store, rateLimiter, realtimeConfig).SetMedia(mediaController).RegisterRoutes(router)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	ops.Run(ctx)
	go runProfiling(ctx, ops)
	go sampleDatabase(ctx, sqlDB)
	go controlService.Run(ctx)
	go recordingCommands.Run(ctx)
	go recorder.RunRetention(ctx, repository, s3Client)
	go chatService.Run(ctx)
	go personalChat.Run(ctx)
	go personalAssets.Run(ctx)
	go notificationService.Run(ctx)
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.APIPort))
	if err != nil {
		return err
	}
	server := &http.Server{Handler: router, BaseContext: /* Вложенный обработчик выполняет выделенный шаг обработки в сборке и запуске компонентов приложения, используя состояние окружающей функции.

	@args
	  - аргумент 1 (net.Listener): значение для проверки, нормализации или преобразования.

	@return:
	  - результат 1 (context.Context): значение, подготовленное операцией для вызывающей стороны. */func(net.Listener) context.Context { return ctx }, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 * 1024}
	failed := make(chan error, 1)
	server.ReadTimeout = cfg.Operations.HTTPReadTimeout
	go /* Вложенный обработчик выполняет выделенный шаг обработки в сборке и запуске компонентов приложения, используя состояние окружающей функции.

	 */func() { failed <- server.Serve(listener) }()
	select {
	case err := <-failed:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
	}
	// Переданные приложению соединения WebSocket не закрываются вызовом http.Server.Shutdown.
	ops.Drain()
	notificationHandler.BeginDrain()
	_ = listener.Close()
	shutdown, cancel := context.WithTimeout(context.Background(), cfg.Operations.ShutdownTimeout)
	defer cancel()
	hubErr := hub.ShutdownContext(shutdown)
	err = errors.Join(hubErr, server.Shutdown(shutdown))
	if errors.Is(err, net.ErrClosed) || errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// rateLimitConfig преобразует общие настройки приложения в конфигурацию HTTP-ограничителя.
//
// @args
//   - cfg (config.Config): проверенные настройки соответствующего компонента.
//
// @return:
//   - результат 1 (httpmiddleware.RateLimitConfig): значение, подготовленное операцией для вызывающей стороны.
func rateLimitConfig(cfg config.Config) httpmiddleware.RateLimitConfig {
	window := cfg.RateLimitWindow
	defaultLimit := cfg.RateLimit.DefaultRPM
	// Вложенный обработчик выполняет выделенный шаг обработки в сборке и запуске компонентов приложения, используя состояние окружающей функции.
	//
	// @args
	//   - value (int): значение для проверки, нормализации или преобразования.
	//
	// @return:
	//   - результат 1 (int): значение, подготовленное операцией для вызывающей стороны.
	limit := func(value int) int {
		if value > 0 {
			return value
		}

		return defaultLimit
	}
	// Вложенный обработчик выполняет выделенный шаг обработки в сборке и запуске компонентов приложения, используя состояние окружающей функции.
	//
	// @args
	//   - route (string): адрес и версия действующего владельца медиа-комнаты.
	//
	// @return:
	//   - результат 1 (string): значение, подготовленное операцией для вызывающей стороны.
	path := func(route string) string {
		return httptransport.APIV1Prefix + route
	}
	rules := []httpmiddleware.Rule{
		{Method: "GET", Path: path("/conference-invites/:code"), Scope: "invite_lookup_ip", Limit: 60, Window: window, Key: httpmiddleware.ClientIPKey},
		{Method: "POST", Path: path("/auth/login"), Scope: "ip", Limit: limit(cfg.RateLimit.AuthLoginIPRPM), Window: window, Key: httpmiddleware.ClientIPKey},
		{Method: "POST", Path: path("/auth/register"), Scope: "ip", Limit: limit(cfg.RateLimit.AuthRegisterIPRPM), Window: window, Key: httpmiddleware.ClientIPKey},
	}

	return httpmiddleware.RateLimitConfig{
		Enabled: cfg.RateLimitEnabled,
		Rules:   rules,
	}
}

// RunMigrations применяет миграции без запуска API.
// @args нет.
// @return ошибку подключения или миграции.
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
