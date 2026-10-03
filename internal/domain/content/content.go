// Пакет content описывает расшифровки, проверенные результаты ИИ и поиск по встречам.
// Он не зависит от SDK внешних поставщиков и не участвует в передаче медиапотока.
package content

import (
	"context"
	"encoding/json"
	"io"
	"time"
)

const (
	Queued     = "queued"
	Processing = "processing"
	Ready      = "ready"
	Failed     = "failed"
)

// Transcript хранит состояние одного поколения расшифровки готовой записи.
// Внешний ответ не содержит ключей MinIO, аренды job или секретов провайдера.
type Transcript struct {
	ID           string     `json:"id"`
	ConferenceID string     `json:"conferenceId"`
	RecordingID  string     `json:"recordingId"`
	Status       string     `json:"status"`
	Language     string     `json:"language"`
	Provider     string     `json:"provider"`
	Generation   int64      `json:"generation"`
	ErrorCode    *string    `json:"errorCode,omitempty"`
	ErrorMessage *string    `json:"errorMessage,omitempty"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
	ProcessedAt  *time.Time `json:"processedAt,omitempty"`
}

// Segment связывает текст с интервалом готовой записи; speakerLabel означает
// только метку диаризации, а не подтверждённую личность участника встречи.
type Segment struct {
	ID           string   `json:"id"`
	TranscriptID string   `json:"transcriptId"`
	Ordinal      int      `json:"-"`
	StartMS      int64    `json:"startMs" gorm:"column:start_ms"`
	EndMS        int64    `json:"endMs" gorm:"column:end_ms"`
	SpeakerID    *string  `json:"speakerId"`
	SpeakerLabel *string  `json:"speakerLabel"`
	Text         string   `json:"text"`
	Confidence   *float64 `json:"confidence"`
}

// ActionItem содержит только подтверждённое текстом действие. Исполнитель и
// срок остаются null, если в исходных сегментах нет соответствующего подтверждения.
type ActionItem struct {
	Text             string   `json:"text"`
	Assignee         *string  `json:"assignee"`
	DueDate          *string  `json:"dueDate"`
	SourceSegmentIDs []string `json:"sourceSegmentIds"`
}

// SummaryOutput задаёт единственную принимаемую JSON-схему ответа AI.
type SummaryOutput struct {
	Summary     string       `json:"summary"`
	KeyPoints   []string     `json:"keyPoints"`
	ActionItems []ActionItem `json:"actionItems"`
	Topics      []string     `json:"topics"`
}

// Summary хранит проверенный результат с версией исходной расшифровки и инструкции.
type Summary struct {
	ID                   string `json:"id"`
	ConferenceID         string `json:"conferenceId"`
	TranscriptID         string `json:"transcriptId"`
	TranscriptGeneration int64  `json:"transcriptGeneration"`
	Status               string `json:"status"`
	SummaryOutput        `gorm:"-"`
	StructuredOutput     json.RawMessage `json:"-" gorm:"column:structured_output"`
	Provider             string          `json:"provider"`
	Model                string          `json:"model"`
	PromptVersion        string          `json:"promptVersion"`
	SchemaVersion        string          `json:"schemaVersion"`
	Generation           int64           `json:"generation"`
	ErrorCode            *string         `json:"errorCode,omitempty"`
	ErrorMessage         *string         `json:"errorMessage,omitempty"`
	CreatedAt            time.Time       `json:"createdAt"`
	UpdatedAt            time.Time       `json:"updatedAt"`
	ProcessedAt          *time.Time      `json:"processedAt,omitempty"`
}

// TranscriptState позволяет отличить отсутствие расшифровки от отключённой
// возможности и показывает проверенное сервером право повторного запуска.
type TranscriptState struct {
	Item         *Transcript `json:"item"`
	Enabled      bool        `json:"enabled"`
	CanRetry     bool        `json:"canRetry"`
	ProviderMode string      `json:"providerMode"`
}

// SummaryState возвращает необязательный результат и права без вымышленных состояний обработки.
type SummaryState struct {
	Item          *Summary `json:"item"`
	Enabled       bool     `json:"enabled"`
	CanRegenerate bool     `json:"canRegenerate"`
	ProviderMode  string   `json:"providerMode"`
}

// SegmentPage содержит ограниченную страницу хронологически упорядоченного текста.
type SegmentPage struct {
	Items  []Segment `json:"items"`
	Total  int64     `json:"total"`
	Limit  int       `json:"limit"`
	Offset int       `json:"offset"`
}

// SearchQuery задаёт серверные фильтры; время передаётся как однозначная временная отметка UTC.
type SearchQuery struct {
	Mode, ParticipantID, Membership string
	Query, Source, ConferenceID     string
	From, To                        *time.Time
	Limit, Offset                   int
}

// SearchResult возвращает обычный текст фрагмента и точку перехода к записи, без URL хранилища.
type SearchResult struct {
	SpeakerID       *string `json:"speakerId,omitempty"`
	Speaker         string  `json:"speaker,omitempty"`
	Type            string  `json:"type"`
	ConferenceID    string  `json:"conferenceId"`
	ConferenceTitle string  `json:"conferenceTitle"`
	RecordingID     *string `json:"recordingId"`
	TranscriptID    *string `json:"transcriptId"`
	SegmentID       *string `json:"segmentId"`
	StartMS         *int64  `json:"startMs" gorm:"column:start_ms"`
	Snippet         string  `json:"snippet"`
	Rank            float64 `json:"rank"`
}

// SearchPage содержит ограниченную страницу только разрешённых результатов.
type SearchPage struct {
	EffectiveMode  string         `json:"effectiveMode"`
	FallbackReason string         `json:"fallbackReason,omitempty"`
	Items          []SearchResult `json:"items"`
	Total          int64          `json:"total"`
	Limit          int            `json:"limit"`
	Offset         int            `json:"offset"`
}

// RecordingSource описывает приватный объект из проверенной БД, а не пользовательский URL.
type RecordingSource struct {
	RecordingID, ConferenceID, ObjectKey string
	DurationSec                          int
	SizeBytes                            int64
}

// Audio содержит ограниченный источник mono/16k WAV и обязательную очистку ресурсов.
type Audio struct {
	Reader      io.ReadCloser
	Size        int64
	ContentType string
	Cleanup     func()
}

// TranscriptionRequest передаёт аудио, автоматический выбор языка и стабильный ключ идемпотентности адаптеру.
type TranscriptionRequest struct {
	Audio                                 io.Reader
	Size                                  int64
	ContentType, Language, IdempotencyKey string
	DurationSec                           int
}

// TranscriptionResult возвращает язык и непроверенные сегменты поставщика.
type TranscriptionResult struct {
	Language string    `json:"language"`
	Segments []Segment `json:"segments"`
}

// TranscriptionProvider изолирует внешний STT от domain/usecase.
type TranscriptionProvider interface {
	// Name возвращает постоянное имя адаптера без секретов.
	Name() string
	// Transcribe распознаёт аудио, соблюдая отмену контекста.
	// @args ctx — срок выполнения; request — ограниченное аудио и ключ дедупликации.
	// @return непроверенный результат или классифицированную ошибку.
	Transcribe(context.Context, TranscriptionRequest) (TranscriptionResult, error)
}

// AIRequest отделяет неизменные инструкции от недоверенного JSON содержимого встречи.
// Инструменты и сетевые действия не входят в контракт.
type AIRequest struct {
	PromptVersion, SchemaVersion, Instructions, InputJSON, IdempotencyKey string
	Merge                                                                 bool
}

// AIProvider возвращает JSON, который сценарий проверяет перед сохранением.
type AIProvider interface {
	// Name возвращает постоянное имя адаптера.
	Name() string
	// Model возвращает техническое имя модели без credentials.
	Model() string
	// Summarize обрабатывает только переданные входные данные, не выполняя внешних действий.
	// @args ctx — срок выполнения; request — инструкции и недоверенные данные.
	// @return сырой JSON для строгой серверной проверки или ошибку.
	Summarize(context.Context, AIRequest) (json.RawMessage, error)
}
