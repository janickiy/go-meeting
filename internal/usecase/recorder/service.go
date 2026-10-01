package recorder

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/records"
	"github.com/janickiy/go-recorder/internal/infrastructure/ffmpeg"
	localstorage "github.com/janickiy/go-recorder/internal/infrastructure/storage/local"
	s3storage "github.com/janickiy/go-recorder/internal/infrastructure/storage/s3"
	webrtcingest "github.com/janickiy/go-recorder/internal/infrastructure/webrtc"
)

type apiRepository interface {
	Create(ctx context.Context, record records.Record) (records.Record, error)
	FindByUUID(ctx context.Context, recordUUID string) (records.Record, error)
	MarkStopping(ctx context.Context, recordUUID string, reason string) error
	MarkFailed(ctx context.Context, recordUUID string, cause error) error
	ListDetails(ctx context.Context, limit int, offset int) ([]records.RecordDetails, error)
	ListSummaryDetailsByConferenceIDs(ctx context.Context, conferenceIDs []string, status string) ([]records.RecordDetails, error)
	FindDetailsByUUID(ctx context.Context, recordUUID string) (records.RecordDetails, error)
}

type workerCommander interface {
	StartRecord(ctx context.Context, recordID string, segmentDurationSec int) error
	StopRecord(ctx context.Context, recordID string, reason string) error
}

type conferenceLocker interface {
	Acquire(ctx context.Context, conferenceID string, recordID string) (bool, error)
	Release(ctx context.Context, conferenceID string, recordID string) error
}

type workerRepository interface {
	ingestFailureRepository
	MarkFinalizing(context.Context, string) error
	MarkUploading(context.Context, string) error
	SaveFinalArtifacts(context.Context, string, records.RecordFile, *records.RecordFile, []records.RecordSegment) error
}

type mediaIngest interface {
	Prepare(string, int) error
	Stop(string) error
	HandleOffer(context.Context, string, records.WebRTCOfferRequest) (records.WebRTCAnswerResponse, error)
}

// Service содержит бизнес-логику API управления записью.
type Service struct {
	repository       apiRepository
	workerCommander  workerCommander
	conferenceLocker conferenceLocker
	s3               *s3storage.Client
}

// WorkerService содержит бизнес-логику recorder-worker.
type WorkerService struct {
	repository       workerRepository
	postProcessor    *ffmpeg.PostProcessor
	ingest           mediaIngest
	s3               *s3storage.Client
	storagePath      string
	workerID         string
	conferenceLocker conferenceReleaser
	commandsMu       sync.Mutex
	commands         map[string]*recordCommandLock
	composite        *CompositeService
}

// SetComposite enables the conference-only recording pipeline. Legacy browser
// ingest retains its existing command and artifact behavior.
func (s *WorkerService) SetComposite(service *CompositeService) { s.composite = service }

// ValidateLegacyRecord prevents the historical unauthenticated worker HTTP
// endpoints from controlling an authenticated conference recording.
func (s *WorkerService) ValidateLegacyRecord(ctx context.Context, id string) error {
	record, err := s.repository.FindByUUID(ctx, id)
	if err != nil {
		return err
	}
	if records.IsComposite(record) {
		return records.ErrRecordStateChanged
	}
	return nil
}

type recordCommandLock struct {
	mu   sync.Mutex
	refs int
}

// NewService создает API use-case.
// Параметры:
// - repository: repository записей.
// - workerCommander: транспорт команд recorder-worker, сейчас RabbitMQ publisher.
// - s3: MinIO/S3-клиент для генерации ссылок на артефакты.
// - conferenceLocker: Redis lock для запрета параллельной записи одной конференции.
// Возвращает: Service.
func NewService(repository apiRepository, workerCommander workerCommander, s3 *s3storage.Client, conferenceLocker conferenceLocker) *Service {
	return &Service{
		repository:       repository,
		workerCommander:  workerCommander,
		conferenceLocker: conferenceLocker,
		s3:               s3,
	}
}

// NewWorkerService создает worker use-case.
// Параметры:
// - repository: repository записей.
// - postProcessor: FFmpeg post-processor.
// - s3: MinIO/S3-клиент.
// - storagePath: локальный storage volume.
// - workerID: идентификатор worker-а.
// Возвращает: WorkerService.
func NewWorkerService(repository workerRepository, postProcessor *ffmpeg.PostProcessor, ingest mediaIngest, s3 *s3storage.Client, storagePath string, workerID string, locker conferenceReleaser) *WorkerService {
	return &WorkerService{
		repository:       repository,
		postProcessor:    postProcessor,
		ingest:           ingest,
		s3:               s3,
		storagePath:      storagePath,
		workerID:         workerID,
		conferenceLocker: locker,
	}
}

// Start создает запись и публикует команду подготовки WebRTC ingest в RabbitMQ.
// Параметры:
// - ctx: контекст HTTP-запроса.
// - request: параметры записи.
// Возвращает: response с recordId или ошибку.
func (s *Service) Start(ctx context.Context, request records.StartRequest) (records.StartResponse, error) {
	if guard, ok := s.repository.(interface {
		LegacyConferenceAllowed(context.Context, string) error
	}); ok {
		if err := guard.LegacyConferenceAllowed(ctx, request.ConferenceID); err != nil {
			return records.StartResponse{}, err
		}
	}
	request = records.NormalizeStartRequest(request)
	if request.SegmentDurationSec <= 0 {
		request.SegmentDurationSec = 5
	}
	videoSettings := records.VideoSettingsForQuality(request.Quality)
	recordID := uuid.NewString()
	locked, err := s.acquireConferenceLock(ctx, request.ConferenceID, recordID)
	if err != nil {
		return records.StartResponse{}, err
	}
	if !locked {
		return records.StartResponse{}, records.ErrConferenceAlreadyRecording
	}
	releaseLock := true
	defer func() {
		if releaseLock {
			_ = s.releaseConferenceLock(context.Background(), request.ConferenceID, recordID)
		}
	}()

	record, err := s.repository.Create(ctx, records.Record{
		UUID:               recordID,
		ConferenceID:       request.ConferenceID,
		RequestedBy:        request.RequestedBy,
		SourceType:         "browser",
		TransportType:      "webrtc",
		Status:             records.StatusStarting,
		QualityMode:        request.QualityMode,
		SegmentDurationSec: request.SegmentDurationSec,
		NeedPreview:        true,
	})
	if err != nil {
		return records.StartResponse{}, err
	}

	if err := s.workerCommander.StartRecord(ctx, record.UUID, request.SegmentDurationSec); err != nil {
		_ = s.repository.MarkFailed(ctx, record.UUID, err)
		return records.StartResponse{}, err
	}
	releaseLock = false

	return records.StartResponse{
		Status:       record.Status,
		Message:      "Record job accepted",
		RecordID:     record.UUID,
		ConferenceID: record.ConferenceID,
		WebRTC: records.WebRTCInfo{
			OfferURL: "/api/v1/records/" + record.UUID + "/webrtc/offer",
			ICEServers: []records.ICEServer{
				{URLs: []string{"stun:stun.l.google.com:19302"}},
			},
			Video: videoSettings,
		},
	}, nil
}

// Stop помечает запись stopping и публикует в RabbitMQ команду завершения.
// Параметры:
// - ctx: контекст HTTP-запроса.
// - request: recordId и reason.
// Возвращает: ошибку БД или публикации RabbitMQ-команды.
func (s *Service) Stop(ctx context.Context, request records.EndRequest) error {
	record, err := s.repository.FindByUUID(ctx, request.RecordID)
	if err != nil {
		return err
	}
	if records.IsComposite(record) {
		return apperrors.ErrNotFound
	}
	if records.IsTerminalStatus(record.Status) || record.Status == records.StatusFinalizing || record.Status == records.StatusUploading {
		return nil
	}
	if record.Status != records.StatusStopping {
		if err := s.repository.MarkStopping(ctx, request.RecordID, request.Reason); err != nil {
			if errors.Is(err, records.ErrRecordStateChanged) {
				return nil
			}
			return err
		}
	}
	// A retry in stopping must republish after a previous publish failure.
	// Only the worker knows when media has actually stopped and can release the lock.
	return s.workerCommander.StopRecord(ctx, request.RecordID, request.Reason)
}

// List возвращает список записей с файлами из MinIO и метаданными сегментов.
// Параметры:
// - ctx: контекст HTTP-запроса.
// - limit: количество.
// - offset: смещение.
// Возвращает: список карточек записей или ошибку БД/MinIO.
func (s *Service) List(ctx context.Context, limit int, offset int) ([]records.RecordCard, error) {
	details, err := s.repository.ListDetails(ctx, limit, offset)
	if err != nil {
		return nil, err
	}
	result := make([]records.RecordCard, 0, len(details))
	for _, item := range details {
		if records.IsComposite(item.Record) {
			continue
		}
		card, err := s.recordCard(ctx, item)
		if err != nil {
			return nil, err
		}
		result = append(result, card)
	}

	return result, nil
}

// CountByConference возвращает количество записей и краткие карточки записей для каждой переданной конференции.
// Параметры:
// - ctx: контекст HTTP-запроса.
// - conferenceIDs: список UUID конференций.
// - status: optional фильтр по статусу записи.
// Возвращает: список conferenceId + recordsCount + records[] или ошибку БД/MinIO.
func (s *Service) CountByConference(ctx context.Context, conferenceIDs []string, status string) ([]records.ConferenceRecordSummary, error) {
	details, err := s.repository.ListSummaryDetailsByConferenceIDs(ctx, conferenceIDs, status)
	if err != nil {
		return nil, err
	}

	summaryIndexesByConference := make(map[string]int, len(conferenceIDs))
	result := make([]records.ConferenceRecordSummary, 0, len(conferenceIDs))
	for _, conferenceID := range conferenceIDs {
		if _, exists := summaryIndexesByConference[conferenceID]; exists {
			continue
		}
		summary := records.ConferenceRecordSummary{
			ConferenceID: conferenceID,
			Records:      []records.ConferenceRecordItem{},
		}
		result = append(result, summary)
		summaryIndexesByConference[conferenceID] = len(result) - 1
	}

	for _, item := range details {
		if records.IsComposite(item.Record) {
			continue
		}
		index, ok := summaryIndexesByConference[item.Record.ConferenceID]
		if !ok {
			continue
		}
		card, err := s.conferenceRecordItem(ctx, item)
		if err != nil {
			return nil, err
		}
		summary := &result[index]
		summary.Records = append(summary.Records, card)
		summary.RecordsCount = int64(len(summary.Records))
	}

	return result, nil
}

// Read возвращает карточку записи по UUID.
// Параметры:
// - ctx: контекст HTTP-запроса.
// - uuid: UUID записи.
// Возвращает: карточку записи или ошибку БД/MinIO.
func (s *Service) Read(ctx context.Context, uuid string) (records.RecordCard, error) {
	details, err := s.repository.FindDetailsByUUID(ctx, uuid)
	if err != nil {
		return records.RecordCard{}, err
	}
	if records.IsComposite(details.Record) {
		return records.RecordCard{}, apperrors.ErrNotFound
	}

	return s.recordCard(ctx, details)
}

// acquireConferenceLock ставит lock на активную запись конференции.
// Параметры:
// - ctx: контекст операции.
// - conferenceID: UUID конференции.
// - recordID: UUID создаваемой записи.
// Возвращает: true, если lock получен.
func (s *Service) acquireConferenceLock(ctx context.Context, conferenceID string, recordID string) (bool, error) {
	if s.conferenceLocker == nil {
		return true, nil
	}

	return s.conferenceLocker.Acquire(ctx, conferenceID, recordID)
}

// releaseConferenceLock снимает lock активной записи конференции.
// Параметры:
// - ctx: контекст операции.
// - conferenceID: UUID конференции.
// - recordID: UUID записи-владельца lock-а.
// Возвращает: ошибку Redis.
func (s *Service) releaseConferenceLock(ctx context.Context, conferenceID string, recordID string) error {
	if s.conferenceLocker == nil {
		return nil
	}

	return s.conferenceLocker.Release(ctx, conferenceID, recordID)
}

// HandleCommand исполняет внутреннюю команду worker-а.
// Параметры:
// - ctx: контекст worker-а.
// - command: record.start или record.stop.
// Возвращает: ошибку обработки команды.
func (s *WorkerService) HandleCommand(ctx context.Context, command records.Command) error {
	// Commands can arrive directly at the worker as well as through RabbitMQ.
	// Validate before using recordId in any filesystem path.
	id, err := uuid.Parse(command.RecordID)
	if err != nil {
		return fmt.Errorf("recordId must be valid UUID: %w", err)
	}
	command.RecordID = id.String()
	if s.composite != nil {
		record, err := s.repository.FindByUUID(ctx, command.RecordID)
		if err != nil {
			return err
		}
		if records.IsComposite(record) {
			return s.composite.HandleCommand(ctx, command, record)
		}
	}
	unlock := s.lockCommand(command.RecordID)
	defer unlock()
	var commandErr error
	switch command.Type {
	case "record.start":
		commandErr = s.handleStart(ctx, command)
	case "record.stop":
		commandErr = s.handleStop(ctx, command)
	default:
		return fmt.Errorf("unknown command type %q", command.Type)
	}
	if errors.Is(commandErr, records.ErrRecordStateChanged) {
		return nil // A newer lifecycle transition has already superseded this command.
	}
	return commandErr
}

// RabbitMQ deliveries and legacy HTTP commands can overlap. Serialize commands
// for the same record only, and discard locks once all callers have completed.
func (s *WorkerService) lockCommand(recordID string) func() {
	s.commandsMu.Lock()
	if s.commands == nil {
		s.commands = make(map[string]*recordCommandLock)
	}
	lock := s.commands[recordID]
	if lock == nil {
		lock = &recordCommandLock{}
		s.commands[recordID] = lock
	}
	lock.refs++
	s.commandsMu.Unlock()
	lock.mu.Lock()
	return func() {
		lock.mu.Unlock()
		s.commandsMu.Lock()
		lock.refs--
		if lock.refs == 0 {
			delete(s.commands, recordID)
		}
		s.commandsMu.Unlock()
	}
}

func (s *WorkerService) handleStart(ctx context.Context, command records.Command) error {
	record, err := s.repository.FindByUUID(ctx, command.RecordID)
	if err != nil {
		return err
	}
	if record.Status != records.StatusStarting {
		if records.IsTerminalStatus(record.Status) {
			return releaseRecordLock(ctx, s.conferenceLocker, record)
		}
		return nil
	}
	recordDir := s.recordDir(command.RecordID)
	if err := os.MkdirAll(recordDir, 0o755); err != nil {
		return s.failWorkerRecord(ctx, command.RecordID, "record.worker.prepare.failed", err)
	}
	if s.ingest != nil {
		if err := s.ingest.Prepare(command.RecordID, command.SegmentDurationSec); err != nil {
			return s.failWorkerRecord(ctx, command.RecordID, "record.worker.prepare.failed", err)
		}
	}
	if err := s.repository.AddEvent(ctx, command.RecordID, "record.worker.ready", "worker", "info", "worker prepared WebRTC ingest", s.workerID); err != nil {
		return err
	}

	return nil
}

func (s *WorkerService) failWorkerRecord(ctx context.Context, recordID string, eventType string, cause error) error {
	err := failRecord(ctx, s.repository, s.conferenceLocker, recordID, s.workerID, eventType, cause)
	s.cleanupEmptyLocalStorageAfterFailure(ctx, recordID)
	return err
}

func (s *WorkerService) handleStop(ctx context.Context, command records.Command) error {
	record, err := s.repository.FindByUUID(ctx, command.RecordID)
	if err != nil {
		return err
	}
	if records.IsTerminalStatus(record.Status) {
		// Acknowledge duplicate deliveries without touching artifacts or terminal status.
		return releaseRecordLock(ctx, s.conferenceLocker, record)
	}
	if s.ingest != nil {
		if err := s.ingest.Stop(command.RecordID); err != nil && !errors.Is(err, webrtcingest.ErrNoMedia) {
			return s.failWorkerRecord(ctx, command.RecordID, "record.ingest.stop.failed", err)
		}
	}
	if err := releaseRecordLock(ctx, s.conferenceLocker, record); err != nil {
		return err
	}
	if err := s.repository.MarkFinalizing(ctx, command.RecordID); err != nil {
		return err
	}
	result, err := s.postProcessor.Finalize(ctx, s.recordDir(command.RecordID))
	if err != nil {
		return s.failWorkerRecord(ctx, command.RecordID, "record.finalize.failed", err)
	}

	if err := s.repository.MarkUploading(ctx, command.RecordID); err != nil {
		return err
	}
	finalKey := filepath.ToSlash(filepath.Join("records", command.RecordID, "final.mp4"))
	previewKey := filepath.ToSlash(filepath.Join("records", command.RecordID, "preview.jpg"))
	finalUpload, err := s.s3.UploadFile(ctx, finalKey, result.FinalPath, "video/mp4")
	if err != nil {
		return s.failWorkerRecord(ctx, command.RecordID, "record.upload.failed", err)
	}
	previewUpload, err := s.s3.UploadFile(ctx, previewKey, result.PreviewPath, "image/jpeg")
	if err != nil {
		return s.failWorkerRecord(ctx, command.RecordID, "record.upload.failed", err)
	}
	if err := s.s3.RemovePrefix(ctx, filepath.ToSlash(filepath.Join("records", command.RecordID, "segments"))+"/"); err != nil {
		return s.failWorkerRecord(ctx, command.RecordID, "record.segments.cleanup.failed", err)
	}
	segments := segmentMetadata(result.Segments)

	duration := result.DurationSec
	finalSize := result.FinalSizeBytes
	finalChecksum := result.FinalChecksum
	previewSize := result.PreviewSizeBytes
	previewChecksum := result.PreviewChecksum
	previewFile := &records.RecordFile{
		FileType:       records.FileTypePreviewJPG,
		Bucket:         previewUpload.Bucket,
		ObjectKey:      previewUpload.ObjectKey,
		FileName:       "preview.jpg",
		MimeType:       "image/jpeg",
		SizeBytes:      &previewSize,
		ChecksumSHA256: &previewChecksum,
		IsPrimary:      false,
		IsPublic:       false,
	}
	finalFile := records.RecordFile{
		FileType:       records.FileTypeFinalMP4,
		Bucket:         finalUpload.Bucket,
		ObjectKey:      finalUpload.ObjectKey,
		FileName:       "final.mp4",
		MimeType:       "video/mp4",
		SizeBytes:      &finalSize,
		DurationSec:    &duration,
		ChecksumSHA256: &finalChecksum,
		IsPrimary:      true,
		IsPublic:       false,
	}
	if err := s.repository.SaveFinalArtifacts(ctx, command.RecordID, finalFile, previewFile, segments); err != nil {
		return err
	}
	_ = s.repository.AddEvent(ctx, command.RecordID, "record.artifacts.uploaded", "worker", "info", "final.mp4 and preview.jpg uploaded to MinIO", s.workerID)
	if err := s.cleanupLocalStorage(command.RecordID); err != nil {
		_ = s.repository.AddEvent(ctx, command.RecordID, "record.storage.cleanup_failed", "worker", "warning", err.Error(), s.workerID)
		return nil
	}
	_ = s.repository.AddEvent(ctx, command.RecordID, "record.storage.cleaned", "worker", "info", "local storage cleaned after successful MinIO upload", s.workerID)

	return nil
}

func (s *WorkerService) recordDir(recordID string) string {
	return filepath.Join(s.storagePath, "records", recordID)
}

func (s *Service) recordCard(ctx context.Context, details records.RecordDetails) (records.RecordCard, error) {
	card := records.RecordCard{
		Record:   details.Record,
		Files:    make([]records.RecordFileView, 0, len(details.Files)),
		Segments: make([]records.RecordSegmentView, 0, len(details.Segments)),
		Events:   details.Events,
	}
	for _, file := range details.Files {
		view := records.RecordFileView{RecordFile: file}
		url, err := s.presignedURL(ctx, file.ObjectKey)
		if err != nil {
			return records.RecordCard{}, err
		}
		view.URL = url
		card.Files = append(card.Files, view)
	}

	for _, segment := range details.Segments {
		view := records.RecordSegmentView{RecordSegment: segment}
		if segment.ObjectKey != nil {
			if details.Record.StorageBucket != nil {
				view.Bucket = *details.Record.StorageBucket
			} else if s.s3 != nil {
				view.Bucket = s.s3.Bucket()
			}
			url, err := s.presignedURL(ctx, *segment.ObjectKey)
			if err != nil {
				return records.RecordCard{}, err
			}
			view.URL = url
		}
		card.Segments = append(card.Segments, view)
	}

	return card, nil
}

// conferenceRecordItem собирает краткую карточку записи для endpoint-а count-by-conference.
// Параметры:
// - details: запись с итоговым файлом и превью, без событий и сегментов.
// Возвращает: recordId, ссылки на final/preview и временные поля записи.
func (s *Service) conferenceRecordItem(ctx context.Context, details records.RecordDetails) (records.ConferenceRecordItem, error) {
	record := details.Record
	result := records.ConferenceRecordItem{
		RecordID:    record.UUID,
		Status:      record.Status,
		DurationSec: record.DurationSec,
		StartedAt:   record.StartedAt,
		StoppedAt:   record.StoppedAt,
		EndedAt:     record.EndedAt,
		CreatedAt:   record.CreatedAt,
	}
	for _, file := range details.Files {
		if file.FileType != records.FileTypeFinalMP4 && file.FileType != records.FileTypePreviewJPG {
			continue
		}
		url, err := s.presignedURL(ctx, file.ObjectKey)
		if err != nil {
			return records.ConferenceRecordItem{}, err
		}
		switch file.FileType {
		case records.FileTypeFinalMP4:
			result.FinalURL = url
		case records.FileTypePreviewJPG:
			result.PreviewURL = url
		}
	}

	return result, nil
}

func (s *Service) presignedURL(ctx context.Context, objectKey string) (string, error) {
	if s.s3 == nil || objectKey == "" {
		return "", nil
	}

	return s.s3.PresignedGetURL(ctx, objectKey, 24*time.Hour)
}

func segmentMetadata(segments []ffmpeg.Segment) []records.RecordSegment {
	result := make([]records.RecordSegment, 0, len(segments))
	for _, segment := range segments {
		fileName := segment.FileName
		sizeBytes := segment.SizeBytes
		checksum := segment.ChecksumSHA256
		result = append(result, records.RecordSegment{
			SeqNo:          segment.SeqNo,
			Status:         "closed",
			FileName:       &fileName,
			SizeBytes:      &sizeBytes,
			ChecksumSHA256: &checksum,
		})
	}

	return result
}

func (s *WorkerService) cleanupLocalStorage(recordID string) error {
	if err := os.RemoveAll(s.recordDir(recordID)); err != nil {
		return fmt.Errorf("cleanup record storage: %w", err)
	}
	if err := os.RemoveAll(filepath.Join(s.storagePath, "tmp", recordID)); err != nil {
		return fmt.Errorf("cleanup tmp storage: %w", err)
	}

	return nil
}

func (s *WorkerService) cleanupEmptyLocalStorage(recordID string) error {
	return localstorage.RemoveEmptyTrees(
		s.recordDir(recordID),
		filepath.Join(s.storagePath, "tmp", recordID),
	)
}

func (s *WorkerService) cleanupEmptyLocalStorageAfterFailure(ctx context.Context, recordID string) {
	if err := s.cleanupEmptyLocalStorage(recordID); err != nil {
		_ = s.repository.AddEvent(ctx, recordID, "record.storage.empty_cleanup_failed", "worker", "warning", err.Error(), s.workerID)
	}
}

// HandleOffer передает browser SDP offer в WebRTC ingest manager.
// Параметры:
// - ctx: HTTP context worker-а.
// - recordID: UUID записи.
// - request: SDP offer.
// Возвращает: SDP answer или ошибку signaling.
func (s *WorkerService) HandleOffer(ctx context.Context, recordID string, request records.WebRTCOfferRequest) (records.WebRTCAnswerResponse, error) {
	record, err := s.repository.FindByUUID(ctx, recordID)
	if err != nil {
		return records.WebRTCAnswerResponse{}, err
	}
	if records.IsComposite(record) {
		return records.WebRTCAnswerResponse{}, records.ErrRecordStateChanged
	}
	if s.ingest == nil {
		return records.WebRTCAnswerResponse{}, fmt.Errorf("webrtc ingest is not configured")
	}

	return s.ingest.HandleOffer(ctx, recordID, request)
}
