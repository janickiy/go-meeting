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
	"github.com/janickiy/go-recorder/internal/transport/mediaworker"
	"github.com/pion/webrtc/v4"
)

// RunMediaWorker has no PostgreSQL, RabbitMQ, MinIO or FFmpeg dependency.
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
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
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
		ICEDisconnectedTimeout: cfg.ICEDisconnectedTimeout, ICEFailedTimeout: cfg.ICEFailedTimeout, ICEKeepaliveInterval: cfg.ICEKeepaliveInterval, NegotiationTimeout: cfg.NegotiationTimeout, Logger: logger, Emit: emit})
	if err != nil {
		return err
	}
	cleanupDone := make(chan struct{})
	go func() {
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
	handler := mediaworker.NewHandler(cfg, registry, store, tickets, engine, func() any { return engine.Snapshot() }, logger)
	defer func() {
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
	server := &http.Server{Handler: handler.Routes(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: cfg.OperationTimeout + 5*time.Second, WriteTimeout: cfg.OperationTimeout + 5*time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 8192}
	failed := make(chan error, 1)
	go func() { failed <- server.Serve(listener) }()
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
	cancel()
	<-done
	shutdown, stop := context.WithTimeout(context.Background(), 10*time.Second)
	defer stop()
	return server.Shutdown(shutdown)
}
