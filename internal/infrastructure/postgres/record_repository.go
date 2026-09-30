package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/janickiy/go-recorder/internal/domain/records"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// RecordRepository инкапсулирует запись/чтение таблиц record*.
type RecordRepository struct {
	db *gorm.DB
}

// NewRecordRepository создает repository.
// Параметры:
// - db: GORM-подключение.
// Возвращает: repository для записей.
func NewRecordRepository(db *gorm.DB) *RecordRepository {
	return &RecordRepository{db: db}
}

// Create создает запись record.
// Параметры:
// - ctx: контекст операции.
// - record: модель с входными полями.
// Возвращает: созданную запись с UUID или ошибку БД.
func (r *RecordRepository) Create(ctx context.Context, record records.Record) (records.Record, error) {
	if err := r.db.WithContext(ctx).Create(&record).Error; err != nil {
		return records.Record{}, err
	}

	return record, nil
}

// FindByUUID возвращает запись по публичному UUID.
// Параметры:
// - ctx: контекст операции.
// - uuid: UUID записи.
// Возвращает: запись или ошибку not found.
func (r *RecordRepository) FindByUUID(ctx context.Context, uuid string) (records.Record, error) {
	var record records.Record
	if err := r.db.WithContext(ctx).Where("uuid = ?", uuid).First(&record).Error; err != nil {
		return records.Record{}, err
	}

	return record, nil
}

// List возвращает последние записи.
// Параметры:
// - ctx: контекст операции.
// - limit: ограничение количества.
// - offset: смещение.
// Возвращает: список записей или ошибку БД.
func (r *RecordRepository) List(ctx context.Context, limit int, offset int) ([]records.Record, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	var result []records.Record
	err := r.db.WithContext(ctx).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&result).
		Error
	return result, err
}

// ListDetails возвращает записи со связанными файлами, сегментами и событиями.
// Параметры:
// - ctx: контекст операции.
// - limit: ограничение количества.
// - offset: смещение.
// Возвращает: список карточек данных или ошибку БД.
func (r *RecordRepository) ListDetails(ctx context.Context, limit int, offset int) ([]records.RecordDetails, error) {
	items, err := r.List(ctx, limit, offset)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return []records.RecordDetails{}, nil
	}

	recordIDs := make([]int64, 0, len(items))
	for _, item := range items {
		recordIDs = append(recordIDs, item.ID)
	}
	files, segments, events, err := r.relatedByRecordIDs(ctx, recordIDs)
	if err != nil {
		return nil, err
	}

	result := make([]records.RecordDetails, 0, len(items))
	for _, item := range items {
		result = append(result, records.RecordDetails{
			Record:   item,
			Files:    files[item.ID],
			Segments: segments[item.ID],
			Events:   events[item.ID],
		})
	}

	return result, nil
}

// ListSummaryDetailsByConferenceIDs returns records with only final and preview files.
// Параметры:
// - ctx: контекст операции.
// - conferenceIDs: список UUID конференций.
// - status: optional фильтр по статусу записи.
// Возвращает: список карточек данных, отсортированный от новых к старым, или ошибку БД.
func (r *RecordRepository) ListSummaryDetailsByConferenceIDs(ctx context.Context, conferenceIDs []string, status string) ([]records.RecordDetails, error) {
	if len(conferenceIDs) == 0 {
		return []records.RecordDetails{}, nil
	}

	query := r.db.WithContext(ctx).
		Where("conference_id IN ?", conferenceIDs)
	if status != "" {
		query = query.Where("status = ?", status)
	}

	var items []records.Record
	if err := query.Order("created_at DESC").Find(&items).Error; err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return []records.RecordDetails{}, nil
	}

	recordIDs := make([]int64, 0, len(items))
	for _, item := range items {
		recordIDs = append(recordIDs, item.ID)
	}
	files, err := r.filesByRecordIDs(ctx, recordIDs, records.FileTypeFinalMP4, records.FileTypePreviewJPG)
	if err != nil {
		return nil, err
	}

	result := make([]records.RecordDetails, 0, len(items))
	for _, item := range items {
		result = append(result, records.RecordDetails{
			Record: item,
			Files:  files[item.ID],
		})
	}

	return result, nil
}

// FindDetailsByUUID возвращает одну запись со связанными файлами, сегментами и событиями.
// Параметры:
// - ctx: контекст операции.
// - uuid: UUID записи.
// Возвращает: карточку данных или ошибку not found.
func (r *RecordRepository) FindDetailsByUUID(ctx context.Context, uuid string) (records.RecordDetails, error) {
	record, err := r.FindByUUID(ctx, uuid)
	if err != nil {
		return records.RecordDetails{}, err
	}
	files, segments, events, err := r.relatedByRecordIDs(ctx, []int64{record.ID})
	if err != nil {
		return records.RecordDetails{}, err
	}

	return records.RecordDetails{
		Record:   record,
		Files:    files[record.ID],
		Segments: segments[record.ID],
		Events:   events[record.ID],
	}, nil
}

// MarkRecording переводит запись в recording.
// Параметры:
// - ctx: контекст операции.
// - uuid: UUID записи.
// - workerID: идентификатор worker-а.
// Возвращает: ошибку БД.
func (r *RecordRepository) MarkRecording(ctx context.Context, uuid string, workerID string) error {
	now := time.Now().UTC()
	return r.transition(ctx, uuid, []string{records.StatusStarting}, map[string]any{
		"status":     records.StatusRecording,
		"worker_id":  workerID,
		"started_at": now,
	})
}

// MarkStopping переводит запись в stopping.
// Параметры:
// - ctx: контекст операции.
// - uuid: UUID записи.
// - reason: причина остановки.
// Возвращает: ошибку БД.
func (r *RecordRepository) MarkStopping(ctx context.Context, uuid string, reason string) error {
	now := time.Now().UTC()
	return r.transition(ctx, uuid, []string{records.StatusStarting, records.StatusRecording, records.StatusDegraded}, map[string]any{
		"status":       records.StatusStopping,
		"stopped_at":   now,
		"ended_reason": reason,
	})
}

// MarkFinalizing переводит запись в finalizing.
// Параметры:
// - ctx: контекст операции.
// - uuid: UUID записи.
// Возвращает: ошибку БД.
func (r *RecordRepository) MarkFinalizing(ctx context.Context, uuid string) error {
	// Uploading is allowed on retry: local artifacts remain until the DB commit.
	return r.transition(ctx, uuid, []string{records.StatusStarting, records.StatusRecording, records.StatusDegraded, records.StatusStopping, records.StatusFinalizing, records.StatusUploading}, map[string]any{"status": records.StatusFinalizing})
}

// MarkUploading переводит запись в uploading.
// Параметры:
// - ctx: контекст операции.
// - uuid: UUID записи.
// Возвращает: ошибку БД.
func (r *RecordRepository) MarkUploading(ctx context.Context, uuid string) error {
	return r.transition(ctx, uuid, []string{records.StatusFinalizing, records.StatusUploading}, map[string]any{"status": records.StatusUploading})
}

// MarkFailed переводит запись в failed и сохраняет текст ошибки.
// Параметры:
// - ctx: контекст операции.
// - uuid: UUID записи.
// - cause: причина ошибки.
// Возвращает: ошибку БД.
func (r *RecordRepository) MarkFailed(ctx context.Context, uuid string, cause error) error {
	message := "unknown error"
	if cause != nil {
		message = cause.Error()
	}
	now := time.Now().UTC()
	return r.transition(ctx, uuid, []string{records.StatusStarting, records.StatusRecording, records.StatusDegraded, records.StatusStopping, records.StatusFinalizing, records.StatusUploading}, map[string]any{
		"status":        records.StatusFailed,
		"error_message": message,
		"ended_at":      now,
	})
}

// SaveFinalArtifacts сохраняет record_file и обновляет record ссылками на MinIO.
// Параметры:
// - ctx: контекст операции.
// - uuid: UUID записи.
// - finalFile: запись итогового MP4.
// - previewFile: запись preview.jpg, может быть nil.
// Возвращает: ошибку транзакции.
func (r *RecordRepository) SaveFinalArtifacts(ctx context.Context, uuid string, finalFile records.RecordFile, previewFile *records.RecordFile, segments []records.RecordSegment) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var record records.Record
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("uuid = ?", uuid).First(&record).Error; err != nil {
			return err
		}
		if record.Status != records.StatusUploading {
			return records.ErrRecordStateChanged
		}

		finalFile.RecordID = record.ID
		if err := upsertRecordFile(tx, &finalFile); err != nil {
			return err
		}
		updates := map[string]any{
			"status":             records.StatusReady,
			"storage_bucket":     finalFile.Bucket,
			"storage_object_key": finalFile.ObjectKey,
			"size_bytes":         finalFile.SizeBytes,
			"duration_sec":       finalFile.DurationSec,
			"ended_at":           time.Now().UTC(),
			"error_message":      nil,
		}

		if previewFile != nil {
			previewFile.RecordID = record.ID
			if err := upsertRecordFile(tx, previewFile); err != nil {
				return err
			}
			updates["preview_object_key"] = previewFile.ObjectKey
		}
		for index := range segments {
			segments[index].RecordID = record.ID
			if err := upsertRecordSegment(tx, &segments[index]); err != nil {
				return err
			}
		}

		return tx.Model(&records.Record{}).Where("id = ?", record.ID).Updates(updates).Error
	})
}

// AddEvent сохраняет событие записи.
// Параметры:
// - ctx: контекст операции.
// - recordUUID: UUID записи.
// - eventType: тип события.
// - source: источник события.
// - severity: info/warning/error.
// - message: текст события.
// - workerID: идентификатор worker-а.
// Возвращает: ошибку БД.
func (r *RecordRepository) AddEvent(ctx context.Context, recordUUID string, eventType string, source string, severity string, message string, workerID string) error {
	record, err := r.FindByUUID(ctx, recordUUID)
	if err != nil {
		return err
	}
	event := records.RecordEvent{
		RecordID:    record.ID,
		EventType:   eventType,
		EventSource: source,
		Severity:    severity,
		Message:     message,
		WorkerID:    workerID,
	}

	return r.db.WithContext(ctx).Create(&event).Error
}

func (r *RecordRepository) transition(ctx context.Context, uuid string, from []string, updates map[string]any) error {
	result := r.db.WithContext(ctx).Model(&records.Record{}).
		Where("uuid = ? AND status IN ?", uuid, from).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return records.ErrRecordStateChanged
	}
	return nil
}

func (r *RecordRepository) filesByRecordIDs(ctx context.Context, recordIDs []int64, fileTypes ...string) (map[int64][]records.RecordFile, error) {
	filesByRecord := make(map[int64][]records.RecordFile, len(recordIDs))
	query := r.db.WithContext(ctx).Where("record_id IN ?", recordIDs)
	if len(fileTypes) > 0 {
		query = query.Where("file_type IN ?", fileTypes)
	}
	var files []records.RecordFile
	if err := query.Order("file_type ASC").Find(&files).Error; err != nil {
		return nil, err
	}
	for _, file := range files {
		filesByRecord[file.RecordID] = append(filesByRecord[file.RecordID], file)
	}
	return filesByRecord, nil
}

func (r *RecordRepository) relatedByRecordIDs(ctx context.Context, recordIDs []int64) (map[int64][]records.RecordFile, map[int64][]records.RecordSegment, map[int64][]records.RecordEvent, error) {
	filesByRecord, err := r.filesByRecordIDs(ctx, recordIDs)
	if err != nil {
		return nil, nil, nil, err
	}
	segmentsByRecord := make(map[int64][]records.RecordSegment, len(recordIDs))
	eventsByRecord := make(map[int64][]records.RecordEvent, len(recordIDs))

	var segments []records.RecordSegment
	if err := r.db.WithContext(ctx).
		Where("record_id IN ?", recordIDs).
		Order("seq_no ASC").
		Find(&segments).
		Error; err != nil {
		return nil, nil, nil, err
	}
	for _, segment := range segments {
		segmentsByRecord[segment.RecordID] = append(segmentsByRecord[segment.RecordID], segment)
	}

	var events []records.RecordEvent
	if err := r.db.WithContext(ctx).
		Where("record_id IN ?", recordIDs).
		Order("created_at ASC").
		Find(&events).
		Error; err != nil {
		return nil, nil, nil, err
	}
	for _, event := range events {
		eventsByRecord[event.RecordID] = append(eventsByRecord[event.RecordID], event)
	}

	return filesByRecord, segmentsByRecord, eventsByRecord, nil
}

func upsertRecordFile(tx *gorm.DB, file *records.RecordFile) error {
	if file.RecordID == 0 {
		return fmt.Errorf("recordID is required for file %s", file.FileType)
	}
	var existing records.RecordFile
	result := tx.Where("record_id = ? AND file_type = ?", file.RecordID, file.FileType).Limit(1).Find(&existing)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected > 0 {
		return tx.Model(&records.RecordFile{}).
			Where("record_id = ? AND file_type = ?", file.RecordID, file.FileType).
			Updates(map[string]any{
				"bucket":          file.Bucket,
				"object_key":      file.ObjectKey,
				"file_name":       file.FileName,
				"mime_type":       file.MimeType,
				"size_bytes":      file.SizeBytes,
				"duration_sec":    file.DurationSec,
				"checksum_sha256": file.ChecksumSHA256,
				"is_primary":      file.IsPrimary,
				"is_public":       file.IsPublic,
				"metadata_json":   file.MetadataJSON,
			}).Error
	}

	return tx.Create(file).Error
}

func upsertRecordSegment(tx *gorm.DB, segment *records.RecordSegment) error {
	if segment.RecordID == 0 {
		return fmt.Errorf("recordID is required for segment %d", segment.SeqNo)
	}
	var existing records.RecordSegment
	result := tx.Where("record_id = ? AND seq_no = ?", segment.RecordID, segment.SeqNo).Limit(1).Find(&existing)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected > 0 {
		return tx.Model(&records.RecordSegment{}).
			Where("record_id = ? AND seq_no = ?", segment.RecordID, segment.SeqNo).
			Updates(map[string]any{
				"status":          segment.Status,
				"local_path":      segment.LocalPath,
				"object_key":      segment.ObjectKey,
				"file_name":       segment.FileName,
				"mime_type":       segment.MimeType,
				"started_at":      segment.StartedAt,
				"ended_at":        segment.EndedAt,
				"duration_ms":     segment.DurationMS,
				"size_bytes":      segment.SizeBytes,
				"checksum_sha256": segment.ChecksumSHA256,
			}).Error
	}

	return tx.Create(segment).Error
}
