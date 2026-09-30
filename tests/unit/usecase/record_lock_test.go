package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"git.svc-dev.net/board/go-recorder/internal/domain/records"
	"git.svc-dev.net/board/go-recorder/internal/usecase/recorder"
)

func TestStartReturnsConflictWhenConferenceLockExists(t *testing.T) {
	repository := &fakeRepository{}
	worker := &fakeWorkerCommander{}
	locker := &fakeConferenceLocker{acquireOK: false}
	service := recorder.NewService(repository, worker, nil, locker)

	_, err := service.Start(context.Background(), records.StartRequest{
		ConferenceID:       "11111111-1111-4111-8111-111111111111",
		QualityMode:        "auto",
		SegmentDurationSec: 5,
	})

	if !errors.Is(err, records.ErrConferenceAlreadyRecording) {
		t.Fatalf("err = %v, want ErrConferenceAlreadyRecording", err)
	}
	if repository.createCalled {
		t.Fatal("record was created when lock was not acquired")
	}
	if worker.startCalled {
		t.Fatal("worker start was called when lock was not acquired")
	}
}

func TestStartKeepsConferenceLockAfterWorkerPrepare(t *testing.T) {
	repository := &fakeRepository{}
	worker := &fakeWorkerCommander{}
	locker := &fakeConferenceLocker{acquireOK: true}
	service := recorder.NewService(repository, worker, nil, locker)

	response, err := service.Start(context.Background(), records.StartRequest{
		ConferenceID:       "11111111-1111-4111-8111-111111111111",
		QualityMode:        "auto",
		SegmentDurationSec: 5,
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if response.RecordID == "" {
		t.Fatal("RecordID is empty")
	}
	if !locker.acquireCalled {
		t.Fatal("lock was not acquired")
	}
	if locker.releaseCalled {
		t.Fatal("lock was released after successful start")
	}
	if worker.startRecordID != response.RecordID {
		t.Fatalf("worker start recordId = %q, want %q", worker.startRecordID, response.RecordID)
	}
	if response.WebRTC.Video.Quality != records.DefaultVideoQuality {
		t.Fatalf("response video quality = %q, want %q", response.WebRTC.Video.Quality, records.DefaultVideoQuality)
	}
	if response.WebRTC.Video.Width != records.DefaultVideoWidth || response.WebRTC.Video.Height != records.DefaultVideoHeight {
		t.Fatalf("response video size = %dx%d, want %dx%d", response.WebRTC.Video.Width, response.WebRTC.Video.Height, records.DefaultVideoWidth, records.DefaultVideoHeight)
	}
	if response.WebRTC.Video.MaxBitrateBPS != records.DefaultVideoMaxBitrateBPS {
		t.Fatalf("response max bitrate = %d, want %d", response.WebRTC.Video.MaxBitrateBPS, records.DefaultVideoMaxBitrateBPS)
	}
}

func TestStartReleasesConferenceLockWhenWorkerPrepareFails(t *testing.T) {
	repository := &fakeRepository{}
	worker := &fakeWorkerCommander{startErr: errors.New("worker down")}
	locker := &fakeConferenceLocker{acquireOK: true}
	service := recorder.NewService(repository, worker, nil, locker)

	_, err := service.Start(context.Background(), records.StartRequest{
		ConferenceID:       "11111111-1111-4111-8111-111111111111",
		QualityMode:        "auto",
		SegmentDurationSec: 5,
	})

	if err == nil {
		t.Fatal("Start() error is nil")
	}
	if !locker.releaseCalled {
		t.Fatal("lock was not released after worker prepare error")
	}
	if !repository.markFailedCalled {
		t.Fatal("record was not marked failed after worker prepare error")
	}
}

func TestStopKeepsConferenceLockUntilWorkerStopsMedia(t *testing.T) {
	repository := &fakeRepository{
		record: records.Record{
			UUID:         "22222222-2222-4222-8222-222222222222",
			ConferenceID: "11111111-1111-4111-8111-111111111111",
			Status:       records.StatusRecording,
		},
	}
	worker := &fakeWorkerCommander{}
	locker := &fakeConferenceLocker{acquireOK: true}
	service := recorder.NewService(repository, worker, nil, locker)

	err := service.Stop(context.Background(), records.EndRequest{
		RecordID: "22222222-2222-4222-8222-222222222222",
		Reason:   "client_stop",
	})
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	if !repository.markStoppingCalled {
		t.Fatal("record was not marked stopping")
	}
	if !worker.stopCalled {
		t.Fatal("worker stop was not called")
	}
	if locker.releaseCalled {
		t.Fatal("lock was released before the worker stopped media")
	}
}

func TestStopDoesNotReopenCompletedOrFinalizingRecord(t *testing.T) {
	for _, status := range []string{records.StatusReady, records.StatusPartialReady, records.StatusFailed, records.StatusCancelled, records.StatusFinalizing, records.StatusUploading} {
		t.Run(status, func(t *testing.T) {
			repository := &fakeRepository{record: records.Record{UUID: "record-1", Status: status}}
			worker := &fakeWorkerCommander{}
			service := recorder.NewService(repository, worker, nil, nil)
			if err := service.Stop(context.Background(), records.EndRequest{RecordID: "record-1", Reason: "retry"}); err != nil {
				t.Fatal(err)
			}
			if repository.markStoppingCalled || worker.stopCalled {
				t.Fatal("stop reopened a completed or finalizing record")
			}
		})
	}
}

func TestStopRetriesPublishWithoutChangingStopMetadata(t *testing.T) {
	repository := &fakeRepository{record: records.Record{UUID: "record-1", Status: records.StatusRecording}}
	wantErr := errors.New("broker unavailable")
	worker := &fakeWorkerCommander{stopErr: wantErr}
	locker := &fakeConferenceLocker{}
	service := recorder.NewService(repository, worker, nil, locker)
	request := records.EndRequest{RecordID: "record-1", Reason: "client_stop"}
	if err := service.Stop(context.Background(), request); !errors.Is(err, wantErr) {
		t.Fatalf("Stop() = %v, want publish error", err)
	}
	if locker.releaseCalled {
		t.Fatal("failed publication released lock")
	}
	repository.markStoppingCalled = false
	worker.stopCalled, worker.stopErr = false, nil
	if err := service.Stop(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if repository.markStoppingCalled || !worker.stopCalled || locker.releaseCalled {
		t.Fatal("retry must only republish the stop command")
	}
}

func TestStopIgnoresStaleState(t *testing.T) {
	repository := &fakeRepository{record: records.Record{UUID: "record-1", Status: records.StatusRecording}, markStoppingErr: records.ErrRecordStateChanged}
	worker := &fakeWorkerCommander{}
	service := recorder.NewService(repository, worker, nil, nil)
	if err := service.Stop(context.Background(), records.EndRequest{RecordID: "record-1"}); err != nil {
		t.Fatal(err)
	}
	if worker.stopCalled {
		t.Fatal("stale request published a stop command")
	}
}

func TestCountByConferenceReturnsRecordsWithTimeFields(t *testing.T) {
	duration := 37
	startedAt := time.Date(2026, 5, 29, 9, 30, 0, 0, time.UTC)
	createdAt := time.Date(2026, 5, 29, 9, 29, 0, 0, time.UTC)
	repository := &fakeRepository{
		listDetailsByConferenceResponse: []records.RecordDetails{
			{
				Record: records.Record{
					UUID:         "22222222-2222-4222-8222-222222222222",
					ConferenceID: "11111111-1111-4111-8111-111111111111",
					Status:       records.StatusReady,
					DurationSec:  &duration,
					StartedAt:    &startedAt,
					CreatedAt:    createdAt,
				},
				Files: []records.RecordFile{
					{FileType: records.FileTypeFinalMP4, ObjectKey: "records/222/final.mp4"},
					{FileType: records.FileTypePreviewJPG, ObjectKey: "records/222/preview.jpg"},
				},
			},
		},
	}
	service := recorder.NewService(repository, &fakeWorkerCommander{}, nil, nil)

	result, err := service.CountByConference(context.Background(), []string{"11111111-1111-4111-8111-111111111111"}, records.StatusReady)
	if err != nil {
		t.Fatalf("CountByConference() error = %v", err)
	}

	if !repository.listDetailsByConferenceCalled {
		t.Fatal("repository ListSummaryDetailsByConferenceIDs() was not called")
	}
	if repository.listDetailsByConferenceStatus != records.StatusReady {
		t.Fatalf("status = %q, want %q", repository.listDetailsByConferenceStatus, records.StatusReady)
	}
	if got, want := len(result), 1; got != want {
		t.Fatalf("result len = %d, want %d", got, want)
	}
	if result[0].RecordsCount != 1 {
		t.Fatalf("recordsCount = %d, want 1", result[0].RecordsCount)
	}
	if got, want := len(result[0].Records), 1; got != want {
		t.Fatalf("records len = %d, want %d", got, want)
	}
	if result[0].Records[0].RecordID != "22222222-2222-4222-8222-222222222222" {
		t.Fatalf("recordId = %q", result[0].Records[0].RecordID)
	}
	if result[0].Records[0].DurationSec == nil || *result[0].Records[0].DurationSec != duration {
		t.Fatalf("durationSec = %v, want %d", result[0].Records[0].DurationSec, duration)
	}
}

type fakeRepository struct {
	createCalled                    bool
	markFailedCalled                bool
	markStoppingCalled              bool
	markStoppingErr                 error
	listDetailsByConferenceCalled   bool
	listDetailsByConferenceIDs      []string
	listDetailsByConferenceStatus   string
	listDetailsByConferenceResponse []records.RecordDetails
	listDetailsByConferenceErr      error
	record                          records.Record
}

func (r *fakeRepository) Create(_ context.Context, record records.Record) (records.Record, error) {
	r.createCalled = true
	if record.UUID == "" {
		record.UUID = "22222222-2222-4222-8222-222222222222"
	}
	r.record = record

	return record, nil
}

func (r *fakeRepository) FindByUUID(_ context.Context, _ string) (records.Record, error) {
	if r.record.UUID == "" {
		return records.Record{}, errors.New("not found")
	}

	return r.record, nil
}

func (r *fakeRepository) MarkStopping(_ context.Context, _ string, _ string) error {
	r.markStoppingCalled = true
	if r.markStoppingErr == nil {
		r.record.Status = records.StatusStopping
	}
	return r.markStoppingErr
}

func (r *fakeRepository) MarkFailed(_ context.Context, _ string, _ error) error {
	r.markFailedCalled = true

	return nil
}

func (r *fakeRepository) ListDetails(_ context.Context, _ int, _ int) ([]records.RecordDetails, error) {
	return nil, nil
}

func (r *fakeRepository) ListSummaryDetailsByConferenceIDs(_ context.Context, conferenceIDs []string, status string) ([]records.RecordDetails, error) {
	r.listDetailsByConferenceCalled = true
	r.listDetailsByConferenceIDs = conferenceIDs
	r.listDetailsByConferenceStatus = status

	return r.listDetailsByConferenceResponse, r.listDetailsByConferenceErr
}

func (r *fakeRepository) FindDetailsByUUID(_ context.Context, _ string) (records.RecordDetails, error) {
	return records.RecordDetails{}, nil
}

type fakeWorkerCommander struct {
	startCalled      bool
	startRecordID    string
	startDurationSec int
	startErr         error
	stopCalled       bool
	stopRecordID     string
	stopReason       string
	stopErr          error
}

func (c *fakeWorkerCommander) StartRecord(_ context.Context, recordID string, segmentDurationSec int) error {
	c.startCalled = true
	c.startRecordID = recordID
	c.startDurationSec = segmentDurationSec

	return c.startErr
}

func (c *fakeWorkerCommander) StopRecord(_ context.Context, recordID string, reason string) error {
	c.stopCalled = true
	c.stopRecordID = recordID
	c.stopReason = reason

	return c.stopErr
}

type fakeConferenceLocker struct {
	acquireOK     bool
	acquireCalled bool
	releaseCalled bool
}

func (l *fakeConferenceLocker) Acquire(_ context.Context, _ string, _ string) (bool, error) {
	l.acquireCalled = true

	return l.acquireOK, nil
}

func (l *fakeConferenceLocker) Release(_ context.Context, _ string, _ string) error {
	l.releaseCalled = true

	return nil
}
