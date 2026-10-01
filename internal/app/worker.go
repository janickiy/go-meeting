package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/janickiy/go-recorder/internal/config"
	"github.com/janickiy/go-recorder/internal/domain/realtime"
	"github.com/janickiy/go-recorder/internal/domain/records"
	ffmpeginfra "github.com/janickiy/go-recorder/internal/infrastructure/ffmpeg"
	postgresinfra "github.com/janickiy/go-recorder/internal/infrastructure/postgres"
	rabbitmqinfra "github.com/janickiy/go-recorder/internal/infrastructure/rabbitmq"
	redisinfra "github.com/janickiy/go-recorder/internal/infrastructure/redis"
	s3storage "github.com/janickiy/go-recorder/internal/infrastructure/storage/s3"
	webrtcingest "github.com/janickiy/go-recorder/internal/infrastructure/webrtc"
	"github.com/janickiy/go-recorder/internal/usecase/recorder"
)

// RunWorker запускает recorder-worker.
// Параметры: нет.
// Возвращает: ошибку bootstrap-а.
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
	logger := log.New(os.Stdout, "recorder-worker: ", log.LstdFlags|log.Lmicroseconds)
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
	conferenceLock := redisinfra.NewConferenceLock(redisClient, cfg.RecordLockTTL)
	ingest, err := webrtcingest.NewManager(webrtcingest.Options{
		StoragePath: cfg.StoragePath,
		FFmpegPath:  cfg.FFmpegPath,
		UDPPort:     cfg.WebRTCUDPPort,
		TCPPort:     cfg.WebRTCTCPPort,
		NATIPs:      cfg.WebRTCNATIPs,
		Logger:      logger,
		OnStarted: func(ctx context.Context, recordID string) {
			if err := repository.MarkRecording(ctx, recordID, cfg.WorkerID); err != nil && !errors.Is(err, records.ErrRecordStateChanged) {
				logger.Printf("mark record %s recording failed: %v", recordID, err)
			}
		},
		OnFailed: func(ctx context.Context, recordID string, cause error) {
			if err := recorder.FailIngest(ctx, repository, conferenceLock, recordID, cfg.WorkerID, cause); err != nil {
				logger.Printf("handle ingest failure for record %s: %v", recordID, err)
			}
		},
	})
	if err != nil {
		return err
	}
	postProcessor := ffmpeginfra.NewPostProcessor(cfg.FFmpegPath)
	service := recorder.NewWorkerService(repository, postProcessor, ingest, s3Client, cfg.StoragePath, cfg.WorkerID, conferenceLock)
	store := redisinfra.NewRealtimeStore(redisClient, realtimeConfig.Namespace)
	composites := recorder.NewCompositeService(recorder.CompositeOptions{
		Repository: repository, Registry: redisinfra.NewMediaRegistry(redisClient, mediaConfig.Namespace), S3: s3Client,
		StoragePath: cfg.StoragePath, FFmpegPath: cfg.FFmpegPath, WorkerID: cfg.WorkerID, InternalSecret: mediaConfig.InternalSecret,
		Config: compositeConfig, ConferenceLock: conferenceLock, Logger: logger,
		Publish: func(ctx context.Context, record records.Record, kind string) error {
			status := record.Status
			if status == records.StatusFinalizing || status == records.StatusUploading {
				status = "processing"
			}
			event := realtime.Event(kind, record.ConferenceID, map[string]any{"recordingId": record.UUID, "conferenceId": record.ConferenceID, "status": status, "mode": "composite", "error": record.ErrorMessage})
			return store.Publish(ctx, realtime.Bus{Kind: "event", ConferenceID: record.ConferenceID, Event: &event})
		},
	})
	service.SetComposite(composites)
	composites.Start(ctx)
	defer func() { stop(); composites.Wait() }()
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

	errCh := make(chan error, 1)
	go func() {
		errCh <- consumer.Consume(ctx, service.HandleCommand)
	}()
	go serveWorkerHTTP(ctx, cfg.WorkerPort, service, logger)
	logger.Printf("worker is ready; http=:%d storage=%s minio=%s/%s rabbitmq_queue=%s", cfg.WorkerPort, cfg.StoragePath, cfg.MinIOEndpoint, cfg.MinIOBucket, cfg.RabbitMQQueue)

	select {
	case <-ctx.Done():
		return nil
	case err := <-errCh:
		if err != nil && ctx.Err() == nil {
			return err
		}

		return nil
	}
}

func serveWorkerHTTP(ctx context.Context, port int, service *recorder.WorkerService, logger *log.Logger) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("/records/", func(w http.ResponseWriter, r *http.Request) {
		handleWorkerRecords(w, r, service, logger)
	})
	server := &http.Server{Addr: fmt.Sprintf(":%d", port), Handler: mux}
	go func() {
		<-ctx.Done()
		_ = server.Close()
	}()
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Printf("worker health server failed: %v", err)
	}
}

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
