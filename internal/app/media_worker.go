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

	"github.com/janickiy/go-recorder/internal/config"
	"github.com/janickiy/go-recorder/internal/domain/media"
	"github.com/janickiy/go-recorder/internal/domain/realtime"
	redisinfra "github.com/janickiy/go-recorder/internal/infrastructure/redis"
	"github.com/janickiy/go-recorder/internal/infrastructure/security"
	"github.com/janickiy/go-recorder/internal/infrastructure/sfu"
	"github.com/janickiy/go-recorder/internal/operations"
	"github.com/janickiy/go-recorder/internal/transport/mediaworker"
	"github.com/pion/webrtc/v4"
)

// RunMediaWorker собирает SFU, реестр владения и внутренний сервер медиа-воркера.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func RunMediaWorker() error {
	base, err := config.Load()
	if err != nil {
		return err
	}
	cfg, err := config.LoadMedia()
	if err != nil {
		return err
	}
	rt, err := config.LoadRealtime()
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", "media-worker", "instance_id", cfg.WorkerID)
	slog.SetDefault(logger)
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	connect, stopConnect := context.WithTimeout(ctx, cfg.OperationTimeout)
	client, err := redisinfra.NewClient(connect, base.RedisAddr, base.RedisPassword, base.RedisDB)
	stopConnect()
	if err != nil {
		return err
	}
	defer client.Close()
	store := redisinfra.NewRealtimeStore(client, rt.Namespace)
	registry := redisinfra.NewMediaRegistry(client, cfg.Namespace)
	tickets, err := security.NewMediaTickets(cfg.TicketSecret, cfg.TicketTTL)
	if err != nil {
		return err
	}
	ice := make([]webrtc.ICEServer, 0, len(cfg.ICE.ICEServers))
	for _, server := range cfg.ICE.ICEServers {
		ice = append(ice, webrtc.ICEServer{URLs: server.URLs, Username: server.Username, Credential: server.Credential})
	}
	var engine *sfu.Manager
	failedPeers := make(chan string, 128)
	// Вложенный обработчик выполняет выделенный шаг обработки в сборке и запуске компонентов приложения, используя состояние окружающей функции.
	//
	// @args
	//   - binding (media.Binding): проверенная идентичность медиа-подключения, назначенная сервером.
	//   - kind (string): тип события, ошибки или медиа, определяющий ветку обработки.
	//   - data (any): полезная нагрузка события или байты обрабатываемого содержимого.
	emit := func(binding media.Binding, kind string, data any) {
		op, stop := context.WithTimeout(ctx, cfg.OperationTimeout)
		event := realtime.Event(kind, binding.ConferenceID, data)
		err := store.Publish(op, realtime.Bus{Kind: "event", ConferenceID: binding.ConferenceID, ConnectionID: binding.ConnectionID, Event: &event})
		stop()
		if err != nil {
			logger.Warn("media signaling delivery failed", "conference_id", binding.ConferenceID, "participant_id", binding.ParticipantID, "session_id", binding.SessionID, "event_type", kind)
			// One bounded cleanup worker avoids waiting on this same peer event worker.
			select {
			case failedPeers <- binding.ConnectionID:
			default:
				cancel()
			}
		}
	}
	engine, err = sfu.NewManager(sfu.Options{WorkerID: cfg.WorkerID, ICE: ice, UDPPort: cfg.UDPPort, UDPMinPort: cfg.UDPMinPort, UDPMaxPort: cfg.UDPMaxPort, TCPPort: cfg.TCPPort, NATIPs: cfg.NATIPs,
		MaxPeers: cfg.MaxPeers, MaxRooms: cfg.MaxRooms, MaxPublishedTracks: cfg.MaxPublishedTracks, MaxAudioTracks: cfg.MaxAudioTracks, MaxVideoTracks: cfg.MaxVideoTracks, QueueSize: 128,
		MaxScreenSharers: cfg.MaxScreenSharers, EgressQueueSize: cfg.EgressQueueSize,
		ICEDisconnectedTimeout: cfg.ICEDisconnectedTimeout, ICEFailedTimeout: cfg.ICEFailedTimeout, ICEKeepaliveInterval: cfg.ICEKeepaliveInterval, NegotiationTimeout: cfg.NegotiationTimeout, Logger: logger, Emit: emit})
	if err != nil {
		return err
	}
	cleanupDone := make(chan struct{})
	go /* Вложенный обработчик выполняет выделенный шаг обработки в сборке и запуске компонентов приложения, используя состояние окружающей функции.

	 */func() {
		defer close(cleanupDone)
		for {
			select {
			case <-ctx.Done():
				return
			case id := <-failedPeers:
				cleanup, stop := context.WithTimeout(context.Background(), cfg.OperationTimeout)
				engine.LeaveConnection(cleanup, id)
				stop()
			}
		}
	}()
	handler := mediaworker.NewHandler(cfg, registry, store, tickets, engine, /* Вложенный обработчик выполняет выделенный шаг обработки в сборке и запуске компонентов приложения, используя состояние окружающей функции.


		@return:
		  - результат 1 (any): значение, подготовленное операцией для вызывающей стороны. */func() any { return engine.Snapshot() }, logger)
	defer /* Вложенный обработчик выполняет выделенный шаг обработки в сборке и запуске компонентов приложения, используя состояние окружающей функции.

	 */func() {
		cancel()
		<-cleanupDone
		shutdown, done := context.WithTimeout(context.Background(), 10*time.Second)
		defer done()
		_ = handler.Stop(shutdown)
	}()
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.HTTPPort))
	if err != nil {
		return err
	}
	done, err := handler.Start(ctx)
	if err != nil {
		_ = listener.Close()
		return err
	}
	ops := operations.New("media-worker", cfg.WorkerID, base.Operations, map[string]operations.Check{"redis": func(ctx context.Context) error { return client.Ping(ctx).Err() }, "ownership": func(ctx context.Context) error {
		if !handler.Ready() {
			return media.ErrUnavailable
		}
		return nil
	}})
	ops.Run(ctx)
	go runProfiling(ctx, ops)
	go sampleMedia(ctx, engine)
	mux := http.NewServeMux()
	ops.Register(mux)
	mux.Handle("/", handler.Routes())
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: cfg.OperationTimeout + 5*time.Second, WriteTimeout: cfg.OperationTimeout + 5*time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 8192}
	failed := make(chan error, 1)
	go /* Вложенный обработчик выполняет выделенный шаг обработки в сборке и запуске компонентов приложения, используя состояние окружающей функции.

	 */func() { failed <- server.Serve(listener) }()
	logger.Info("media worker ready", "worker_id", cfg.WorkerID, "internal_port", cfg.HTTPPort, "max_peers", cfg.MaxPeers, "udp_mux_port", cfg.UDPPort, "tcp_mux_port", cfg.TCPPort)
	select {
	case <-ctx.Done():
	case err := <-failed:
		if !errors.Is(err, http.ErrServerClosed) {
			cancel()
			<-done
			return err
		}
	}
	ops.Drain()
	cancel()
	<-done
	shutdown, stop := context.WithTimeout(context.Background(), base.Operations.ShutdownTimeout)
	defer stop()
	// Long-lived recording egress must close before HTTP waits for active
	// requests; otherwise shutdown always consumes its entire deadline.
	mediaErr := handler.Stop(shutdown)
	return errors.Join(mediaErr, server.Shutdown(shutdown))
}
