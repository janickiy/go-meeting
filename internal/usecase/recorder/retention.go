package recorder

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/janickiy/go-recorder/internal/domain/records"
)

type RetentionRepository interface {
	ExpireNextRecording(context.Context, []int64, func(context.Context, records.Record) error) (int64, error)
}

type RecordingRemover interface {
	RemoveRecording(context.Context, records.Record) error
}

// RunRetention deletes expired artifacts at startup and every minute. A failed
// deletion remains eligible for retry; completed metadata is retained for audit.
func RunRetention(ctx context.Context, repo RetentionRepository, storage RecordingRemover) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		count, err := SweepRetention(ctx, repo, storage)
		if err != nil && ctx.Err() == nil {
			slog.Error("recording retention failed", "event_type", "recording.retention_failed", "error", err)
		} else if count > 0 {
			slog.Info("expired recordings removed", "event_type", "recording.retention_completed", "count", count)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// SweepRetention bounds each storage operation and each pass. Row locks prevent
// two API replicas from deleting the same record concurrently.
func SweepRetention(ctx context.Context, repo RetentionRepository, storage RecordingRemover) (int, error) {
	count := 0
	var attempted []int64
	var failures error
	for len(attempted) < 100 && ctx.Err() == nil {
		operation, cancel := context.WithTimeout(ctx, 2*time.Minute)
		id, err := repo.ExpireNextRecording(operation, attempted, storage.RemoveRecording)
		cancel()
		if id == 0 {
			return count, errors.Join(failures, err)
		}
		attempted = append(attempted, id)
		if err != nil {
			failures = errors.Join(failures, err)
			continue
		}
		count++
	}
	return count, errors.Join(failures, ctx.Err())
}
