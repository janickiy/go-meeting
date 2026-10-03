package integration_test

import (
	"context"
	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/config"
	"github.com/janickiy/go-recorder/internal/domain/records"
	pg "github.com/janickiy/go-recorder/internal/infrastructure/postgres"
	redisinfra "github.com/janickiy/go-recorder/internal/infrastructure/redis"
	s3 "github.com/janickiy/go-recorder/internal/infrastructure/storage/s3"
	"github.com/janickiy/go-recorder/internal/usecase/recorder"
	"io"
	"log"
	"net/http"
	"os"
	"testing"
	"time"
)

// TestStageEightRecordingStorage проверяет реальный воркер, MinIO и актуальность аренды для каждой новой стратегии.
// @args t — исполнитель на отдельном локальном стенде с синтетическими участниками.
func TestStageEightRecordingStorage(t *testing.T) {
	if os.Getenv("RECORDER_STAGE8_STORAGE_E2E") != "true" {
		t.Skip("set RECORDER_STAGE8_STORAGE_E2E=true with isolated PostgreSQL/Redis/MinIO")
	}
	f := stageTwo(t)
	binary := stageEightFFmpeg(t)
	fixture := encodedFixture(t, binary)
	engine, mc := startMediaWithLimits(t, f, 4, 2, 2)
	a := newMediaTestPeerWithPublisher(t, f, 0, f.ownerToken, fixture.publish)
	b := newMediaTestPeerWithPublisher(t, f, 1, f.memberToken, fixture.publish)
	a.assertReceived(t, b.id())
	b.assertReceived(t, a.id())
	endpoint := os.Getenv("RECORDER_STAGE4_TEST_MINIO_ENDPOINT")
	if endpoint == "" {
		t.Fatal("explicit isolated MinIO endpoint required")
	}
	storage, err := s3.NewClient(context.Background(), endpoint, os.Getenv("RECORDER_STAGE4_TEST_MINIO_ACCESS_KEY"), os.Getenv("RECORDER_STAGE4_TEST_MINIO_SECRET_KEY"), "recordings", false)
	if err != nil {
		t.Fatal(err)
	}
	storage.SetPublicEndpoint(endpoint)
	t.Cleanup(func() { _ = storage.RemovePrefix(context.Background(), "recordings/"+f.conference.ID+"/") })
	repository := pg.NewRecordRepository(f.db)
	control := pg.NewConferenceRecordingRepository(f.db)
	ctx, cancel := context.WithCancel(context.Background())
	service := recorder.NewCompositeService(recorder.CompositeOptions{Repository: repository, Registry: redisinfra.NewMediaRegistry(f.redis, mc.Namespace), S3: storage, StoragePath: t.TempDir(), FFmpegPath: binary, WorkerID: "stage8-test-" + uuid.NewString(), InternalSecret: mc.InternalSecret, Config: config.CompositeConfig{Width: 320, Height: 240, FPS: 25, Concurrency: 1, MaxActive: 1, MaxBytes: 32 << 20, LeaseTTL: 10 * time.Second, PollInterval: 100 * time.Millisecond, MaxDuration: time.Minute, KeepLocal: true}, Logger: log.New(io.Discard, "", 0)})
	service.Start(ctx)
	t.Cleanup(func() { cancel(); service.Wait() })
	for _, mode := range []string{"composite", "audio_only", "individual_tracks", "screen_focus"} {
		t.Run(mode, func(t *testing.T) {
			record, created, err := control.StartMode(ctx, f.owner.ID, f.conference.ID, 2, mode)
			if err != nil || !created {
				t.Fatal("start", err)
			}
			duplicate, created, err := control.StartMode(ctx, f.owner.ID, f.conference.ID, 2, mode)
			if err != nil || created || duplicate.UUID != record.UUID {
				t.Fatal("idempotency", err)
			}
			if err = service.HandleCommand(ctx, records.Command{Type: "record.start", RecordID: record.UUID}, record); err != nil {
				t.Fatal(err)
			}
			waitRecording(t, repository, record.UUID, records.StatusRecording, 10*time.Second)
			time.Sleep(2300 * time.Millisecond)
			stopped, err := control.Stop(ctx, f.owner.ID, f.conference.ID, record.UUID)
			if err != nil {
				t.Fatal(err)
			}
			if err = service.HandleCommand(ctx, records.Command{Type: "record.stop", RecordID: record.UUID}, stopped); err != nil {
				t.Fatal(err)
			}
			ready := waitRecording(t, repository, record.UUID, records.StatusReady, 20*time.Second)
			card, err := recorder.NewService(repository, nil, storage, nil).ReadComposite(ctx, record.UUID)
			if err != nil {
				t.Fatal(err)
			}
			kinds := map[string]bool{}
			for _, file := range card.Files {
				kinds[file.FileType] = true
				response, err := http.Get(file.URL)
				if err != nil {
					t.Fatal(err)
				}
				size, err := io.Copy(io.Discard, response.Body)
				response.Body.Close()
				if err != nil || response.StatusCode != 200 || size <= 0 {
					t.Fatal("invalid private artifact", response.StatusCode, err)
				}
			}
			if mode == "audio_only" || mode == "individual_tracks" {
				if !kinds[records.FileTypeFinalAudio] || kinds[records.FileTypeFinalMP4] || kinds[records.FileTypePreviewJPG] {
					t.Fatal("audio mode artifact types", kinds)
				}
			} else if !kinds[records.FileTypeFinalMP4] || !kinds[records.FileTypePreviewJPG] {
				t.Fatal("video artifacts", kinds)
			}
			if (mode == "individual_tracks") != kinds[records.FileTypeTracksArchive] {
				t.Fatal("archive strategy", kinds)
			}
			if ready.Mode != mode || ready.StorageObjectKey == nil {
				t.Fatal("mode/storage lost")
			}
			var origin *time.Time
			if err = f.db.Raw(`SELECT media_started_at FROM record WHERE uuid=?`, record.UUID).Scan(&origin).Error; err != nil || origin == nil {
				t.Fatal("timeline not committed", err)
			}
			if state := engine.Snapshot(); state.Peers != 2 || state.Dropped != 0 {
				t.Fatal("recording affected conference", state)
			}
			t.Logf("mode=%s MinIO artifacts=%v duration=%ds", mode, kinds, *ready.DurationSec)
		})
	}
}
