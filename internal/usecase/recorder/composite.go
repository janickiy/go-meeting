package recorder

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/config"
	"github.com/janickiy/go-recorder/internal/domain/media"
	"github.com/janickiy/go-recorder/internal/domain/records"
	"github.com/janickiy/go-recorder/internal/infrastructure/composite"
	"github.com/janickiy/go-recorder/internal/infrastructure/ffmpeg"
	s3storage "github.com/janickiy/go-recorder/internal/infrastructure/storage/s3"
)

type CompositeRepository interface {
	FindByUUID(context.Context, string) (records.Record, error)
	ListActiveComposite(context.Context) ([]records.Record, error)
	ClaimComposite(context.Context, string, string, string, time.Duration) (bool, error)
	RenewComposite(context.Context, string, string, time.Duration) (bool, error)
	ReleaseComposite(context.Context, string, string) error
	TransitionComposite(context.Context, string, string, string, error) error
	SaveCompositeArtifacts(context.Context, string, string, records.RecordFile, *records.RecordFile, []records.RecordSegment) error
	MarkStopping(context.Context, string, string) error
	AddEvent(context.Context, string, string, string, string, string, string) error
}

type CompositeRegistry interface {
	GetOwner(context.Context, string) (media.Route, error)
}

type CompositeOptions struct {
	Repository     CompositeRepository
	Registry       CompositeRegistry
	S3             *s3storage.Client
	StoragePath    string
	FFmpegPath     string
	WorkerID       string
	InternalSecret string
	Config         config.CompositeConfig
	ConferenceLock conferenceReleaser
	// Publish is called after a committed lifecycle transition. It must use
	// bounded I/O and must not call back into this service.
	Publish func(context.Context, records.Record, string) error
	Logger  *log.Logger
}

// CompositeService owns recorder lifecycle independently of RabbitMQ delivery
// duration. Its database lease fences capture, FFmpeg and artifact publication.
type CompositeService struct {
	o         CompositeOptions
	composer  *composite.Composer
	processor *ffmpeg.PostProcessor
	client    *http.Client
	mu        sync.Mutex
	ctx       context.Context
	running   map[string]struct{}
	wg        sync.WaitGroup
	started   bool
}

func NewCompositeService(o CompositeOptions) *CompositeService {
	if o.Logger == nil {
		o.Logger = log.Default()
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.ResponseHeaderTimeout = 5 * time.Second
	return &CompositeService{o: o, composer: composite.NewComposer(o.FFmpegPath, o.Config.Width, o.Config.Height, o.Config.FPS, o.Config.Concurrency), processor: ffmpeg.NewPostProcessor(o.FFmpegPath), client: &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, ctx: context.Background(), running: map[string]struct{}{}}
}

func (s *CompositeService) Start(ctx context.Context) {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return
	}
	s.ctx = ctx
	s.started = true
	s.wg.Add(1)
	s.mu.Unlock()
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(s.o.Config.PollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.reconcile(ctx)
			}
		}
	}()
}

func (s *CompositeService) Wait() { s.wg.Wait(); s.client.CloseIdleConnections() }

func (s *CompositeService) HandleCommand(ctx context.Context, command records.Command, record records.Record) error {
	switch command.Type {
	case "record.start":
		if records.IsTerminalStatus(record.Status) {
			return nil
		}
		return s.launch(ctx, record)
	case "record.stop":
		if records.IsTerminalStatus(record.Status) {
			return nil
		}
		if record.Status == records.StatusStarting || record.Status == records.StatusRecording {
			if err := s.o.Repository.MarkStopping(ctx, record.UUID, command.Reason); err != nil && !errors.Is(err, records.ErrRecordStateChanged) {
				return err
			}
			record.Status = records.StatusStopping
			s.publish(ctx, record, "recording.stopping")
		}
		// A command can reach another replica. The owner's DB poll observes
		// stopping; launch only succeeds if no live owner currently exists.
		return s.launch(ctx, record)
	default:
		return fmt.Errorf("unknown composite command")
	}
}

func (s *CompositeService) reconcile(ctx context.Context) {
	queryCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	records, err := s.o.Repository.ListActiveComposite(queryCtx)
	if err != nil {
		return
	}
	for _, record := range records {
		if err := s.launch(queryCtx, record); err != nil {
			s.o.Logger.Printf("composite reconciliation record=%s failed", record.UUID)
		}
	}
}

func (s *CompositeService) launch(ctx context.Context, record records.Record) error {
	s.mu.Lock()
	if _, ok := s.running[record.UUID]; ok || len(s.running) >= s.o.Config.MaxActive || s.ctx.Err() != nil {
		s.mu.Unlock()
		return nil
	}
	s.running[record.UUID] = struct{}{}
	workParent := s.ctx
	s.wg.Add(1)
	s.mu.Unlock()
	token := uuid.NewString()
	claimed, err := s.o.Repository.ClaimComposite(ctx, record.UUID, token, s.o.WorkerID, s.o.Config.LeaseTTL)
	if err != nil || !claimed {
		s.mu.Lock()
		delete(s.running, record.UUID)
		s.mu.Unlock()
		s.wg.Done()
		return err
	}
	go func() {
		defer s.wg.Done()
		defer func() { s.mu.Lock(); delete(s.running, record.UUID); s.mu.Unlock() }()
		workCtx, cancel := context.WithCancel(workParent)
		defer cancel()
		renewed := make(chan struct{})
		go func() {
			defer close(renewed)
			ticker := time.NewTicker(s.o.Config.LeaseTTL / 4)
			defer ticker.Stop()
			for {
				select {
				case <-workCtx.Done():
					return
				case <-ticker.C:
					checkCtx, done := context.WithTimeout(workCtx, s.o.Config.LeaseTTL/8)
					ok, err := s.o.Repository.RenewComposite(checkCtx, record.UUID, token, s.o.Config.LeaseTTL)
					done()
					if err != nil || !ok {
						cancel()
						return
					}
				}
			}
		}()
		err := s.run(workCtx, record, token)
		if err != nil {
			s.fail(record, token, err)
		}
		cancel()
		<-renewed
		releaseCtx, done := context.WithTimeout(context.Background(), 3*time.Second)
		defer done()
		_ = s.o.Repository.ReleaseComposite(releaseCtx, record.UUID, token)
	}()
	return nil
}

func (s *CompositeService) run(ctx context.Context, record records.Record, token string) error {
	dir := filepath.Join(s.o.StoragePath, "records", record.UUID)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	if record.Status == records.StatusRecording || (record.Status == records.StatusStarting && record.RecorderToken != nil) {
		return fmt.Errorf("recorder interrupted; completed chunks retained for recovery")
	}
	if record.Status == records.StatusStarting {
		if err := s.capture(ctx, record, token, dir); err != nil {
			return err
		}
	}
	current, err := s.o.Repository.FindByUUID(ctx, record.UUID)
	if err != nil {
		return err
	}
	if current.Status != records.StatusStopping && current.Status != records.StatusFinalizing && current.Status != records.StatusUploading {
		return fmt.Errorf("recording input ended without a stop transition")
	}
	if err := s.o.Repository.TransitionComposite(ctx, record.UUID, token, records.StatusFinalizing, nil); err != nil {
		return err
	}
	current.Status = records.StatusFinalizing
	s.publish(ctx, current, "recording.processing")
	if err := s.composer.Recover(ctx, dir); err != nil {
		return err
	}
	result, err := s.processor.FinalizeComposite(ctx, dir)
	if err != nil {
		return err
	}
	if err := s.o.Repository.TransitionComposite(ctx, record.UUID, token, records.StatusUploading, nil); err != nil {
		return err
	}
	if s.o.S3 == nil {
		return fmt.Errorf("recording storage is unavailable")
	}
	// Immutable incarnation keys also fence storage: a stale in-flight upload
	// cannot overwrite the winning lease owner's already-published artifact.
	base := filepath.ToSlash(filepath.Join("recordings", record.ConferenceID, record.UUID, "artifacts", token))
	finalUpload, err := s.o.S3.UploadFile(ctx, base+"/final.mp4", result.FinalPath, "video/mp4")
	if err != nil {
		return fmt.Errorf("upload final recording: %w", err)
	}
	previewUpload, err := s.o.S3.UploadFile(ctx, base+"/preview.jpg", result.PreviewPath, "image/jpeg")
	if err != nil {
		return fmt.Errorf("upload recording preview: %w", err)
	}
	final := records.RecordFile{FileType: records.FileTypeFinalMP4, Bucket: finalUpload.Bucket, ObjectKey: finalUpload.ObjectKey, FileName: "final.mp4", MimeType: "video/mp4", SizeBytes: &result.FinalSizeBytes, DurationSec: &result.DurationSec, ChecksumSHA256: &result.FinalChecksum, IsPrimary: true}
	preview := &records.RecordFile{FileType: records.FileTypePreviewJPG, Bucket: previewUpload.Bucket, ObjectKey: previewUpload.ObjectKey, FileName: "preview.jpg", MimeType: "image/jpeg", SizeBytes: &result.PreviewSizeBytes, ChecksumSHA256: &result.PreviewChecksum}
	if err := s.o.Repository.SaveCompositeArtifacts(ctx, record.UUID, token, final, preview, segmentMetadata(result.Segments)); err != nil {
		return err
	}
	current, err = s.o.Repository.FindByUUID(ctx, record.UUID)
	if err == nil {
		s.publish(ctx, current, "recording.ready")
	}
	if s.o.ConferenceLock != nil {
		_ = s.o.ConferenceLock.Release(ctx, record.ConferenceID, record.UUID)
	}
	_ = s.o.Repository.AddEvent(ctx, record.UUID, "recording.ready", "recorder", "info", "Validated composite MP4 and preview uploaded", s.o.WorkerID)
	// Failed/interrupted recordings retain chunks. Once the fenced ready commit
	// is durable, private MinIO becomes authoritative and successful local spool
	// can be removed; KEEP_LOCAL is intended for diagnostics/acceptance tests.
	if !s.o.Config.KeepLocal {
		if err := os.RemoveAll(dir); err != nil {
			s.o.Logger.Printf("composite local cleanup failed record=%s", record.UUID)
		}
	}
	return nil
}

func (s *CompositeService) capture(ctx context.Context, record records.Record, token, dir string) error {
	captureCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	route, err := s.o.Registry.GetOwner(captureCtx, record.ConferenceID)
	if err != nil {
		return fmt.Errorf("conference media worker unavailable")
	}
	endpoint, err := url.Parse(route.Endpoint)
	if err != nil || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.Host == "" || endpoint.User != nil {
		return fmt.Errorf("invalid media worker endpoint")
	}
	seconds := record.SegmentDurationSec
	if seconds < 1 || seconds > 30 {
		seconds = 5
	}
	request := media.EgressRequest{RequestID: uuid.NewString(), ConferenceID: record.ConferenceID, RecordingID: record.UUID, Route: route, SegmentDurationSec: seconds}
	data, _ := json.Marshal(request)
	httpRequest, err := http.NewRequestWithContext(captureCtx, http.MethodPost, strings.TrimRight(route.Endpoint, "/")+"/internal/media/egress", bytes.NewReader(data))
	if err != nil {
		return err
	}
	httpRequest.Header.Set("Authorization", "Bearer "+s.o.InternalSecret)
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("X-Request-ID", request.RequestID)
	response, err := s.client.Do(httpRequest)
	if err != nil {
		return fmt.Errorf("connect recording egress failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return fmt.Errorf("recording egress rejected (%d)", response.StatusCode)
	}
	monitored := make(chan struct{})
	input := &activityReader{reader: response.Body}
	input.lastRead.Store(time.Now().UnixNano())
	go func() {
		defer close(monitored)
		ticker := time.NewTicker(s.o.Config.PollInterval)
		defer ticker.Stop()
		deadline := time.NewTimer(s.o.Config.MaxDuration)
		defer deadline.Stop()
		for {
			select {
			case <-captureCtx.Done():
				return
			case <-deadline.C:
				queryCtx, done := context.WithTimeout(captureCtx, 3*time.Second)
				_ = s.o.Repository.MarkStopping(queryCtx, record.UUID, "duration_limit")
				done()
				cancel()
				return
			case <-ticker.C:
				if time.Since(time.Unix(0, input.lastRead.Load())) > 15*time.Second {
					cancel()
					return
				}
				queryCtx, done := context.WithTimeout(captureCtx, 3*time.Second)
				current, err := s.o.Repository.FindByUUID(queryCtx, record.UUID)
				done()
				if err != nil {
					cancel()
					return
				}
				if current.Status == records.StatusStopping || records.IsTerminalStatus(current.Status) {
					cancel()
					return
				}
			}
		}
	}()
	err = s.composer.Capture(captureCtx, ctx, input, dir, composite.CaptureOptions{SegmentDuration: time.Duration(seconds) * time.Second, MaxBytes: s.o.Config.MaxBytes, OnStarted: func() error {
		if err := s.o.Repository.TransitionComposite(ctx, record.UUID, token, records.StatusRecording, nil); err != nil {
			if errors.Is(err, records.ErrRecordStateChanged) {
				current, readErr := s.o.Repository.FindByUUID(ctx, record.UUID)
				if readErr == nil && current.Status == records.StatusStopping {
					cancel()
					return nil
				}
			}
			return err
		}
		current, err := s.o.Repository.FindByUUID(ctx, record.UUID)
		if err != nil {
			return err
		}
		s.publish(ctx, current, "recording.started")
		return nil
	}})
	cancel()
	<-monitored
	if composite.OnlyEgressEnded(err) {
		current, readErr := s.o.Repository.FindByUUID(ctx, record.UUID)
		if readErr == nil && current.Status == records.StatusStopping {
			return nil
		}
	}
	return err
}

type activityReader struct {
	reader   io.Reader
	lastRead atomic.Int64
}

func (r *activityReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if n > 0 {
		r.lastRead.Store(time.Now().UnixNano())
	}
	return n, err
}

func (r *activityReader) Close() error {
	if closer, ok := r.reader.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

func (s *CompositeService) fail(record records.Record, token string, cause error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// Full FFmpeg diagnostics stay local; clients receive a bounded safe error.
	s.o.Logger.Printf("composite record=%s failed: %v", record.UUID, cause)
	safe := errors.New("Conference recording failed; completed segments were retained")
	if err := s.o.Repository.TransitionComposite(ctx, record.UUID, token, records.StatusFailed, safe); err != nil {
		return
	}
	current, err := s.o.Repository.FindByUUID(ctx, record.UUID)
	if err == nil {
		s.publish(ctx, current, "recording.failed")
	}
	if s.o.ConferenceLock != nil {
		_ = s.o.ConferenceLock.Release(ctx, record.ConferenceID, record.UUID)
	}
	_ = s.o.Repository.AddEvent(ctx, record.UUID, "recording.failed", "recorder", "error", safe.Error(), s.o.WorkerID)
}

func (s *CompositeService) publish(ctx context.Context, record records.Record, event string) {
	if s.o.Publish == nil {
		return
	}
	publishCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := s.o.Publish(publishCtx, record, event); err != nil {
		s.o.Logger.Printf("recording event publish failed record=%s event=%s", record.UUID, event)
	}
}
