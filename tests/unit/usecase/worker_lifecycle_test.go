package usecase_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/janickiy/go-recorder/internal/domain/records"
	"github.com/janickiy/go-recorder/internal/infrastructure/ffmpeg"
	"github.com/janickiy/go-recorder/internal/usecase/recorder"
)

const workerTestRecordID = "22222222-2222-4222-8222-222222222222"

func TestWorkerIgnoresCommandsForTerminalRecords(t *testing.T) {
	for _, status := range []string{records.StatusReady, records.StatusPartialReady, records.StatusFailed, records.StatusCancelled} {
		for _, commandType := range []string{"record.start", "record.stop"} {
			t.Run(status+"/"+commandType, func(t *testing.T) {
				repo := newWorkerRepository(status)
				ingest := &fakeIngest{}
				storage := t.TempDir()
				locker := &failureLock{}
				service := recorder.NewWorkerService(repo, nil, ingest, nil, storage, "worker", locker)
				if err := service.HandleCommand(context.Background(), records.Command{Type: commandType, RecordID: workerTestRecordID}); err != nil {
					t.Fatal(err)
				}
				if ingest.prepared || ingest.stopped || repo.marked || repo.finalizing || repo.eventType != "" {
					t.Fatal("terminal record was processed again")
				}
				entries, err := os.ReadDir(storage)
				if err != nil || len(entries) != 0 {
					t.Fatalf("duplicate command touched storage: %v", err)
				}
				if locker.recordID != workerTestRecordID {
					t.Fatal("retry did not release residual owner lock")
				}
			})
		}
	}
}

func TestWorkerReleasesLockAfterMediaStopsBeforeFinalization(t *testing.T) {
	repo := newWorkerRepository(records.StatusStopping)
	ingest := &fakeIngest{}
	lock := &orderedRelease{t: t, ingest: ingest}
	repo.onFinalizing = func() {
		if !lock.released {
			t.Error("finalization started before the closed media lock was released")
		}
	}
	// An empty directory makes Finalize fail without launching FFmpeg.
	service := recorder.NewWorkerService(repo, ffmpeg.NewPostProcessor("not-used"), ingest, nil, t.TempDir(), "worker", lock)
	if err := service.HandleCommand(context.Background(), records.Command{Type: "record.stop", RecordID: workerTestRecordID}); err != nil {
		t.Fatal(err)
	}
	if !lock.released || !repo.marked || repo.eventType != "record.finalize.failed" {
		t.Fatal("failed finalization did not persist failure and release lock")
	}
}

func TestWorkerPrepareFailureReleasesLock(t *testing.T) {
	repo := newWorkerRepository(records.StatusStarting)
	ingest := &fakeIngest{prepareErr: errors.New("prepare failed")}
	lock := &failureLock{}
	service := recorder.NewWorkerService(repo, nil, ingest, nil, t.TempDir(), "worker", lock)
	if err := service.HandleCommand(context.Background(), records.Command{Type: "record.start", RecordID: workerTestRecordID}); err != nil {
		t.Fatal(err)
	}
	if !repo.marked || lock.recordID != workerTestRecordID || repo.eventType != "record.worker.prepare.failed" {
		t.Fatal("prepare failure left an active lock or missed failure event")
	}
}

func TestWorkerDoesNotAcknowledgeDatabaseFailure(t *testing.T) {
	wantErr := errors.New("database unavailable")
	repo := newWorkerRepository(records.StatusStarting)
	repo.markErr = wantErr
	lock := &failureLock{}
	service := recorder.NewWorkerService(repo, nil, &fakeIngest{prepareErr: errors.New("prepare failed")}, nil, t.TempDir(), "worker", lock)
	if err := service.HandleCommand(context.Background(), records.Command{Type: "record.start", RecordID: workerTestRecordID}); !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want database failure", err)
	}
	if lock.recordID != "" {
		t.Fatal("failure persistence did not succeed but lock was released")
	}
}

func TestWorkerRejectsUnsafeIDBeforeTouchingStorage(t *testing.T) {
	storage := t.TempDir()
	service := recorder.NewWorkerService(nil, nil, nil, nil, storage, "worker", nil)
	for _, recordID := range []string{"", "../escape", filepath.Join(storage, "escape"), "not-a-uuid"} {
		if err := service.HandleCommand(context.Background(), records.Command{Type: "record.start", RecordID: recordID}); err == nil {
			t.Fatalf("accepted unsafe ID %q", recordID)
		}
	}
	entries, err := os.ReadDir(storage)
	if err != nil || len(entries) != 0 {
		t.Fatal("invalid command created files")
	}
}

type fakeWorkerRepository struct {
	failureRepository
	record       records.Record
	finalizing   bool
	onFinalizing func()
}

func TestConcurrentDuplicateStopsRunMediaStopOnce(t *testing.T) {
	repo := newWorkerRepository(records.StatusStopping)
	ingest := &fakeIngest{}
	service := recorder.NewWorkerService(repo, ffmpeg.NewPostProcessor("not-used"), ingest, nil, t.TempDir(), "worker", nil)
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := service.HandleCommand(context.Background(), records.Command{Type: "record.stop", RecordID: workerTestRecordID}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if ingest.stopCalls != 1 || repo.record.Status != records.StatusFailed {
		t.Fatalf("stop calls = %d, status = %s", ingest.stopCalls, repo.record.Status)
	}
}

func newWorkerRepository(status string) *fakeWorkerRepository {
	return &fakeWorkerRepository{record: records.Record{UUID: workerTestRecordID, ConferenceID: "conference-1", Status: status}}
}

func (r *fakeWorkerRepository) FindByUUID(context.Context, string) (records.Record, error) {
	return r.record, nil
}

func (r *fakeWorkerRepository) MarkFailed(ctx context.Context, id string, cause error) error {
	if err := r.failureRepository.MarkFailed(ctx, id, cause); err != nil {
		return err
	}
	r.record.Status = records.StatusFailed
	return nil
}

func (r *fakeWorkerRepository) MarkFinalizing(context.Context, string) error {
	r.finalizing = true
	if r.onFinalizing != nil {
		r.onFinalizing()
	}
	return nil
}

func (r *fakeWorkerRepository) MarkUploading(context.Context, string) error { return nil }
func (r *fakeWorkerRepository) SaveFinalArtifacts(context.Context, string, records.RecordFile, *records.RecordFile, []records.RecordSegment) error {
	return nil
}

type fakeIngest struct {
	prepared, stopped bool
	prepareErr        error
	stopCalls         int
}

func (i *fakeIngest) Prepare(string, int) error { i.prepared = true; return i.prepareErr }
func (i *fakeIngest) Stop(string) error         { i.stopped = true; i.stopCalls++; return nil }
func (i *fakeIngest) HandleOffer(context.Context, string, records.WebRTCOfferRequest) (records.WebRTCAnswerResponse, error) {
	return records.WebRTCAnswerResponse{}, nil
}

type orderedRelease struct {
	t        *testing.T
	ingest   *fakeIngest
	released bool
}

func (l *orderedRelease) Release(context.Context, string, string) error {
	if !l.ingest.stopped {
		l.t.Error("lock released while media is still running")
	}
	l.released = true
	return nil
}
