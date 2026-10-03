// Пакет search описывает векторные представления без зависимости от конкретной модели или SDK.
package search

import "context"

// EmbeddingRequest привязывает набор ограниченных текстов к единому пространству векторов.
type EmbeddingRequest struct {
	Model          string   `json:"model"`
	Version        string   `json:"version"`
	Dimensions     int      `json:"dimensions"`
	Inputs         []string `json:"inputs"`
	IdempotencyKey string   `json:"-"`
}

// EmbeddingProvider изолирует внешний сервис от хранения, прав и ранжирования.
type EmbeddingProvider interface {
	// Embed возвращает векторы в том же порядке, что Inputs; размерность проверяет сценарий.
	// @args ctx — срок выполнения; request — ограниченная порция и версия модели.
	// @return массив векторов либо классифицированная ошибка.
	Embed(context.Context, EmbeddingRequest) ([][]float32, error)
}

// Chunk связывает ограниченный текст с поколением расшифровки, источником и точкой перехода.
type Chunk struct {
	ID, ConferenceID, RecordingID, TranscriptID, SegmentID string
	Generation                                             int64
	Ordinal                                                int
	StartMS, EndMS                                         int64
	SpeakerID                                              *string
	Speaker, Text, ContentHash, ModelKey                   string
	Vector                                                 []float32
}
