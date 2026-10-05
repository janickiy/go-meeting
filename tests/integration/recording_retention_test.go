package integration_test

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/records"
	pg "github.com/janickiy/go-recorder/internal/infrastructure/postgres"
	storage "github.com/janickiy/go-recorder/internal/infrastructure/storage/s3"
	"github.com/janickiy/go-recorder/internal/usecase/recorder"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"gorm.io/gorm"
)

func TestRecordingRetentionDeadlineAndRetry(t *testing.T) {
	f := stageSevenContent(t)
	ctx := context.Background()
	repo := pg.NewRecordRepository(f.db)
	control := pg.NewConferenceRecordingRepository(f.db)
	rid, cid := f.recording.UUID, f.conference.ID
	key := "recordings/" + cid + "/" + rid + "/artifacts/" + uuid.NewString() + "/final.mp4"
	if err := f.db.Create(&records.RecordFile{RecordID: f.recording.ID, FileType: records.FileTypeFinalMP4, Bucket: "recordings", ObjectKey: key, MimeType: "video/mp4", IsPrimary: true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.Create(&records.RecordSegment{RecordID: f.recording.ID, SeqNo: 1, Status: "uploaded", ObjectKey: &key}).Error; err != nil {
		t.Fatal(err)
	}
	// An old creation date does not shorten the week after finalization.
	if err := f.db.Exec("UPDATE record SET created_at=clock_timestamp()-interval '10 days', ended_at=clock_timestamp()-interval '7 days'+interval '1 minute' WHERE uuid=?", rid).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := repo.FindDetailsByUUID(ctx, rid); err != nil {
		t.Fatal("record expired early", err)
	}
	called := false
	if id, err := repo.ExpireNextRecording(ctx, nil, func(context.Context, records.Record) error { called = true; return nil }); err != nil || id != 0 || called {
		t.Fatal("fresh record selected", id, err)
	}
	if err := f.db.Exec("UPDATE record SET ended_at=clock_timestamp()-interval '7 days' WHERE uuid=?", rid).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := repo.FindDetailsByUUID(ctx, rid); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatal("expired record still readable", err)
	}
	if _, err := control.Accessible(ctx, f.owner.ID, cid, rid); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatal("conference endpoint still authorizes expired record", err)
	}
	if items, err := control.List(ctx, f.owner.ID, cid, 20, 0); err != nil || len(items) != 0 {
		t.Fatal("expired record still listed", items, err)
	}
	failure := errors.New("storage temporarily unavailable")
	if id, err := repo.ExpireNextRecording(ctx, nil, func(context.Context, records.Record) error { return failure }); id != f.recording.ID || !errors.Is(err, failure) {
		t.Fatal("failed deletion not reported", id, err)
	}
	var remaining int64
	f.db.Model(&records.RecordFile{}).Where("record_id=?", f.recording.ID).Count(&remaining)
	if remaining != 1 {
		t.Fatal("failed deletion removed references")
	}
	var deletionState struct{ DeletedAt *time.Time }
	if err := f.db.Table("record").Select("deleted_at").Where("uuid=?", rid).Take(&deletionState).Error; err != nil {
		t.Fatal(err)
	}
	if deletionState.DeletedAt != nil {
		t.Fatal("failure marked record deleted")
	}
	if id, err := repo.ExpireNextRecording(ctx, nil, func(_ context.Context, record records.Record) error {
		if record.UUID != rid {
			t.Fatal("wrong record selected")
		}
		return nil
	}); id != f.recording.ID || err != nil {
		t.Fatal("retry failed", id, err)
	}
	for _, table := range []string{"record_file", "record_segment"} {
		if err := f.db.Table(table).Where("record_id=?", f.recording.ID).Count(&remaining).Error; err != nil || remaining != 0 {
			t.Fatal("artifact references remain", table, remaining, err)
		}
	}
	var state struct {
		DeletedAt        *time.Time
		StorageObjectKey *string
	}
	if err := f.db.Table("record").Select("deleted_at,storage_object_key").Where("uuid=?", rid).Take(&state).Error; err != nil || state.DeletedAt == nil || state.StorageObjectKey != nil {
		t.Fatal("metadata not expired", state, err)
	}
	if id, err := repo.ExpireNextRecording(ctx, nil, func(context.Context, records.Record) error { t.Fatal("duplicate delete"); return nil }); id != 0 || err != nil {
		t.Fatal("completed deletion selected again", id, err)
	}
}

func TestRecordingRetentionProtectsActiveAndLeasedRecords(t *testing.T) {
	db := stageOneDatabase(t)
	repo := pg.NewRecordRepository(db)
	ctx := context.Background()
	old := time.Now().Add(-8 * 24 * time.Hour)
	lease := time.Now().Add(time.Hour)
	for _, status := range []string{"starting", "recording", "degraded", "stopping", "finalizing", "uploading", "ready", "partial_ready", "failed", "cancelled"} {
		record := records.Record{UUID: uuid.NewString(), ConferenceID: uuid.NewString(), Mode: "legacy", SourceType: "browser", TransportType: "webrtc", Status: status, QualityMode: "auto", SegmentDurationSec: 5, CreatedAt: old, UpdatedAt: old}
		if status == "ready" {
			record.RecorderLeaseUntil = &lease
		}
		_, err := repo.Create(ctx, record)
		if err != nil {
			t.Fatal(err)
		}
	}
	var attempted []int64
	for {
		id, err := repo.ExpireNextRecording(ctx, attempted, func(_ context.Context, record records.Record) error {
			if !records.IsTerminalStatus(record.Status) || record.Status == "ready" {
				t.Fatal("active or leased record selected", record.Status)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if id == 0 {
			break
		}
		attempted = append(attempted, id)
	}
	if len(attempted) != 3 {
		t.Fatal("terminal records not cleaned", attempted)
	}
}

func TestRecordingRetentionConcurrentReplicas(t *testing.T) {
	f := stageSevenContent(t)
	if err := f.db.Exec("UPDATE record SET ended_at=clock_timestamp()-interval '8 days' WHERE uuid=?", f.recording.UUID).Error; err != nil {
		t.Fatal(err)
	}
	repo := pg.NewRecordRepository(f.db)
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	go func() {
		_, err := repo.ExpireNextRecording(ctx, nil, func(context.Context, records.Record) error { close(entered); <-release; return nil })
		done <- err
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("cleanup did not acquire lock")
	}
	id, err := repo.ExpireNextRecording(ctx, nil, func(context.Context, records.Record) error {
		t.Error("second replica deleted locked record")
		return nil
	})
	close(release)
	if id != 0 || err != nil {
		t.Fatal("locked record not skipped", id, err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestRecordingRetentionObjectStorage(t *testing.T) {
	endpoint := os.Getenv("RECORDER_RETENTION_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("set RECORDER_RETENTION_S3_ENDPOINT for isolated object storage")
	}
	if !strings.HasPrefix(endpoint, "127.0.0.1:") && !strings.HasPrefix(endpoint, "localhost:") {
		t.Fatal("retention tests require local object storage")
	}
	ctx := context.Background()
	access, secret := os.Getenv("RECORDER_RETENTION_S3_ACCESS_KEY"), os.Getenv("RECORDER_RETENTION_S3_SECRET_KEY")
	bucket := "retention-test-" + uuid.NewString()
	s3, err := minio.New(endpoint, &minio.Options{Creds: credentials.NewStaticV4(access, secret, ""), Region: "us-east-1"})
	if err != nil {
		t.Fatal(err)
	}
	client, err := storage.NewClient(ctx, endpoint, access, secret, bucket, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for object := range s3.ListObjects(ctx, bucket, minio.ListObjectsOptions{Recursive: true, WithVersions: true}) {
			if object.Err == nil {
				_ = s3.RemoveObject(ctx, bucket, object.Key, minio.RemoveObjectOptions{VersionID: object.VersionID})
			}
		}
		if err := s3.RemoveBucket(ctx, bucket); err != nil {
			t.Error("remove test bucket", err)
		}
	})
	if os.Getenv("RECORDER_RETENTION_S3_VERSIONING") == "true" {
		if err := s3.SetBucketVersioning(ctx, bucket, minio.BucketVersioningConfiguration{Status: "Enabled"}); err != nil {
			t.Fatal(err)
		}
	}
	f := stageSevenContent(t)
	record := f.recording
	record.StorageBucket = &bucket
	base := "recordings/" + record.ConferenceID + "/" + record.UUID + "/"
	keys := []string{base + "artifacts/" + uuid.NewString() + "/final.mp4", base + "artifacts/" + uuid.NewString() + "/preview.jpg", base + "artifacts/" + uuid.NewString() + "/tracks.zip", "records/" + record.UUID + "/segments/one.webm"}
	sentinels := []string{"attachments/" + uuid.NewString() + "/chat.txt", "recordings/" + record.ConferenceID + "/" + uuid.NewString() + "/artifacts/" + uuid.NewString() + "/final.mp4"}
	for _, key := range append(keys, sentinels...) {
		for range 2 {
			if _, err := s3.PutObject(ctx, bucket, key, strings.NewReader("fixture"), 7, minio.PutObjectOptions{}); err != nil {
				t.Fatal(err)
			}
		}
	}
	oldURL, err := client.PresignedGetURL(ctx, keys[0], time.Hour)
	if err != nil || oldURL == "" {
		t.Fatal("create signed URL", err)
	}
	if err := f.db.Exec("UPDATE record SET ended_at=clock_timestamp()-interval '8 days', storage_bucket=? WHERE id=?", bucket, record.ID).Error; err != nil {
		t.Fatal(err)
	}
	if count, err := recorder.SweepRetention(ctx, pg.NewRecordRepository(f.db), client); count != 1 || err != nil {
		t.Fatal("cleanup failed", count, err)
	}
	response, err := http.Get(oldURL)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatal("previously issued URL still serves expired file", response.StatusCode)
	}
	for _, key := range keys {
		if _, err := s3.StatObject(ctx, bucket, key, minio.StatObjectOptions{}); err == nil {
			t.Fatal("expired artifact still exists", key)
		}
	}
	versions := 0
	for object := range s3.ListObjects(ctx, bucket, minio.ListObjectsOptions{Prefix: base, Recursive: true, WithVersions: true}) {
		if object.Err != nil {
			t.Fatal(object.Err)
		}
		versions++
	}
	if versions != 0 {
		t.Fatal("expired versions remain", versions)
	}
	for _, key := range sentinels {
		if _, err := s3.StatObject(ctx, bucket, key, minio.StatObjectOptions{}); err != nil {
			t.Fatal("unrelated object removed", key, err)
		}
	}
	if count, err := recorder.SweepRetention(ctx, pg.NewRecordRepository(f.db), client); count != 0 || err != nil {
		t.Fatal("cleanup not idempotent", count, err)
	}
}
