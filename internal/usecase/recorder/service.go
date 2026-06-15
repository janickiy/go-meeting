package recorder

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"git.svc-dev.net/board/go-recorder/internal/domain/records"
	"git.svc-dev.net/board/go-recorder/internal/infrastructure/ffmpeg"
	postgresrepo "git.svc-dev.net/board/go-recorder/internal/infrastructure/postgres"
	localstorage "git.svc-dev.net/board/go-recorder/internal/infrastructure/storage/local"
	s3storage "git.svc-dev.net/board/go-recorder/internal/infrastructure/storage/s3"
	webrtcingest "git.svc-dev.net/board/go-recorder/internal/infrastructure/webrtc"
	"github.com/google/uuid"
)

type apiRepository interface {
	Create(ctx context.Context, record records.Record) (records.Record, error)
	FindByUUID(ctx context.Context, recordUUID string) (records.Record, error)
	MarkStopping(ctx context.Context, recordUUID string, reason string) error
	MarkFailed(ctx context.Context, recordUUID string, cause error) error
	ListDetails(ctx context.Context, limit int, offset int) ([]records.RecordDetails, error)
	ListDetailsByConferenceIDs(ctx context.Context, conferenceIDs []string, status string) ([]records.RecordDetails, error)
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

// Service содержит бизнес-логику API управления записью.
type Service struct {
	repository       apiRepository
	workerCommander  workerCommander
	conferenceLocker conferenceLocker
	s3               *s3storage.Client
}

// WorkerService содержит бизнес-логику recorder-worker.
type WorkerService struct {
	repository    *postgresrepo.RecordRepository
	postProcessor *ffmpeg.PostProcessor
	ingest        *webrtcingest.Manager
	s3            *s3storage.Client
	storagePath   string
	workerID      string
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
func NewWorkerService(repository *postgresrepo.RecordRepository, postProcessor *ffmpeg.PostProcessor, ingest *webrtcingest.Manager, s3 *s3storage.Client, storagePath string, workerID string) *WorkerService {
	return &WorkerService{
		repository:    repository,
		postProcessor: postProcessor,
		ingest:        ingest,
		s3:            s3,
		storagePath:   storagePath,
		workerID:      workerID,
	}
}

// Start создает запись и публикует команду подготовки WebRTC ingest в RabbitMQ.
// Параметры:
// - ctx: контекст HTTP-запроса.
// - request: параметры записи.
// Возвращает: response с recordId или ошибку.
func (s *Service) Start(ctx context.Context, request records.StartRequest) (records.StartResponse, error) {
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
	if err := s.repository.MarkStopping(ctx, request.RecordID, request.Reason); err != nil {
		return err
	}

	if err := s.workerCommander.StopRecord(ctx, request.RecordID, request.Reason); err != nil {
		return err
	}

	return s.releaseConferenceLock(ctx, record.ConferenceID, request.RecordID)
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
	details, err := s.repository.ListDetailsByConferenceIDs(ctx, conferenceIDs, status)
	if err != nil {
		return nil, err
	}

	summaryIndexesByConference := make(map[string]int, len(conferenceIDs))
	result := make([]records.ConferenceRecordSummary, 0, len(conferenceIDs))
	for _, conferenceID := range conferenceIDs {
		summary := records.ConferenceRecordSummary{
			ConferenceID: conferenceID,
			Records:      []records.ConferenceRecordItem{},
		}
		result = append(result, summary)
		summaryIndexesByConference[conferenceID] = len(result) - 1
	}

	for _, item := range details {
		index, ok := summaryIndexesByConference[item.Record.ConferenceID]
		if !ok {
			continue
		}
		card, err := s.recordCard(ctx, item)
		if err != nil {
			return nil, err
		}
		summary := &result[index]
		summary.Records = append(summary.Records, conferenceRecordItem(card))
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
	switch command.Type {
	case "record.start":
		return s.handleStart(ctx, command)
	case "record.stop":
		return s.handleStop(ctx, command)
	default:
		return fmt.Errorf("unknown command type %q", command.Type)
	}
}

func (s *WorkerService) handleStart(ctx context.Context, command records.Command) error {
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
	if markErr := s.repository.MarkFailed(ctx, recordID, cause); markErr != nil {
		return fmt.Errorf("%w; mark record failed: %v", cause, markErr)
	}
	if eventErr := s.repository.AddEvent(ctx, recordID, eventType, "worker", "error", cause.Error(), s.workerID); eventErr != nil {
		return fmt.Errorf("%w; add failure event: %v", cause, eventErr)
	}

	return nil
}

func (s *WorkerService) handleStop(ctx context.Context, command records.Command) error {
	if s.ingest != nil {
		if err := s.ingest.Stop(command.RecordID); err != nil && !errors.Is(err, webrtcingest.ErrNoMedia) {
			_ = s.repository.MarkFailed(ctx, command.RecordID, err)
			_ = s.repository.AddEvent(ctx, command.RecordID, "record.ingest.stop.failed", "worker", "error", err.Error(), s.workerID)
			s.cleanupEmptyLocalStorageAfterFailure(ctx, command.RecordID)
			return nil
		}
	}
	if err := s.repository.MarkFinalizing(ctx, command.RecordID); err != nil {
		return err
	}
	result, err := s.postProcessor.Finalize(ctx, s.recordDir(command.RecordID))
	if err != nil {
		_ = s.repository.MarkFailed(ctx, command.RecordID, err)
		_ = s.repository.AddEvent(ctx, command.RecordID, "record.finalize.failed", "worker", "error", err.Error(), s.workerID)
		s.cleanupEmptyLocalStorageAfterFailure(ctx, command.RecordID)
		return nil
	}

	if err := s.repository.MarkUploading(ctx, command.RecordID); err != nil {
		return err
	}
	finalKey := filepath.ToSlash(filepath.Join("records", command.RecordID, "final.mp4"))
	previewKey := filepath.ToSlash(filepath.Join("records", command.RecordID, "preview.jpg"))
	finalUpload, err := s.s3.UploadFile(ctx, finalKey, result.FinalPath, "video/mp4")
	if err != nil {
		_ = s.repository.MarkFailed(ctx, command.RecordID, err)
		s.cleanupEmptyLocalStorageAfterFailure(ctx, command.RecordID)
		return nil
	}
	previewUpload, err := s.s3.UploadFile(ctx, previewKey, result.PreviewPath, "image/jpeg")
	if err != nil {
		_ = s.repository.MarkFailed(ctx, command.RecordID, err)
		s.cleanupEmptyLocalStorageAfterFailure(ctx, command.RecordID)
		return nil
	}
	if err := s.s3.RemovePrefix(ctx, filepath.ToSlash(filepath.Join("records", command.RecordID, "segments"))+"/"); err != nil {
		_ = s.repository.MarkFailed(ctx, command.RecordID, err)
		_ = s.repository.AddEvent(ctx, command.RecordID, "record.segments.cleanup.failed", "worker", "error", err.Error(), s.workerID)
		s.cleanupEmptyLocalStorageAfterFailure(ctx, command.RecordID)
		return nil
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
// - card: полная карточка записи с файлами и presigned URL.
// Возвращает: recordId, ссылки на final/preview и временные поля записи.
func conferenceRecordItem(card records.RecordCard) records.ConferenceRecordItem {
	result := records.ConferenceRecordItem{
		RecordID:    card.UUID,
		Status:      card.Status,
		DurationSec: card.DurationSec,
		StartedAt:   card.StartedAt,
		StoppedAt:   card.StoppedAt,
		EndedAt:     card.EndedAt,
		CreatedAt:   card.CreatedAt,
	}
	for _, file := range card.Files {
		switch file.FileType {
		case records.FileTypeFinalMP4:
			result.FinalURL = file.URL
		case records.FileTypePreviewJPG:
			result.PreviewURL = file.URL
		}
	}

	return result
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
	if s.ingest == nil {
		return records.WebRTCAnswerResponse{}, fmt.Errorf("webrtc ingest is not configured")
	}

	return s.ingest.HandleOffer(ctx, recordID, request)
}

// Now возвращает текущее UTC-время; оставлено для будущих тестов worker-а.
// Параметры: нет.
// Возвращает: time.Now().UTC().
func Now() time.Time {
	return time.Now().UTC()
}
