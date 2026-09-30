package recorder

import (
	"context"
	"errors"
	"fmt"

	"github.com/janickiy/go-recorder/internal/domain/records"
)

type ingestFailureRepository interface {
	FindByUUID(context.Context, string) (records.Record, error)
	MarkFailed(context.Context, string, error) error
	AddEvent(context.Context, string, string, string, string, string, string) error
}

type conferenceReleaser interface {
	Release(context.Context, string, string) error
}

// FailIngest persists a closed WebRTC session's failure and releases only its lock.
// The ingest manager must close the session before invoking this handler.
func FailIngest(ctx context.Context, repository ingestFailureRepository, locker conferenceReleaser, recordID, workerID string, cause error) error {
	return failRecord(ctx, repository, locker, recordID, workerID, "record.ingest.failed", cause)
}

// Call only after the corresponding media session has closed (or failed to prepare).
func failRecord(ctx context.Context, repository ingestFailureRepository, locker conferenceReleaser, recordID, workerID, eventType string, cause error) error {
	record, err := repository.FindByUUID(ctx, recordID)
	if err != nil {
		return fmt.Errorf("find failed record: %w", err)
	}
	if records.IsTerminalStatus(record.Status) {
		return releaseRecordLock(ctx, locker, record)
	}
	if err := repository.MarkFailed(ctx, recordID, cause); err != nil {
		return fmt.Errorf("mark ingest failed: %w", err)
	}
	releaseErr := releaseRecordLock(ctx, locker, record)
	message := "WebRTC ingest failed"
	if cause != nil {
		message = cause.Error()
	}
	eventErr := repository.AddEvent(ctx, recordID, eventType, "worker", "error", message, workerID)
	return errors.Join(releaseErr, eventErr)
}

func releaseRecordLock(ctx context.Context, locker conferenceReleaser, record records.Record) error {
	if locker == nil {
		return nil
	}
	return locker.Release(ctx, record.ConferenceID, record.UUID)
}
