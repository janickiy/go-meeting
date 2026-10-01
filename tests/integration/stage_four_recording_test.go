package integration_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	recordingsapp "github.com/janickiy/go-recorder/internal/app/recordings"
	"github.com/janickiy/go-recorder/internal/config"
	"github.com/janickiy/go-recorder/internal/domain/conferences"
	mediadomain "github.com/janickiy/go-recorder/internal/domain/media"
	"github.com/janickiy/go-recorder/internal/domain/realtime"
	"github.com/janickiy/go-recorder/internal/domain/records"
	"github.com/janickiy/go-recorder/internal/domain/users"
	"github.com/janickiy/go-recorder/internal/infrastructure/composite"
	"github.com/janickiy/go-recorder/internal/infrastructure/ffmpeg"
	pg "github.com/janickiy/go-recorder/internal/infrastructure/postgres"
	"github.com/janickiy/go-recorder/internal/infrastructure/rabbitmq"
	redisinfra "github.com/janickiy/go-recorder/internal/infrastructure/redis"
	s3storage "github.com/janickiy/go-recorder/internal/infrastructure/storage/s3"
	httptransport "github.com/janickiy/go-recorder/internal/transport/http"
	httpmiddleware "github.com/janickiy/go-recorder/internal/transport/http/middleware"
	"github.com/janickiy/go-recorder/internal/usecase/recorder"
	recordingusecase "github.com/janickiy/go-recorder/internal/usecase/recordings"
	"github.com/pion/rtp"
	"github.com/pion/rtp/codecs"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media/ivfreader"
	"github.com/pion/webrtc/v4/pkg/media/oggreader"
	amqp "github.com/rabbitmq/amqp091-go"
)

// TestStageFourCompositeRecording проверяет сценарий «этап четыре общая запись запись», фиксируя ошибки поведения как регрессию.
//
// @parameters:
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestStageFourCompositeRecording(t *testing.T) {
	if os.Getenv("RECORDER_STAGE4_RECORDING_E2E") != "true" {
		t.Skip("set RECORDER_STAGE4_RECORDING_E2E=true with local PostgreSQL/Redis/MinIO/RabbitMQ")
	}
	ffmpegPath := os.Getenv("RECORDER_TEST_FFMPEG")
	if ffmpegPath == "" {
		ffmpegPath = "/opt/homebrew/bin/ffmpeg"
	}
	if _, err := exec.LookPath(ffmpegPath); err != nil {
		t.Fatal("FFmpeg is required for recording acceptance")
	}
	fixtures := encodedFixture(t, ffmpegPath)
	// Разные участники не должны публиковать одну когерентную синусоиду:
	// задержка сетевого пути способна погасить её при обычном суммировании.
	secondFixtures := encodedFixtureFrequency(t, ffmpegPath, 660)
	f := stageTwo(t)
	engine, mediaConfig := startMediaWithLimits(t, f, 4, 2, 2)
	a := newMediaTestPeerWithPublisher(t, f, 0, f.ownerToken, fixtures.publish)
	b := newMediaTestPeerWithPublisher(t, f, 1, f.memberToken, secondFixtures.publish)
	a.assertReceived(t, b.id())
	b.assertReceived(t, a.id())
	endpoint := os.Getenv("RECORDER_STAGE4_TEST_MINIO_ENDPOINT")
	if endpoint == "" {
		endpoint = "localhost:9000"
	}
	access := os.Getenv("RECORDER_STAGE4_TEST_MINIO_ACCESS_KEY")
	if access == "" {
		access = "go_recorder"
	}
	secret := os.Getenv("RECORDER_STAGE4_TEST_MINIO_SECRET_KEY")
	if secret == "" {
		secret = "go_recorder_pass"
	}
	storage, err := s3storage.NewClient(context.Background(), endpoint, access, secret, "recordings", false)
	if err != nil {
		t.Fatal(err)
	}
	storage.SetPublicEndpoint(endpoint)
	rabbitURL := os.Getenv("RECORDER_STAGE4_TEST_RABBIT_URL")
	if rabbitURL == "" {
		rabbitURL = "amqp://go_recorder:go_recorder_pass@localhost:5672/%2F"
	}
	name := "stage4-test-" + uuid.NewString()
	options := rabbitmq.Options{URL: rabbitURL, Exchange: name, Queue: name, RoutingKey: "commands", ConsumerTag: name, Logger: log.New(io.Discard, "", 0)}
	publisher, err := rabbitmq.NewPublisher(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	consumer, err := rabbitmq.NewConsumer(context.Background(), options)
	if err != nil {
		publisher.Close()
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	dir := t.TempDir()
	repository := pg.NewRecordRepository(f.db)
	lock := redisinfra.NewConferenceLock(f.redis, time.Minute)
	config := config.CompositeConfig{Width: 640, Height: 360, FPS: 25, Concurrency: 1, MaxActive: 2, MaxBytes: 32 << 20, LeaseTTL: 10 * time.Second, PollInterval: 100 * time.Millisecond, MaxDuration: time.Minute, KeepLocal: true}
	var starts, stops atomic.Int32
	service := recorder.NewCompositeService(recorder.CompositeOptions{Repository: repository, Registry: redisinfra.NewMediaRegistry(f.redis, mediaConfig.Namespace), S3: storage, StoragePath: dir, FFmpegPath: ffmpegPath, WorkerID: name, InternalSecret: mediaConfig.InternalSecret, Config: config, ConferenceLock: lock, Logger: log.New(os.Stderr, "stage4-composite: ", 0), Publish: /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

	@parameters:
	  - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
	  - record (records.Record): задача записи с её сохранённым состоянием.
	  - kind (string): тип события, ошибки или медиа, определяющий ветку обработки.

	@return:
	  - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение. */func(ctx context.Context, record records.Record, kind string) error {
		event := realtime.Event(kind, record.ConferenceID, map[string]any{"recordingId": record.UUID, "status": records.PublicStatus(record.Status)})
		return f.store.Publish(ctx, realtime.Bus{Kind: "event", ConferenceID: record.ConferenceID, Event: &event})
	}})
	service.Start(ctx)
	var usageBefore syscall.Rusage
	_ = syscall.Getrusage(syscall.RUSAGE_SELF, &usageBefore)
	var peakFFmpeg atomic.Int32
	sampled := make(chan struct{})
	go /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

	 */func() {
		defer close(sampled)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				output, _ := exec.Command("pgrep", "-P", fmt.Sprint(os.Getpid()), "ffmpeg").Output()
				count := int32(len(strings.Fields(string(output))))
				for old := peakFFmpeg.Load(); count > old; old = peakFFmpeg.Load() {
					if peakFFmpeg.CompareAndSwap(old, count) {
						break
					}
				}
			}
		}
	}()
	consumerDone := make(chan error, 1)
	go /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

	 */func() {
		consumerDone <- consumer.Consume(ctx, /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

			@parameters:
			  - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
			  - command (records.Command): внутренняя команда с типом операции и серверной идентичностью ресурса.

			@return:
			  - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение. */func(ctx context.Context, command records.Command) error {
				if command.Type == "record.start" {
					starts.Add(1)
				}
				if command.Type == "record.stop" {
					stops.Add(1)
				}
				record, err := repository.FindByUUID(ctx, command.RecordID)
				if err != nil {
					return err
				}
				return service.HandleCommand(ctx, command, record)
			})
	}()
	reader := recorder.NewService(repository, publisher, storage, lock)
	orchestration := recordingusecase.NewConferenceService(pg.NewConferenceRecordingRepository(f.db), reader, publisher, lock, f.hubs[0])
	outboxDone := make(chan struct{})
	go /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

	 */func() { defer close(outboxDone); orchestration.Run(ctx) }()
	router := gin.New()
	httptransport.RegisterConferenceRecordingRoutes(router, recordingsapp.NewHandler(orchestration), httpmiddleware.Authenticate(f.tokens))
	api := httptest.NewServer(router)
	t.Cleanup( /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		 */func() {
			cancel()
			consumer.Close()
			<-consumerDone
			<-outboxDone
			service.Wait()
			<-sampled
			publisher.Close()
			api.Close()
			cleanupRabbit, err := amqp.Dial(rabbitURL)
			if err == nil {
				defer cleanupRabbit.Close()
				channel, err := cleanupRabbit.Channel()
				if err == nil {
					defer channel.Close()
					_, _ = channel.QueueDelete(name, false, false, false)
					_ = channel.ExchangeDelete(name, false, false)
				}
			}
			_ = storage.RemovePrefix(context.Background(), "recordings/"+f.conference.ID+"/")
		})
	var created struct {
		Item records.RecordCard `json:"item"`
	}
	recordingRequest(t, api.URL, f.ownerToken, "POST", "/api/v1/conferences/"+f.conference.ID+"/recordings", records.ConferenceStartRequest{SegmentDurationSec: 2}, http.StatusAccepted, &created)
	id := created.Item.UUID
	if id == "" {
		t.Fatal("missing recording UUID")
	}
	var duplicate struct {
		Item records.RecordCard `json:"item"`
	}
	recordingRequest(t, api.URL, f.ownerToken, "POST", "/api/v1/conferences/"+f.conference.ID+"/recordings", records.ConferenceStartRequest{SegmentDurationSec: 2}, http.StatusAccepted, &duplicate)
	if duplicate.Item.UUID != id {
		t.Fatal("duplicate start created another recording")
	}
	waitRecording(t, repository, id, records.StatusRecording, 15*time.Second)
	recordingObservedAt := time.Now()
	time.Sleep(2200 * time.Millisecond)
	screenSender := addRecordingScreen(t, b)
	deadline := time.Now().Add(8 * time.Second)
	for {
		found := false
		for _, track := range engine.Tracks(a.id()) {
			if track.Source == mediadomain.SourceVideoScreen {
				found = true
			}
		}
		if found {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("screen was not published")
		}
		time.Sleep(20 * time.Millisecond)
	}
	time.Sleep(2200 * time.Millisecond)
	third := users.User{ID: uuid.NewString(), Email: "third@recording.example", PasswordHash: f.owner.PasswordHash}
	if _, err := pg.NewUserRepository(f.db).Create(context.Background(), third); err != nil {
		t.Fatal(err)
	}
	thirdParticipant, err := f.service.Join(context.Background(), third.ID, f.conference.ID, conferences.JoinRequest{InviteCode: f.conference.InviteCode})
	if err != nil {
		t.Fatal(err)
	}
	thirdToken, _ := f.tokens.Issue(third.ID)
	c := newMediaTestPeerWithPublisher(t, f, 0, thirdToken, fixtures.publish)
	c.assertReceived(t, a.id(), b.id())
	time.Sleep(2200 * time.Millisecond)
	blockedPolicy := true
	recordingRequest(t, f.servers[0].URL, f.ownerToken, "POST", "/api/v1/conferences/"+f.conference.ID+"/participants/"+thirdParticipant.ID+"/moderation", conferences.ModerationRequest{Action: "mute", Blocked: &blockedPolicy}, http.StatusOK, nil)
	deadline = time.Now().Add(5 * time.Second)
	for {
		found := false
		for _, track := range engine.Tracks(a.id()) {
			if track.ParticipantID == thirdParticipant.ID && track.Kind == mediadomain.KindAudio {
				found = true
			}
		}
		if !found {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("moderated microphone still forwarded during recording")
		}
		time.Sleep(20 * time.Millisecond)
	}
	c.close()
	removed := make(chan struct{})
	b.actions <- /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

	 */func() { b.report(b.pc.RemoveTrack(screenSender)); close(removed) }
	<-removed
	deadline = time.Now().Add(8 * time.Second)
	for {
		found := false
		for _, track := range engine.Tracks(a.id()) {
			if track.Source == mediadomain.SourceVideoScreen {
				found = true
			}
		}
		if !found {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("screen remained after unpublish")
		}
		time.Sleep(20 * time.Millisecond)
	}
	time.Sleep(4200 * time.Millisecond)
	var before runtime.MemStats
	runtime.ReadMemStats(&before)
	t.Logf("recording baseline in-process SFU+recorder+clients: goroutines=%d heap=%d bytes SFU=%+v", runtime.NumGoroutine(), before.HeapAlloc, engine.Snapshot())
	stopAt := time.Now()
	recordingRequest(t, api.URL, f.ownerToken, "POST", "/api/v1/conferences/"+f.conference.ID+"/recordings/"+id+"/stop", struct{}{}, http.StatusAccepted, nil)
	ready := waitRecording(t, repository, id, records.StatusReady, 45*time.Second)
	t.Logf("finalization=%s duration=%ds size=%d", time.Since(stopAt), *ready.DurationSec, *ready.SizeBytes)
	if starts.Load() == 0 || stops.Load() == 0 {
		t.Fatal("RabbitMQ start/stop chain was not exercised")
	}
	var conf conferences.Conference
	if err := f.db.First(&conf, "id = ?", f.conference.ID).Error; err != nil || conf.Status != conferences.Active {
		t.Fatalf("recording stop changed conference: %v %+v", err, conf)
	}
	result, err := ffmpeg.NewPostProcessor(ffmpegPath).ValidateOutput(context.Background(), filepath.Join(dir, "records", id, "final.mp4"), true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Duration < 8 || result.Size < 10000 {
		t.Fatalf("implausible composite output: %+v", result)
	}
	if expected := stopAt.Sub(recordingObservedAt).Seconds(); result.Duration < expected-0.25 {
		t.Fatalf("recording tail missing: output=%.3fs observed capture=%.3fs", result.Duration, expected)
	}
	preview, err := os.Stat(filepath.Join(dir, "records", id, "preview.jpg"))
	if err != nil || preview.Size() == 0 {
		t.Fatal("preview missing", err)
	}
	manifests, _ := filepath.Glob(filepath.Join(dir, "records", id, "chunk_*.json"))
	var layouts []string
	maxCameras := 0
	for _, path := range manifests {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var chunk composite.Chunk
		if err := json.Unmarshal(data, &chunk); err != nil {
			t.Fatal(err)
		}
		layouts = append(layouts, chunk.Layout.Name)
		t.Logf("chunk %d duration=%.3fs layout=%s sources=%d", chunk.Index, chunk.Duration, chunk.Layout.Name, len(chunk.Sources))
		cameras := 0
		for _, source := range chunk.Sources {
			if source.Track.Source == mediadomain.SourceCamera {
				cameras++
			}
		}
		maxCameras = max(maxCameras, cameras)
	}
	if len(layouts) < 4 || layouts[0] != "grid" || layouts[len(layouts)-1] != "grid" || !strings.Contains(strings.Join(layouts, ","), "screen") || maxCameras < 3 {
		t.Fatalf("dynamic layouts missing: %v maxCameras=%d", layouts, maxCameras)
	}
	card, err := orchestration.Read(context.Background(), f.owner.ID, f.conference.ID, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(card.Files) != 2 {
		t.Fatalf("missing storage metadata: %+v", card.Files)
	}
	for _, artifact := range card.Files {
		response, err := http.Get(artifact.URL)
		if err != nil {
			t.Fatal(err)
		}
		size, err := io.Copy(io.Discard, response.Body)
		response.Body.Close()
		if err != nil || response.StatusCode != http.StatusOK || size == 0 {
			t.Fatalf("MinIO artifact invalid status=%d size=%d", response.StatusCode, size)
		}
	}
	t.Logf("ffprobe=%+v layouts=%v MinIO artifacts=%d", result, layouts, len(card.Files))
	if export := os.Getenv("RECORDER_STAGE4_ARTIFACT_DIR"); export != "" {
		if !filepath.IsAbs(export) {
			t.Fatal("artifact directory must be absolute")
		}
		if err := os.MkdirAll(export, 0o750); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"final.mp4", "preview.jpg"} {
			source, err := os.Open(filepath.Join(dir, "records", id, name))
			if err != nil {
				t.Fatal(err)
			}
			output, err := os.OpenFile(filepath.Join(export, id+"_"+name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
			if err != nil {
				source.Close()
				t.Fatal(err)
			}
			_, copyErr := io.Copy(output, source)
			source.Close()
			output.Close()
			if copyErr != nil {
				t.Fatal(copyErr)
			}
		}
		frame := filepath.Join(export, id+"_screen.jpg")
		if _, err := os.Stat(frame); !os.IsNotExist(err) {
			t.Fatal("screen artifact already exists")
		}
		output, err := exec.Command(ffmpegPath, "-hide_banner", "-loglevel", "error", "-ss", "4", "-i", filepath.Join(dir, "records", id, "final.mp4"), "-frames:v", "1", frame).CombinedOutput()
		if err != nil {
			t.Fatalf("export screen frame: %v %s", err, output)
		}
		t.Logf("exported recording artifacts: %s/%s_{final.mp4,preview.jpg,screen.jpg}", export, id)
	}
	assertCompositeContent(t, ffmpegPath, filepath.Join(dir, "records", id, "final.mp4"))
	var usageAfter syscall.Rusage
	_ = syscall.Getrusage(syscall.RUSAGE_SELF, &usageAfter)
	cpuMicros := usageAfter.Utime.Sec*1e6 + int64(usageAfter.Utime.Usec) + usageAfter.Stime.Sec*1e6 + int64(usageAfter.Stime.Usec) - (usageBefore.Utime.Sec*1e6 + int64(usageBefore.Utime.Usec) + usageBefore.Stime.Sec*1e6 + int64(usageBefore.Stime.Usec))
	t.Logf("combined test process CPU=%s peakRSS(OS units)=%d peakFFmpegChildren=%d recordingDrops=%d", time.Duration(cpuMicros)*time.Microsecond, usageAfter.Maxrss, peakFFmpeg.Load(), engine.Snapshot().RecordingDrops)
	// Simulate an expired recorder after a durable chunk and stop were already
	// committed. Recovery is driven by DB state, without a duplicate capture.
	recoveryID := uuid.NewString()
	recoveryDir := filepath.Join(dir, "records", recoveryID)
	if err := os.MkdirAll(recoveryDir, 0o750); err != nil {
		t.Fatal(err)
	}
	seed, err := os.ReadFile(filepath.Join(dir, "records", id, "segment_000000.mp4"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(recoveryDir, "segment_000000.mp4"), seed, 0o640); err != nil {
		t.Fatal(err)
	}
	oldToken := uuid.NewString()
	expired := time.Now().Add(-time.Minute)
	_, err = repository.Create(context.Background(), records.Record{UUID: recoveryID, ConferenceID: f.conference.ID, PlatformConferenceID: &f.conference.ID, RequestedBy: &f.owner.ID, Mode: records.ModeComposite, SourceType: "conference", TransportType: "sfu", Status: records.StatusStopping, QualityMode: "auto", SegmentDurationSec: 2, NeedPreview: true, RecorderToken: &oldToken, RecorderLeaseUntil: &expired})
	if err != nil {
		t.Fatal(err)
	}
	recovered := waitRecording(t, repository, recoveryID, records.StatusReady, 10*time.Second)
	if recovered.StorageObjectKey == nil || strings.Contains(*recovered.StorageObjectKey, oldToken) {
		t.Fatal("recovery published old incarnation artifact")
	}
	t.Logf("expired recorder recovery finalized closed segment: recording=%s duration=%d", recoveryID, *recovered.DurationSec)
	// A failed second recorder (unavailable egress) must leave the live SFU and
	// the conference intact, and must never publish ready.
	deadline = time.Now().Add(3 * time.Second)
	for engine.Snapshot().RecordingOutputs != 0 {
		if time.Now().After(deadline) {
			t.Fatal("recording egress leaked after stop")
		}
		time.Sleep(10 * time.Millisecond)
	}
	blocked, err := engine.SubscribeRecording(context.Background(), f.conference.ID, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(blocked.Close)
	failed, _, err := pg.NewConferenceRecordingRepository(f.db).Start(context.Background(), f.owner.ID, f.conference.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.HandleCommand(context.Background(), records.Command{Type: "record.start", RecordID: failed.UUID}, failed); err != nil {
		t.Fatal(err)
	}
	waitRecording(t, repository, failed.UUID, records.StatusFailed, 5*time.Second)
	a.assertReceived(t, b.id())
	b.assertReceived(t, a.id())
	if err := f.db.First(&conf, "id = ?", f.conference.ID).Error; err != nil || conf.Status != conferences.Active {
		t.Fatal("recording failure ended conference")
	}
	blocked.Close()
	// Finish commits stopping and an outbox command in the same transaction.
	// The recorder must close its final partial chunk and upload without keeping
	// browser peers or the room alive afterwards.
	var finishRecord struct {
		Item records.RecordCard `json:"item"`
	}
	recordingRequest(t, api.URL, f.ownerToken, "POST", "/api/v1/conferences/"+f.conference.ID+"/recordings", records.ConferenceStartRequest{SegmentDurationSec: 2}, http.StatusAccepted, &finishRecord)
	waitRecording(t, repository, finishRecord.Item.UUID, records.StatusRecording, 10*time.Second)
	time.Sleep(2350 * time.Millisecond)
	if _, err := f.service.Transition(context.Background(), f.owner.ID, f.conference.ID, conferences.Finished); err != nil {
		t.Fatal(err)
	}
	finished := waitRecording(t, repository, finishRecord.Item.UUID, records.StatusReady, 20*time.Second)
	if finished.EndedReason == nil || *finished.EndedReason != "conference_finished" {
		t.Fatalf("missing automatic stop reason: %+v", finished.EndedReason)
	}
	finishProbe, err := ffmpeg.NewPostProcessor(ffmpegPath).ValidateOutput(context.Background(), filepath.Join(dir, "records", finished.UUID, "final.mp4"), true)
	if err != nil {
		t.Fatal(err)
	}
	if finishProbe.Duration < 2.2 {
		t.Fatalf("finish lost final partial segment: %.3fs", finishProbe.Duration)
	}
	t.Logf("conference finish auto-stop validated duration=%.3fs", finishProbe.Duration)
}

// assertCompositeContent подготавливает или проверяет часть тестового сценария «проверка общая запись Content».
//
// @parameters:
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
//   - ffmpegPath (string): значение ffmpegPath типа string, используемое согласно назначению этой операции.
//   - path (string): путь к локальному файлу или каталогу операции.
func assertCompositeContent(t *testing.T, ffmpegPath, path string) {
	t.Helper()
	// Synthetic fixtures are continuously colourful/voiced. Check decoded
	// content across every segment boundary, not merely stream metadata.
	pixels, err := exec.Command(ffmpegPath, "-hide_banner", "-loglevel", "error", "-i", path, "-an", "-vf", "fps=4,scale=32:18", "-pix_fmt", "rgb24", "-f", "rawvideo", "-").Output()
	if err != nil {
		t.Fatal(err)
	}
	frameBytes := 32 * 18 * 3
	for frame := 5; frame < len(pixels)/frameBytes; frame++ {
		coloured := 0
		for i := frame * frameBytes; i < (frame+1)*frameBytes; i += 3 {
			high := max(pixels[i], pixels[i+1], pixels[i+2])
			low := min(pixels[i], pixels[i+1], pixels[i+2])
			if high > 70 && high-low > 35 {
				coloured++
			}
		}
		// During the bounded screen-stop layout transition, only small camera
		// tiles in the side column remain; any fully blank frame is still a bug.
		if coloured < 8 {
			t.Fatalf("blank composite frame at %.2fs: colourful pixels=%d", float64(frame)/4, coloured)
		}
	}
	pcm, err := exec.Command(ffmpegPath, "-hide_banner", "-loglevel", "error", "-i", path, "-vn", "-ac", "1", "-ar", "8000", "-f", "f32le", "-").Output()
	if err != nil {
		t.Fatal(err)
	}
	window := 1600
	for offset := 8000; offset+window < len(pcm)/4; offset += window {
		var squares float64
		for i := offset; i < offset+window; i++ {
			sample := float64(math.Float32frombits(binary.LittleEndian.Uint32(pcm[i*4:])))
			squares += sample * sample
		}
		if rms := math.Sqrt(squares / float64(window)); rms < 0.003 {
			t.Fatalf("audio gap at %.2fs: RMS=%f", float64(offset)/8000, rms)
		}
	}
	t.Logf("decoded content validated: %d video frames at 4fps and %.2fs mixed audio", len(pixels)/frameBytes, float64(len(pcm)/4)/8000)
}

// TestStageFourRecorderLeaseFencing проверяет сценарий «этап четыре Recorder аренда защита версии владения», фиксируя ошибки поведения как регрессию.
//
// @parameters:
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestStageFourRecorderLeaseFencing(t *testing.T) {
	f := stageTwo(t)
	repository := pg.NewRecordRepository(f.db)
	record, _, err := pg.NewConferenceRecordingRepository(f.db).Start(context.Background(), f.owner.ID, f.conference.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	first, second := uuid.NewString(), uuid.NewString()
	ctx := context.Background()
	if ok, err := repository.ClaimComposite(ctx, record.UUID, first, "first", time.Minute); err != nil || !ok {
		t.Fatal("first claim failed", err)
	}
	if ok, err := repository.ClaimComposite(ctx, record.UUID, second, "second", time.Minute); err != nil || ok {
		t.Fatal("duplicate recorder admitted", err)
	}
	if err := f.db.Model(&records.Record{}).Where("uuid = ?", record.UUID).Update("recorder_lease_until", time.Now().Add(-time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	if ok, err := repository.ClaimComposite(ctx, record.UUID, second, "second", time.Minute); err != nil || !ok {
		t.Fatal("expired owner was not replaced", err)
	}
	if ok, err := repository.RenewComposite(ctx, record.UUID, first, time.Minute); err != nil || ok {
		t.Fatal("stale owner renewed", err)
	}
	if err := repository.TransitionComposite(ctx, record.UUID, first, records.StatusRecording, nil); err == nil {
		t.Fatal("stale owner changed status")
	}
	if err := repository.MarkStopping(ctx, record.UUID, "fence_test"); err != nil {
		t.Fatal(err)
	}
	if err := repository.TransitionComposite(ctx, record.UUID, second, records.StatusFinalizing, nil); err != nil {
		t.Fatal(err)
	}
	if err := repository.TransitionComposite(ctx, record.UUID, second, records.StatusUploading, nil); err != nil {
		t.Fatal(err)
	}
	if err := repository.SaveCompositeArtifacts(ctx, record.UUID, first, records.RecordFile{}, nil, nil); err == nil {
		t.Fatal("stale owner committed ready")
	}
	current, err := repository.FindByUUID(ctx, record.UUID)
	if err != nil || current.Status != records.StatusUploading {
		t.Fatal("stale save changed record", err)
	}
}

// waitRecording подготавливает или проверяет часть тестового сценария «ожидание запись».
//
// @parameters:
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
//   - repo (*pg.RecordRepository): хранилище постоянных данных прикладного сценария.
//   - id (string): идентификатор обрабатываемого ресурса.
//   - status (string): состояние ресурса, ответа или фильтра выборки.
//   - timeout (time.Duration): максимальное время ожидания операции.
//
// @return:
//   - результат 1 (records.Record): значение, подготовленное операцией для вызывающей стороны.
func waitRecording(t *testing.T, repo *pg.RecordRepository, id, status string, timeout time.Duration) records.Record {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		record, err := repo.FindByUUID(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		if record.Status == status {
			return record
		}
		if record.Status == records.StatusFailed {
			t.Fatalf("recording failed: %v", record.ErrorMessage)
		}
		if time.Now().After(deadline) {
			t.Fatalf("recording stuck in %s, expected %s", record.Status, status)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// recordingRequest подготавливает или проверяет часть тестового сценария «запись Request».
//
// @parameters:
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
//   - base (string): значение base типа string, используемое согласно назначению этой операции.
//   - token (string): подписанный токен или токен владения, который необходимо проверить.
//   - method (string): значение method типа string, используемое согласно назначению этой операции.
//   - path (string): путь к локальному файлу или каталогу операции.
//   - payload (any): типизированная нагрузка события или ссылочные сведения уведомления.
//   - status (int): состояние ресурса, ответа или фильтра выборки.
//   - result (any): результат проверки или обработки, передаваемый следующему шагу.
func recordingRequest(t *testing.T, base, token, method, path string, payload any, status int, result any) {
	t.Helper()
	body, _ := json.Marshal(payload)
	request, err := http.NewRequest(method, base+path, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	response, err := (&http.Client{Timeout: 5 * time.Second}).Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if response.StatusCode != status {
		t.Fatalf("recording request %s: %d %s", path, response.StatusCode, data)
	}
	if result != nil {
		if err := json.Unmarshal(data, result); err != nil {
			t.Fatal(err)
		}
	}
}

// recordingFixture хранит изолированное состояние тестового компонента «запись тестовое окружение».
// Состав:
//   - video: набор значений video для последовательной или пакетной обработки.
//   - audio: набор значений audio для последовательной или пакетной обработки.
type recordingFixture struct{ video, audio [][]byte }

// encodedFixture подготавливает или проверяет часть тестового сценария «encoded тестовое окружение».
//
// @parameters:
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
//   - ffmpegPath (string): значение ffmpegPath типа string, используемое согласно назначению этой операции.
//
// @return:
//   - результат 1 (recordingFixture): значение, подготовленное операцией для вызывающей стороны.
func encodedFixture(t *testing.T, ffmpegPath string) recordingFixture {
	return encodedFixtureFrequency(t, ffmpegPath, 440)
}

// encodedFixtureFrequency создаёт валидные VP8/Opus пакеты с независимым тоном.
// t управляет временными файлами, ffmpegPath задаёт доверенный бинарник,
// frequency — частоту синусоиды, чтобы смешивание не давало фазового погашения.
// Возвращает повторяемый источник закодированных RTP payloads.
func encodedFixtureFrequency(t *testing.T, ffmpegPath string, frequency int) recordingFixture {
	t.Helper()
	dir := t.TempDir()
	video := filepath.Join(dir, "video.ivf")
	audio := filepath.Join(dir, "audio.ogg")
	for _, args := range [][]string{{"-f", "lavfi", "-i", "testsrc2=size=160x90:rate=25", "-t", "2", "-c:v", "libvpx", "-deadline", "realtime", "-g", "25", "-b:v", "150k", "-an", video}, {"-f", "lavfi", "-i", fmt.Sprintf("sine=frequency=%d:sample_rate=48000", frequency), "-t", "2", "-c:a", "libopus", "-ac", "2", "-frame_duration", "20", "-page_duration", "20000", audio}} {
		output, err := exec.Command(ffmpegPath, append([]string{"-hide_banner", "-loglevel", "error", "-y"}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("fixture generation: %v %s", err, output)
		}
	}
	var fixture recordingFixture
	v, err := os.Open(video)
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	vr, _, err := ivfreader.NewWith(v)
	if err != nil {
		t.Fatal(err)
	}
	for {
		frame, _, err := vr.ParseNextFrame()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		fixture.video = append(fixture.video, frame)
	}
	a, err := os.Open(audio)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	ar, _, err := oggreader.NewWith(a)
	if err != nil {
		t.Fatal(err)
	}
	for {
		frame, _, err := ar.ParseNextPage()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if bytes.HasPrefix(frame, []byte("OpusTags")) {
			continue
		}
		fixture.audio = append(fixture.audio, frame)
	}
	if len(fixture.video) != 50 || len(fixture.audio) < 100 {
		t.Fatalf("unexpected fixture frames: video=%d audio=%d", len(fixture.video), len(fixture.audio))
	}
	return fixture
}

// publish передаёт сохранённое изменение через транспорт событий или внутренних команд.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @parameters:
//   - p (*mediaTestPeer): байты, переданные по контракту io.Writer.
func (f recordingFixture) publish(p *mediaTestPeer) {
	defer p.wg.Done()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	packetizers := map[string]rtp.Packetizer{}
	tick := 0
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			p.mu.Lock()
			tracks := append([]*webrtc.TrackLocalStaticRTP(nil), p.local...)
			p.mu.Unlock()
			for _, track := range tracks {
				video := track.Kind() == webrtc.RTPCodecTypeVideo
				if video && tick%2 != 0 {
					continue
				}
				packetizer := packetizers[track.ID()]
				if packetizer == nil {
					var payloader rtp.Payloader = &codecs.OpusPayloader{}
					rate := uint32(48000)
					if video {
						payloader = &codecs.VP8Payloader{}
						rate = 90000
					}
					packetizer = rtp.NewPacketizer(1200, 96, 1, payloader, rtp.NewRandomSequencer(), rate)
					packetizers[track.ID()] = packetizer
				}
				payload := f.audio[tick%len(f.audio)]
				samples := uint32(960)
				if video {
					payload = f.video[(tick/2)%len(f.video)]
					samples = 3600
				}
				for _, packet := range packetizer.Packetize(payload, samples) {
					_ = track.WriteRTP(packet)
				}
			}
			tick++
		}
	}
}

// addRecordingScreen подготавливает или проверяет часть тестового сценария «add запись экран».
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @parameters:
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
//   - p (*mediaTestPeer): байты, переданные по контракту io.Writer.
//
// @return:
//   - результат 1 (*webrtc.RTPSender): значение, подготовленное операцией для вызывающей стороны.
func addRecordingScreen(t *testing.T, p *mediaTestPeer) *webrtc.RTPSender {
	t.Helper()
	result := make(chan *webrtc.RTPSender, 1)
	p.actions <- /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.
	Синхронизирует доступ к разделяемому состоянию блокировкой.

	*/func() {
		track, err := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8, ClockRate: 90000}, uuid.NewString(), uuid.NewString())
		if err != nil {
			p.report(err)
			result <- nil
			return
		}
		sender, err := p.pc.AddTrack(track)
		if err != nil {
			p.report(err)
			result <- nil
			return
		}
		p.mu.Lock()
		p.local = append(p.local, track)
		if p.sources == nil {
			p.sources = map[string]mediadomain.Source{}
		}
		p.sources[track.ID()] = mediadomain.SourceVideoScreen
		p.mu.Unlock()
		p.wg.Add(1)
		go /* Вложенный обработчик выполняет выделенный шаг обработки в проверках поведения приложения, используя состояние окружающей функции.

		 */func() {
			defer p.wg.Done()
			for {
				if _, _, err := sender.ReadRTCP(); err != nil {
					return
				}
			}
		}()
		result <- sender
	}
	select {
	case sender := <-result:
		if sender == nil {
			t.Fatal("screen add failed")
		}
		return sender
	case <-time.After(5 * time.Second):
		t.Fatal("screen add timeout")
		return nil
	}
}
