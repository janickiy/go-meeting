// Пакет captions задаёт независимый от медиа и поставщика контракт живого распознавания.
package captions

import (
	"context"
	"errors"
	"time"
)

var ErrUnavailable = errors.New("live_provider_unavailable")

// SessionConfig описывает одну дорожку одного подключения без биометрической идентификации.
// Audio — знаковый PCM16LE, один канал, 16000 Гц; время событий отсчитывается от первого отсчёта PCM.
type SessionConfig struct {
	SessionID       string `json:"sessionId"`
	TrackInstanceID string `json:"trackInstanceId"`
	Language        string `json:"language"`
	SampleRate      int    `json:"sampleRate"`
	Channels        int    `json:"channels"`
	Format          string `json:"format"`
}

// Event содержит версию реплики поставщика; final запрещает последующую замену partial.
type Event struct {
	UtteranceID string `json:"utteranceId"`
	Sequence    int64  `json:"sequence"`
	Revision    int64  `json:"revision"`
	StartMS     int64  `json:"startMs"`
	EndMS       int64  `json:"endMs"`
	Text        string `json:"text"`
	Language    string `json:"language"`
	Final       bool   `json:"final"`
}

// LiveTranscriptionProvider открывает потоковую сессию; SDK скрывается адаптером.
type LiveTranscriptionProvider interface {
	// StartSession открывает канал распознавания с конечным временем жизни.
	// @args ctx — отмена; config — формат и серверная идентичность дорожки.
	// @return сессия либо безопасная ошибка адаптера.
	StartSession(context.Context, SessionConfig) (Session, error)
}

// Session владеет одним внешним соединением и ограниченным потоком ответов.
type Session interface {
	// WriteAudio отправляет ограниченную порцию PCM только вне SFU.
	// @args ctx — срок выполнения записи; pcm — 16-битные монофонические отсчёты с порядком байтов от младшего к старшему.
	// @return ошибка отправки или отмены.
	WriteAudio(context.Context, []byte) error
	// Events возвращает закрываемый канал partial/final событий.
	// @return поток ограниченного размера, принадлежащий сессии.
	Events() <-chan Event
	// Close прерывает соединение и освобождает ресурсы, повторный вызов безопасен.
	// @return ошибка завершения.
	Close() error
}

// Caption связывает текст с серверной идентичностью и временем конференции.
type Caption struct {
	Cursor          int64  `json:"cursor"`
	Generation      int64  `json:"generation"`
	ID              string `json:"id"`
	ConferenceID    string `json:"conferenceId"`
	SessionID       string `json:"sessionId"`
	ParticipantID   string `json:"participantId"`
	Speaker         string `json:"speaker"`
	TrackInstanceID string `json:"trackInstanceId"`
	Event
	CreatedAt time.Time `json:"createdAt"`
}

// State сообщает всем допущенным участникам состояние передачи аудио на распознавание.
type State struct {
	ConferenceID         string    `json:"conferenceId"`
	SessionID            string    `json:"sessionId"`
	Enabled              bool      `json:"enabled"`
	Available            bool      `json:"available"`
	CanManage            bool      `json:"canManage"`
	Language             string    `json:"language"`
	Status               string    `json:"status"`
	Generation           int64     `json:"generation"`
	Origin               time.Time `json:"origin"`
	CanonicalRecordingID *string   `json:"canonicalRecordingId,omitempty"`
}
