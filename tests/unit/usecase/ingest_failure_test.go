package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/janickiy/go-recorder/internal/domain/records"
	"github.com/janickiy/go-recorder/internal/usecase/recorder"
)

func TestFailIngestReleasesOnlyFailedRecordLock(t *testing.T) {
	repo := &failureRepository{}
	lock := &failureLock{}
	err := recorder.FailIngest(context.Background(), repo, lock, "record-1", "worker-1", errors.New("no tracks"))
	if err != nil {
		t.Fatal(err)
	}
	if !repo.marked || lock.recordID != "record-1" || lock.conferenceID != "conference-1" {
		t.Fatalf("failed record lock was not released: %+v", lock)
	}
	if repo.eventType != "record.ingest.failed" || repo.message != "no tracks" {
		t.Fatal("failure event is missing")
	}
}

func TestFailIngestKeepsLockWhenDatabaseUpdateFails(t *testing.T) {
	repo := &failureRepository{markErr: errors.New("database unavailable")}
	lock := &failureLock{}
	if err := recorder.FailIngest(context.Background(), repo, lock, "record-1", "worker-1", errors.New("no tracks")); err == nil {
		t.Fatal("expected database error")
	}
	if lock.recordID != "" {
		t.Fatal("lock released before failure was persisted")
	}
}

func TestFailIngestReleasesLockEvenWhenEventFails(t *testing.T) {
	wantErr := errors.New("event unavailable")
	repo := &failureRepository{eventErr: wantErr}
	lock := &failureLock{}
	err := recorder.FailIngest(context.Background(), repo, lock, "record-1", "worker-1", errors.New("no tracks"))
	if !errors.Is(err, wantErr) || lock.recordID != "record-1" {
		t.Fatalf("err = %v, released record = %s", err, lock.recordID)
	}
}

type failureRepository struct {
	markErr, eventErr  error
	status             string
	marked             bool
	eventType, message string
}

func (r *failureRepository) FindByUUID(context.Context, string) (records.Record, error) {
	return records.Record{UUID: "record-1", ConferenceID: "conference-1", Status: r.status}, nil
}

func TestLateIngestFailureDoesNotOverwriteReadyRecord(t *testing.T) {
	repo := &failureRepository{status: records.StatusReady}
	lock := &failureLock{}
	if err := recorder.FailIngest(context.Background(), repo, lock, "record-1", "worker-1", errors.New("late ICE error")); err != nil {
		t.Fatal(err)
	}
	if repo.marked || repo.eventType != "" {
		t.Fatal("late ingest failure changed a ready record")
	}
}

func (r *failureRepository) MarkFailed(context.Context, string, error) error {
	r.marked = r.markErr == nil
	return r.markErr
}

func (r *failureRepository) AddEvent(_ context.Context, _, eventType, _, _, message, _ string) error {
	r.eventType, r.message = eventType, message
	return r.eventErr
}

type failureLock struct{ recordID, conferenceID string }

func (l *failureLock) Release(_ context.Context, conferenceID, recordID string) error {
	l.conferenceID, l.recordID = conferenceID, recordID
	return nil
}
