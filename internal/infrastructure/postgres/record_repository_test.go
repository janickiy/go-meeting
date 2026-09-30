package postgres

import (
	"context"
	"errors"
	"net/url"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"git.svc-dev.net/board/go-recorder/internal/domain/records"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// These opt-in tests use a local, migrated database and roll back all test rows.
func testRepository(t *testing.T) (*RecordRepository, *gorm.DB) {
	t.Helper()
	dsn := os.Getenv("RECORDER_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set RECORDER_TEST_POSTGRES_DSN to a local PostgreSQL URL")
	}
	u, err := url.Parse(dsn)
	if err != nil || (u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1") {
		t.Fatal("repository tests require a local PostgreSQL URL")
	}
	db, err := Connect(dsn)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	t.Cleanup(func() { _ = tx.Rollback().Error })
	return NewRecordRepository(tx), tx
}

func createTestRecord(t *testing.T, repo *RecordRepository, status string) records.Record {
	t.Helper()
	record, err := repo.Create(context.Background(), records.Record{
		UUID: uuid.NewString(), ConferenceID: uuid.NewString(), Status: status,
		SourceType: "browser", TransportType: "webrtc", QualityMode: "auto", SegmentDurationSec: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func TestTerminalRecordCannotBeOverwritten(t *testing.T) {
	repo, _ := testRepository(t)
	ctx := context.Background()
	for _, status := range []string{records.StatusReady, records.StatusPartialReady, records.StatusFailed, records.StatusCancelled} {
		t.Run(status, func(t *testing.T) {
			record := createTestRecord(t, repo, status)
			for name, transition := range map[string]func() error{
				"recording":  func() error { return repo.MarkRecording(ctx, record.UUID, "late-worker") },
				"stopping":   func() error { return repo.MarkStopping(ctx, record.UUID, "duplicate") },
				"finalizing": func() error { return repo.MarkFinalizing(ctx, record.UUID) },
				"uploading":  func() error { return repo.MarkUploading(ctx, record.UUID) },
				"failed":     func() error { return repo.MarkFailed(ctx, record.UUID, errors.New("late error")) },
				"ready":      func() error { return repo.SaveFinalArtifacts(ctx, record.UUID, records.RecordFile{}, nil, nil) },
			} {
				if err := transition(); !errors.Is(err, records.ErrRecordStateChanged) {
					t.Fatalf("%s returned %v", name, err)
				}
			}
			got, err := repo.FindByUUID(ctx, record.UUID)
			if err != nil {
				t.Fatal(err)
			}
			if got.Status != status || !got.UpdatedAt.Equal(record.UpdatedAt) || got.EndedAt != nil {
				t.Fatalf("terminal record changed: %+v", got)
			}
		})
	}
}

func TestStopMetadataSurvivesLateStartAndDuplicateStop(t *testing.T) {
	repo, _ := testRepository(t)
	ctx := context.Background()
	record := createTestRecord(t, repo, records.StatusStarting)
	if err := repo.MarkStopping(ctx, record.UUID, "first reason"); err != nil {
		t.Fatal(err)
	}
	stopping, err := repo.FindByUUID(ctx, record.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.MarkRecording(ctx, record.UUID, "late-worker"); !errors.Is(err, records.ErrRecordStateChanged) {
		t.Fatal(err)
	}
	if err := repo.MarkStopping(ctx, record.UUID, "second reason"); !errors.Is(err, records.ErrRecordStateChanged) {
		t.Fatal(err)
	}
	got, err := repo.FindByUUID(ctx, record.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != records.StatusStopping || got.StartedAt != nil || got.EndedReason == nil || *got.EndedReason != "first reason" || !got.StoppedAt.Equal(*stopping.StoppedAt) {
		t.Fatalf("stop metadata was overwritten: %+v", got)
	}
	if err := repo.MarkFinalizing(ctx, record.UUID); err != nil {
		t.Fatal(err)
	}
	if err := repo.MarkUploading(ctx, record.UUID); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveFinalArtifacts(ctx, record.UUID, records.RecordFile{FileType: records.FileTypeFinalMP4, Bucket: "test", ObjectKey: record.UUID + "/final.mp4", MimeType: "video/mp4", IsPrimary: true}, nil, nil); err != nil {
		t.Fatal(err)
	}
	got, err = repo.FindByUUID(ctx, record.UUID)
	if err != nil || got.Status != records.StatusReady {
		t.Fatalf("record did not become ready: %v", err)
	}
}

func TestSummaryLoadsOnlyFinalFilesWithTwoQueries(t *testing.T) {
	repo, tx := testRepository(t)
	ctx := context.Background()
	record := createTestRecord(t, repo, records.StatusReady)
	for _, fileType := range []string{records.FileTypeFinalMP4, records.FileTypePreviewJPG, records.FileTypeDebugLog} {
		file := records.RecordFile{RecordID: record.ID, FileType: fileType, Bucket: "test", ObjectKey: record.UUID + "/" + fileType, MimeType: "application/octet-stream"}
		if err := tx.Create(&file).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Create(&records.RecordSegment{RecordID: record.ID, SeqNo: 1, Status: "closed"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := repo.AddEvent(ctx, record.UUID, "test", "worker", "info", "test event", "worker"); err != nil {
		t.Fatal(err)
	}
	counter := &queryCounter{Interface: logger.Default.LogMode(logger.Silent)}
	countedRepo := NewRecordRepository(tx.Session(&gorm.Session{Logger: counter}))
	details, err := countedRepo.ListSummaryDetailsByConferenceIDs(ctx, []string{record.ConferenceID}, records.StatusReady)
	if err != nil {
		t.Fatal(err)
	}
	if got := counter.queries.Load(); got != 2 {
		t.Fatalf("query count = %d, want 2", got)
	}
	if len(details) != 1 || len(details[0].Files) != 2 || len(details[0].Segments) != 0 || len(details[0].Events) != 0 {
		t.Fatalf("summary loaded unnecessary relations: %+v", details)
	}
	for _, file := range details[0].Files {
		if file.FileType == records.FileTypeDebugLog {
			t.Fatal("summary fetched debug logs")
		}
	}
	full, err := repo.FindDetailsByUUID(ctx, record.UUID)
	if err != nil || len(full.Files) != 3 || len(full.Segments) != 1 || len(full.Events) != 1 {
		t.Fatalf("full detail endpoint lost relations: %+v, %v", full, err)
	}
	filtered, err := countedRepo.ListSummaryDetailsByConferenceIDs(ctx, []string{record.ConferenceID}, records.StatusFailed)
	if err != nil || len(filtered) != 0 {
		t.Fatalf("status filter failed: %v", err)
	}
}

type queryCounter struct {
	logger.Interface
	queries atomic.Int32
}

func (l *queryCounter) Trace(context.Context, time.Time, func() (string, int64), error) {
	l.queries.Add(1)
}
