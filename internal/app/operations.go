package app

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/janickiy/go-recorder/internal/infrastructure/sfu"
	"github.com/janickiy/go-recorder/internal/operations"
)

// hostname возвращает идентичность экземпляра для логов, не для metric labels.
// Аргументов нет; при отсутствии hostname используется фиксированное имя.
func hostname() string {
	value, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return value
}

// runProfiling запускает необязательную loopback-диагностику. ctx прекращает
// сервер, ops содержит проверенный порт. Ошибка не раскрывает данные профиля.
func runProfiling(ctx context.Context, ops *operations.Runtime) {
	if err := ops.Profiling(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("profiling unavailable", "event_type", "profiling.failed")
	}
}

// sampleDatabase обновляет размер пула и накопленные ожидания каждые 5 секунд.
// ctx задаёт время жизни процесса, db — общий SQL-пул; запросы к БД не выполняются.
func sampleDatabase(ctx context.Context, db *sql.DB) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		stats := db.Stats()
		operations.State("db_open", float64(stats.OpenConnections))
		operations.State("db_in_use", float64(stats.InUse))
		operations.State("db_wait_total", float64(stats.WaitCount))
		operations.State("db_wait_seconds", stats.WaitDuration.Seconds())
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// sampleMedia снимает агрегированные показатели SFU без идентификаторов комнат.
// ctx останавливает наблюдение, engine предоставляет согласованный Snapshot.
func sampleMedia(ctx context.Context, engine *sfu.Manager) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		s := engine.Snapshot()
		operations.State("media_relay_total", float64(s.RelayConnections))
		operations.State("media_direct_total", float64(s.DirectConnections))
		for k, v := range map[string]float64{"media_rooms": float64(s.Rooms), "media_peers": float64(s.Peers), "media_tracks": float64(s.Tracks), "media_subscriptions": float64(s.Subscriptions), "media_connections_total": float64(s.PeerConnections), "media_failures_total": float64(s.Failures), "rtp_packets_total": float64(s.Packets), "rtp_bytes_total": float64(s.Bytes), "rtp_dropped_total": float64(s.Dropped), "recording_outputs": float64(s.RecordingOutputs), "recording_drops_total": float64(s.RecordingDrops)} {
			operations.State(k, v)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
