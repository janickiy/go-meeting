package records

import (
	"time"

	"gorm.io/datatypes"
)

const (
	ModeLegacy           = "legacy"
	ModeComposite        = "composite"
	ModeAudioOnly        = "audio_only"
	ModeIndividualTracks = "individual_tracks"
	ModeScreenFocus      = "screen_focus"
	StatusStarting       = "starting"
	StatusRecording      = "recording"
	StatusStopping       = "stopping"
	StatusFinalizing     = "finalizing"
	StatusUploading      = "uploading"
	StatusReady          = "ready"
	StatusPartialReady   = "partial_ready"
	StatusFailed         = "failed"
	StatusCancelled      = "cancelled"
	StatusDegraded       = "degraded"

	FileTypeFinalMP4      = "final_mp4"
	FileTypeFinalAudio    = "final_audio"
	FileTypeTracksArchive = "tracks_archive"
	FileTypePreviewJPG    = "preview_jpg"
	FileTypeDebugLog      = "debug_log"
)

// IsTerminalStatus проверяет, завершён ли жизненный цикл записи и запрещены ли дальнейшие обычные переходы.
//
// @args
//   - status (string): состояние ресурса, ответа или фильтра выборки.
//
// @return:
//   - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния.
func IsTerminalStatus(status string) bool {
	switch status {
	case StatusReady, StatusPartialReady, StatusFailed, StatusCancelled:
		return true
	default:
		return false
	}
}

// Record сохраняет задачу записи, её параметры, состояние, аренду и сведения об ошибке.
// @params
//   - ID: уникальный идентификатор данной сущности.
//   - UUID: идентификатор связанного ресурса, заданного параметром UUID.
//   - Mode: значение Mode типа string, используемое согласно назначению этой операции.
//   - PlatformConferenceID: идентификатор связанного ресурса, заданного параметром PlatformConferenceID.
//   - RecorderToken: значение RecorderToken типа *string, используемое согласно назначению этой операции.
//   - RecorderLeaseUntil: временная отметка RecorderLeaseUntil; указатель допускает отсутствие значения.
//   - ConferenceID: идентификатор конференции, ограничивающий область операции.
//   - RequestedBy: значение RequestedBy типа *string, используемое согласно назначению этой операции.
//   - SourceType: значение SourceType типа string, используемое согласно назначению этой операции.
//   - TransportType: значение TransportType типа string, используемое согласно назначению этой операции.
//   - Status: состояние ресурса, ответа или фильтра выборки.
//   - QualityMode: значение QualityMode типа string, используемое согласно назначению этой операции.
//   - SegmentDurationSec: плановая длительность сегмента записи в секундах.
//   - NeedPreview: логический признак NeedPreview, управляющий соответствующей веткой обработки.
//   - StorageBucket: значение StorageBucket типа *string, используемое согласно назначению этой операции.
//   - StorageObjectKey: значение StorageObjectKey типа *string, используемое согласно назначению этой операции.
//   - PreviewObjectKey: значение PreviewObjectKey типа *string, используемое согласно назначению этой операции.
//   - DurationSec: длительность в секундах.
//   - SizeBytes: фактический размер объекта в байтах.
//   - WorkerID: идентификатор воркера-владельца операции.
//   - StartedAt: момент начала обработки или записи.
//   - StoppedAt: временная отметка StoppedAt; указатель допускает отсутствие значения.
//   - EndedAt: время завершения записи или физической сессии.
//   - EndedReason: значение EndedReason типа *string, используемое согласно назначению этой операции.
//   - ErrorMessage: безопасная причина отказа для внешнего ответа.
//   - MetadataJSON: значение MetadataJSON типа datatypes.JSON, используемое согласно назначению этой операции.
//   - CreatedAt: время создания значения.
//   - UpdatedAt: время последнего сохранённого изменения.
type Record struct {
	ID                   int64          `gorm:"primaryKey" json:"id"`
	UUID                 string         `gorm:"column:uuid;type:uuid;default:gen_random_uuid()" json:"uuid"`
	Mode                 string         `gorm:"column:mode;default:legacy" json:"mode"`
	PlatformConferenceID *string        `gorm:"column:platform_conference_id;type:uuid" json:"-"`
	RecorderToken        *string        `gorm:"column:recorder_token;type:uuid" json:"-"`
	RecorderLeaseUntil   *time.Time     `gorm:"column:recorder_lease_until" json:"-"`
	ConferenceID         string         `gorm:"column:conference_id;type:uuid" json:"conferenceId"`
	RequestedBy          *string        `gorm:"column:requested_by;type:uuid" json:"requestedBy,omitempty"`
	SourceType           string         `gorm:"column:source_type" json:"sourceType"`
	TransportType        string         `gorm:"column:transport_type" json:"transportType"`
	Status               string         `gorm:"column:status" json:"status"`
	QualityMode          string         `gorm:"column:quality_mode" json:"qualityMode"`
	SegmentDurationSec   int            `gorm:"column:segment_duration_sec" json:"segmentDurationSec"`
	NeedPreview          bool           `gorm:"column:need_preview" json:"needPreview"`
	StorageBucket        *string        `gorm:"column:storage_bucket" json:"storageBucket,omitempty"`
	StorageObjectKey     *string        `gorm:"column:storage_object_key" json:"storageObjectKey,omitempty"`
	PreviewObjectKey     *string        `gorm:"column:preview_object_key" json:"previewObjectKey,omitempty"`
	DurationSec          *int           `gorm:"column:duration_sec" json:"durationSec,omitempty"`
	SizeBytes            *int64         `gorm:"column:size_bytes" json:"sizeBytes,omitempty"`
	WorkerID             *string        `gorm:"column:worker_id" json:"workerId,omitempty"`
	StartedAt            *time.Time     `gorm:"column:started_at" json:"startedAt,omitempty"`
	StoppedAt            *time.Time     `gorm:"column:stopped_at" json:"stoppedAt,omitempty"`
	EndedAt              *time.Time     `gorm:"column:ended_at" json:"endedAt,omitempty"`
	EndedReason          *string        `gorm:"column:ended_reason" json:"endedReason,omitempty"`
	ErrorMessage         *string        `gorm:"column:error_message" json:"errorMessage,omitempty"`
	MetadataJSON         datatypes.JSON `gorm:"column:metadata_json" json:"metadataJson,omitempty"`
	CreatedAt            time.Time      `gorm:"column:created_at" json:"createdAt"`
	UpdatedAt            time.Time      `gorm:"column:updated_at" json:"updatedAt"`
}

// TableName возвращает точное имя таблицы для GORM, чтобы модель не зависела от автоматического образования имени.
//
// @return:
//   - результат 1 (string): имя таблицы, используемое ORM.
func (Record) TableName() string { return "record" }

// RecordSegment сохраняет метаданные отдельного сегмента записи.
// @params
//   - ID: уникальный идентификатор данной сущности.
//   - RecordID: внешний UUID задачи записи.
//   - SeqNo: значение SeqNo типа int, используемое согласно назначению этой операции.
//   - Status: состояние ресурса, ответа или фильтра выборки.
//   - LocalPath: значение LocalPath типа *string, используемое согласно назначению этой операции.
//   - ObjectKey: серверный ключ объекта внутри приватного бакета.
//   - FileName: значение FileName типа *string, используемое согласно назначению этой операции.
//   - MimeType: заявленный либо проверенный MIME-тип содержимого.
//   - StartedAt: момент начала обработки или записи.
//   - EndedAt: время завершения записи или физической сессии.
//   - DurationMS: значение DurationMS типа *int, используемое согласно назначению этой операции.
//   - SizeBytes: фактический размер объекта в байтах.
//   - ChecksumSHA256: SHA-256 содержимого артефакта.
//   - CreatedAt: время создания значения.
//   - UpdatedAt: время последнего сохранённого изменения.
type RecordSegment struct {
	ID             int64      `gorm:"primaryKey" json:"id"`
	RecordID       int64      `gorm:"column:record_id" json:"recordId"`
	SeqNo          int        `gorm:"column:seq_no" json:"seqNo"`
	Status         string     `gorm:"column:status" json:"status"`
	LocalPath      *string    `gorm:"column:local_path" json:"localPath,omitempty"`
	ObjectKey      *string    `gorm:"column:object_key" json:"objectKey,omitempty"`
	FileName       *string    `gorm:"column:file_name" json:"fileName,omitempty"`
	MimeType       *string    `gorm:"column:mime_type" json:"mimeType,omitempty"`
	StartedAt      *time.Time `gorm:"column:started_at" json:"startedAt,omitempty"`
	EndedAt        *time.Time `gorm:"column:ended_at" json:"endedAt,omitempty"`
	DurationMS     *int       `gorm:"column:duration_ms" json:"durationMs,omitempty"`
	SizeBytes      *int64     `gorm:"column:size_bytes" json:"sizeBytes,omitempty"`
	ChecksumSHA256 *string    `gorm:"column:checksum_sha256" json:"checksumSha256,omitempty"`
	CreatedAt      time.Time  `gorm:"column:created_at" json:"createdAt"`
	UpdatedAt      time.Time  `gorm:"column:updated_at" json:"updatedAt"`
}

// TableName возвращает точное имя таблицы для GORM, чтобы модель не зависела от автоматического образования имени.
//
// @return:
//   - результат 1 (string): имя таблицы, используемое ORM.
func (RecordSegment) TableName() string { return "record_segment" }

// RecordFile сохраняет метаданные итогового артефакта в приватном объектном хранилище.
// @params
//   - ID: уникальный идентификатор данной сущности.
//   - UUID: идентификатор связанного ресурса, заданного параметром UUID.
//   - RecordID: внешний UUID задачи записи.
//   - FileType: роль файла записи: итоговое видео, превью или другой артефакт.
//   - Bucket: имя бакета объектного хранилища.
//   - ObjectKey: серверный ключ объекта внутри приватного бакета.
//   - FileName: значение FileName типа string, используемое согласно назначению этой операции.
//   - MimeType: заявленный либо проверенный MIME-тип содержимого.
//   - SizeBytes: фактический размер объекта в байтах.
//   - DurationSec: длительность в секундах.
//   - ChecksumSHA256: SHA-256 содержимого артефакта.
//   - IsPrimary: логический признак IsPrimary, управляющий соответствующей веткой обработки.
//   - IsPublic: логический признак IsPublic, управляющий соответствующей веткой обработки.
//   - MetadataJSON: значение MetadataJSON типа datatypes.JSON, используемое согласно назначению этой операции.
//   - CreatedAt: время создания значения.
//   - UpdatedAt: время последнего сохранённого изменения.
type RecordFile struct {
	// Related содержит дополнительные приватные артефакты для того же атомарного ready commit.
	Related        []RecordFile   `gorm:"-" json:"-"`
	ID             int64          `gorm:"primaryKey" json:"id"`
	UUID           string         `gorm:"column:uuid;type:uuid;default:gen_random_uuid()" json:"uuid"`
	RecordID       int64          `gorm:"column:record_id" json:"recordId"`
	FileType       string         `gorm:"column:file_type" json:"fileType"`
	Bucket         string         `gorm:"column:bucket" json:"bucket"`
	ObjectKey      string         `gorm:"column:object_key" json:"objectKey"`
	FileName       string         `gorm:"column:file_name" json:"fileName"`
	MimeType       string         `gorm:"column:mime_type" json:"mimeType"`
	SizeBytes      *int64         `gorm:"column:size_bytes" json:"sizeBytes,omitempty"`
	DurationSec    *int           `gorm:"column:duration_sec" json:"durationSec,omitempty"`
	ChecksumSHA256 *string        `gorm:"column:checksum_sha256" json:"checksumSha256,omitempty"`
	IsPrimary      bool           `gorm:"column:is_primary" json:"isPrimary"`
	IsPublic       bool           `gorm:"column:is_public" json:"isPublic"`
	MetadataJSON   datatypes.JSON `gorm:"column:metadata_json" json:"metadataJson,omitempty"`
	CreatedAt      time.Time      `gorm:"column:created_at" json:"createdAt"`
	UpdatedAt      time.Time      `gorm:"column:updated_at" json:"updatedAt"`
}

// TableName возвращает точное имя таблицы для GORM, чтобы модель не зависела от автоматического образования имени.
//
// @return:
//   - результат 1 (string): имя таблицы, используемое ORM.
func (RecordFile) TableName() string { return "record_file" }

// RecordEvent сохраняет диагностическое событие жизненного цикла записи.
// @params:
//   - ID: уникальный идентификатор данной сущности.
//   - RecordID: внешний UUID задачи записи.
//   - EventType: значение EventType типа string, используемое согласно назначению этой операции.
//   - EventSource: значение EventSource типа string, используемое согласно назначению этой операции.
//   - Severity: значение Severity типа string, используемое согласно назначению этой операции.
//   - Message: сообщение чата или безопасный текст ответа согласно указанному типу.
//   - PayloadJSON: значение PayloadJSON типа datatypes.JSON, используемое согласно назначению этой операции.
//   - WorkerID: идентификатор воркера-владельца операции.
//   - CreatedAt: время создания значения.
type RecordEvent struct {
	ID          int64          `gorm:"primaryKey" json:"id"`
	RecordID    int64          `gorm:"column:record_id" json:"recordId"`
	EventType   string         `gorm:"column:event_type" json:"eventType"`
	EventSource string         `gorm:"column:event_source" json:"eventSource"`
	Severity    string         `gorm:"column:severity" json:"severity"`
	Message     string         `gorm:"column:message" json:"message"`
	PayloadJSON datatypes.JSON `gorm:"column:payload_json" json:"payloadJson,omitempty"`
	WorkerID    string         `gorm:"column:worker_id" json:"workerId"`
	CreatedAt   time.Time      `gorm:"column:created_at" json:"createdAt"`
}

// TableName возвращает точное имя таблицы для GORM, чтобы модель не зависела от автоматического образования имени.
//
// @return:
//   - результат 1 (string): имя таблицы, используемое ORM.
func (RecordEvent) TableName() string { return "record_event" }
