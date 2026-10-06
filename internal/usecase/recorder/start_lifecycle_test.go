package recorder

import (
	"context"
	"errors"
	"io"
	"log"
	"testing"
	"time"

	"github.com/janickiy/go-recorder/internal/domain/records"
	webrtcingest "github.com/janickiy/go-recorder/internal/infrastructure/webrtc"
)

type failingReadyEventRepository struct {
	workerRepository
	ingest        *webrtcingest.Manager
	preparedCalls int
}

func (r *failingReadyEventRepository) FindByUUID(_ context.Context, id string) (records.Record, error) {
	return records.Record{UUID: id, Status: records.StatusStarting}, nil
}

func (r *failingReadyEventRepository) AddEvent(context.Context, string, string, string, string, string, string) error {
	if r.ingest.Active() == 1 {
		r.preparedCalls++
	}
	return errors.New("injected ready-event database failure")
}

func TestWorkerReadyEventFailureReleasesPreparedSession(t *testing.T) {
	root := t.TempDir()
	ingest, err := webrtcingest.NewManager(webrtcingest.Options{StoragePath: root, Logger: log.New(io.Discard, "", 0), MaxSessions: 2})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = ingest.Shutdown(ctx)
	})
	repository := &failingReadyEventRepository{ingest: ingest}
	worker := &WorkerService{repository: repository, ingest: ingest, storagePath: root, workerID: "p0-start-failure"}
	for attempt := 0; attempt < 2; attempt++ {
		err := worker.handleStart(context.Background(), records.Command{RecordID: "11111111-1111-4111-8111-111111111111", SegmentDurationSec: 1})
		if err == nil {
			t.Fatal("database failure was hidden")
		}
		t.Logf("attempt=%d active_recording_slots=%d prepared_event_calls=%d", attempt+1, ingest.Active(), repository.preparedCalls)
		if ingest.Active() != 0 {
			t.Fatal("failed preparation retained a recording slot")
		}
	}
	if repository.preparedCalls != 2 {
		t.Fatal("retry did not prepare an owned media session")
	}
}
