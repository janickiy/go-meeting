package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/janickiy/go-recorder/internal/config"
	"github.com/janickiy/go-recorder/internal/domain/realtime"
	"github.com/janickiy/go-recorder/internal/domain/records"
	ffmpeginfra "github.com/janickiy/go-recorder/internal/infrastructure/ffmpeg"
	postgresinfra "github.com/janickiy/go-recorder/internal/infrastructure/postgres"
	rabbitmqinfra "github.com/janickiy/go-recorder/internal/infrastructure/rabbitmq"
	redisinfra "github.com/janickiy/go-recorder/internal/infrastructure/redis"
	s3storage "github.com/janickiy/go-recorder/internal/infrastructure/storage/s3"
	webrtcingest "github.com/janickiy/go-recorder/internal/infrastructure/webrtc"
	"github.com/janickiy/go-recorder/internal/operations"
	"github.com/janickiy/go-recorder/internal/usecase/recorder"
)

// RunWorker запускает recorder-worker.
// @args нет.
// @return ошибку bootstrap-а.
func RunWorker() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	mediaConfig, err := config.LoadMedia()
	if err != nil {
		return err
	}
	realtimeConfig, err := config.LoadRealtime()
	if err != nil {
		return err
	}
	compositeConfig, err := config.LoadComposite()
	if err != nil {
		return err
	}
	logger := log.New(operations.LogWriter{}, "", 0)
	if err := operations.ValidateStorage(cfg.StoragePath, cfg.Operations.DiskMinBytes); err != nil {
		return err
	}
	if _, err := exec.LookPath(cfg.FFmpegPath); err != nil {
		return fmt.Errorf("FFMPEG_PATH executable unavailable")
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	db, err := postgresinfra.Connect(cfg.PostgresDSN)
	if err != nil {
		return err
	}
	if err := postgresinfra.RunMigrations(db, "database/migrations"); err != nil {
		return err
	}
	s3Client, err := s3storage.NewClient(ctx, cfg.MinIOEndpoint, cfg.MinIOAccessKey, cfg.MinIOSecretKey, cfg.MinIOBucket, cfg.MinIOUseSSL)
	if err != nil {
		return err
	}

	repository := postgresinfra.NewRecordRepository(db)
	redisClient, err := redisinfra.NewClient(ctx, cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	if err != nil {
		return err
	}
	defer redisClient.Close()
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	defer sqlDB.Close()
	sqlDB.SetMaxOpenConns(cfg.Operations.DBMaxOpen)
	sqlDB.SetMaxIdleConns(cfg.Operations.DBMaxIdle)
	sqlDB.SetConnMaxLifetime(cfg.Operations.DBLifetime)
	conferenceLock := redisinfra.NewConferenceLock(redisClient, cfg.RecordLockTTL)
	ingest, err := webrtcingest.NewManager(webrtcingest.Options{
		StoragePath: cfg.StoragePath,
		MaxSessions: compositeConfig.MaxActive,
		FFmpegPath:  cfg.FFmpegPath,
		UDPPort:     cfg.WebRTCUDPPort,
		TCPPort:     cfg.WebRTCTCPPort,
		NATIPs:      cfg.WebRTCNATIPs,
		Logger:      logger,
		OnStarted: /* Вложенный обработчик выполняет выделенный шаг обработки в сборке и запуске компонентов приложения, используя состояние окружающей функции.

		@args
		  - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
		  - recordID (string): внешний UUID задачи записи.
		*/func(ctx context.Context, recordID string) {
			if err := repository.MarkRecording(ctx, recordID, cfg.WorkerID); err != nil && !errors.Is(err, records.ErrRecordStateChanged) {
				logger.Printf("mark record %s recording failed: %v", recordID, err)
			}
		},
		OnFailed: /* Вложенный обработчик выполняет выделенный шаг обработки в сборке и запуске компонентов приложения, используя состояние окружающей функции.

		@args
		  - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
		  - recordID (string): внешний UUID задачи записи.
		  - cause (error): значение cause типа error, используемое согласно назначению этой операции.
		*/func(ctx context.Context, recordID string, cause error) {
			if err := recorder.FailIngest(ctx, repository, conferenceLock, recordID, cfg.WorkerID, cause); err != nil {
				logger.Printf("handle ingest failure for record %s: %v", recordID, err)
			}
		},
	})
	if err != nil {
		return err
	}
	postProcessor := ffmpeginfra.NewPostProcessor(cfg.FFmpegPath)
	defer func() {
		shutdown, cancel := context.WithTimeout(context.Background(), cfg.Operations.ShutdownTimeout)
		defer cancel()
		_ = ingest.Shutdown(shutdown)
	}()
	service := recorder.NewWorkerService(repository, postProcessor, ingest, s3Client, cfg.StoragePath, cfg.WorkerID, conferenceLock)
	store := redisinfra.NewRealtimeStore(redisClient, realtimeConfig.Namespace)
	composites := recorder.NewCompositeService(recorder.CompositeOptions{
		Repository: repository, Registry: redisinfra.NewMediaRegistry(redisClient, mediaConfig.Namespace), S3: s3Client,
		StoragePath: cfg.StoragePath, FFmpegPath: cfg.FFmpegPath, WorkerID: cfg.WorkerID, InternalSecret: mediaConfig.InternalSecret,
		Config: compositeConfig, ConferenceLock: conferenceLock, Logger: logger,
		Publish: /* Вложенный обработчик выполняет выделенный шаг обработки в сборке и запуске компонентов приложения, используя состояние окружающей функции.

		@args
		  - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
		  - record (records.Record): задача записи с её сохранённым состоянием.
		  - kind (string): тип события, ошибки или медиа, определяющий ветку обработки.

		@return:
		  - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение. */func(ctx context.Context, record records.Record, kind string) error {
			status := record.Status
			if status == records.StatusFinalizing || status == records.StatusUploading {
				status = "processing"
			}
			event := realtime.Event(kind, record.ConferenceID, map[string]any{"recordingId": record.UUID, "conferenceId": record.ConferenceID, "status": status, "mode": record.Mode, "error": record.ErrorMessage})
			return store.Publish(ctx, realtime.Bus{Kind: "event", ConferenceID: record.ConferenceID, Event: &event})
		},
	})
	service.SetComposite(composites)
	composites.Start(ctx)
	defer /* Вложенный обработчик выполняет выделенный шаг обработки в сборке и запуске компонентов приложения, используя состояние окружающей функции.

	 */func() {
		stop()
		shutdown, cancel := context.WithTimeout(context.Background(), cfg.Operations.ShutdownTimeout)
		defer cancel()
		if err := composites.WaitContext(shutdown); err != nil {
			slog.Error("recorder shutdown timed out", "event_type", "service.shutdown_timeout")
		}
	}()
	consumer, err := rabbitmqinfra.NewConsumer(ctx, rabbitmqinfra.Options{
		URL:         cfg.RabbitMQURL,
		Exchange:    cfg.RabbitMQExchange,
		Queue:       cfg.RabbitMQQueue,
		RoutingKey:  cfg.RabbitMQRoutingKey,
		ConsumerTag: cfg.WorkerID,
		Logger:      logger,
	})
	if err != nil {
		return err
	}
	defer consumer.Close()
	ops := operations.New("recorder-worker", cfg.WorkerID, cfg.Operations, map[string]operations.Check{"postgres": sqlDB.PingContext, "redis": func(ctx context.Context) error { return redisClient.Ping(ctx).Err() }, "minio": s3Client.Check, "rabbitmq": consumer.Check, "disk": operations.DiskCheck(cfg.StoragePath, cfg.Operations.DiskMinBytes)})
	ops.Run(ctx)
	go runProfiling(ctx, ops)
	go sampleDatabase(ctx, sqlDB)
	go sampleRecorder(ctx, composites)

	errCh := make(chan error, 2)
	go /* Вложенный обработчик выполняет выделенный шаг обработки в сборке и запуске компонентов приложения, используя состояние окружающей функции.

	 */func() {
		errCh <- consumer.Consume(ctx, service.HandleCommand)
	}()
	httpDone := make(chan struct{})
	go func() { defer close(httpDone); errCh <- serveWorkerHTTP(ctx, cfg.WorkerPort, service, logger, ops) }()
	defer func() {
		ops.Drain()
		stop()
		select {
		case <-httpDone:
		case <-time.After(cfg.Operations.ShutdownTimeout + time.Second):
			slog.Error("HTTP drain timed out", "event_type", "service.shutdown_timeout")
		}
	}()
	logger.Printf("worker is ready; http=:%d storage=%s minio=%s/%s rabbitmq_queue=%s", cfg.WorkerPort, cfg.StoragePath, cfg.MinIOEndpoint, cfg.MinIOBucket, cfg.RabbitMQQueue)

	select {
	case <-ctx.Done():
		ops.Drain()
		return nil
	case err := <-errCh:
		if err != nil && ctx.Err() == nil {
			return err
		}

		return nil
	}
}

// serveWorkerHTTP запускает HTTP-сервер воркера и завершает его по отмене контекста.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - port (int): локальный сетевой порт передачи RTP.
//   - service (*recorder.WorkerService): значение service типа *recorder.WorkerService, используемое согласно назначению этой операции.
//   - logger (*log.Logger): значение logger типа *log.Logger, используемое согласно назначению этой операции.
func serveWorkerHTTP(ctx context.Context, port int, service *recorder.WorkerService, logger *log.Logger, ops *operations.Runtime) error {
	mux := http.NewServeMux()
	ops.Register(mux)
	mux.HandleFunc("/health", /* Вложенный обработчик выполняет выделенный шаг обработки в сборке и запуске компонентов приложения, используя состояние окружающей функции.

		@args
		  - w (http.ResponseWriter): получатель HTTP-ответа.
		  - _ (*http.Request): неиспользуемый аргумент, сохранённый для совместимости с контрактом вызова.
		*/func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		})
	mux.HandleFunc("/records/", /* Вложенный обработчик выполняет выделенный шаг обработки в сборке и запуске компонентов приложения, используя состояние окружающей функции.

		@args
		  - w (http.ResponseWriter): получатель HTTP-ответа.
		  - r (*http.Request): входящий HTTP-запрос.
		*/func(w http.ResponseWriter, r *http.Request) {
			if !operations.Authorized(r, ops.Config.InternalSecret) {
				http.NotFound(w, r)
				return
			}
			if !ops.Ready() {
				http.Error(w, "worker unavailable", http.StatusServiceUnavailable)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, ops.Config.HTTPBodyBytes)
			r = r.WithContext(operations.WithID(r.Context(), r.Header.Get("X-Request-ID")))
			handleWorkerRecords(w, r, service, logger)
		})
	server := &http.Server{Addr: fmt.Sprintf(":%d", port), Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 45 * time.Minute, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 8192}
	shutdownDone := make(chan struct{})
	go /* Вложенный обработчик выполняет выделенный шаг обработки в сборке и запуске компонентов приложения, используя состояние окружающей функции.

	 */func() {
		<-ctx.Done()
		defer close(shutdownDone)
		ops.Drain()
		shutdown, done := context.WithTimeout(context.Background(), ops.Config.ShutdownTimeout)
		defer done()
		if err := server.Shutdown(shutdown); err != nil {
			_ = server.Close()
		}
	}()
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("worker HTTP server: %w", err)
	}
	if ctx.Err() != nil {
		<-shutdownDone
	}
	return nil
}

// sampleRecorder наблюдает активные композитные записи и фиксирует отмену процесса.
// ctx задаёт срок наблюдения, service возвращает только агрегированное число.
func sampleRecorder(ctx context.Context, service *recorder.CompositeService) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		operations.State("recordings_active", float64(service.Active()))
		select {
		case <-ctx.Done():
			slog.Info("recorder draining", "event_type", "service.draining")
			return
		case <-ticker.C:
		}
	}
}

// handleWorkerRecords обрабатывает HTTP-операции воркера над записью.
//
// @args
//   - w (http.ResponseWriter): получатель HTTP-ответа.
//   - r (*http.Request): входящий HTTP-запрос.
//   - service (*recorder.WorkerService): значение service типа *recorder.WorkerService, используемое согласно назначению этой операции.
//   - logger (*log.Logger): значение logger типа *log.Logger, используемое согласно назначению этой операции.
func handleWorkerRecords(w http.ResponseWriter, r *http.Request, service *recorder.WorkerService, logger *log.Logger) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 3 || parts[0] != "records" {
		http.NotFound(w, r)
		return
	}
	recordID := parts[1]
	if err := service.ValidateLegacyRecord(r.Context(), recordID); err != nil {
		http.NotFound(w, r)
		return
	}
	switch {
	case len(parts) == 3 && parts[2] == "start":
		handleWorkerCommand(w, r, service, logger, records.Command{Type: "record.start", RecordID: recordID})
	case len(parts) == 3 && parts[2] == "stop":
		handleWorkerCommand(w, r, service, logger, records.Command{Type: "record.stop", RecordID: recordID})
	case len(parts) == 4 && parts[2] == "webrtc" && parts[3] == "offer":
		handleWorkerWebRTCOffer(w, r, service, recordID)
	default:
		http.NotFound(w, r)
	}
}

// handleWorkerCommand разбирает и исполняет внутреннюю HTTP-команду воркера записи.
//
// @args
//   - w (http.ResponseWriter): получатель HTTP-ответа.
//   - r (*http.Request): входящий HTTP-запрос.
//   - service (*recorder.WorkerService): значение service типа *recorder.WorkerService, используемое согласно назначению этой операции.
//   - logger (*log.Logger): значение logger типа *log.Logger, используемое согласно назначению этой операции.
//   - command (records.Command): внутренняя команда с типом операции и серверной идентичностью ресурса.
func handleWorkerCommand(w http.ResponseWriter, r *http.Request, service *recorder.WorkerService, logger *log.Logger, command records.Command) {
	var request records.Command
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	command.Reason = request.Reason
	command.SegmentDurationSec = request.SegmentDurationSec
	logger.Printf("received command %s for record %s", command.Type, command.RecordID)
	if err := service.HandleCommand(r.Context(), command); err != nil {
		logger.Printf("command %s for record %s failed: %v", command.Type, command.RecordID, err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

// handleWorkerWebRTCOffer принимает SDP-предложение на внутреннем HTTP-маршруте старого воркера записи.
//
// @args
//   - w (http.ResponseWriter): получатель HTTP-ответа.
//   - r (*http.Request): входящий HTTP-запрос.
//   - service (*recorder.WorkerService): значение service типа *recorder.WorkerService, используемое согласно назначению этой операции.
//   - recordID (string): внешний UUID задачи записи.
func handleWorkerWebRTCOffer(w http.ResponseWriter, r *http.Request, service *recorder.WorkerService, recordID string) {
	var request records.WebRTCOfferRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	response, err := service.HandleOffer(r.Context(), recordID, request)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}
