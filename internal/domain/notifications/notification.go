package notifications

import (
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
)

// Payload содержит только ссылки на ресурсы и состояние; уведомление не раскрывает чат, файлы и не выдаёт права доступа.
// @params
//   - ConferenceID: идентификатор конференции, ограничивающий область операции.
//   - RecordingID: идентификатор записи конференции.
//   - ScheduledAt: однозначное запланированное время встречи.
//   - AdmissionState: состояние ожидания, допуска, отклонения либо исключения.
type Payload struct {
	ConferenceID   string     `json:"conferenceId"`
	InvitationID   string     `json:"invitationId,omitempty"`
	RecordingID    string     `json:"recordingId,omitempty"`
	MessageID      string     `json:"messageId,omitempty"`
	ScheduledAt    *time.Time `json:"scheduledAt,omitempty"`
	AdmissionState string     `json:"admissionState,omitempty"`
	TranscriptID   string     `json:"transcriptId,omitempty"`
	SummaryID      string     `json:"summaryId,omitempty"`
	Generation     int64      `json:"generation,omitempty"`
}

// Notification сохраняет личное уведомление, ссылочную нагрузку, дедупликацию, прочтение и факт публикации.
// @params
//   - ID: уникальный идентификатор данной сущности.
//   - UserID: идентификатор пользователя, для которого выполняется операция.
//   - Type: значение Type типа string, используемое согласно назначению этой операции.
//   - Version: версия изменения для защиты от устаревших операций.
//   - Payload: типизированная нагрузка события или ссылочные сведения уведомления.
//   - DedupKey: уникальный ключ факта для предотвращения дублирующих уведомлений.
//   - CreatedAt: время создания значения.
//   - ReadAt: момент отметки прочтения; nil означает непрочитанное.
//   - PublishedAt: момент подтверждённой публикации; nil сохраняет необходимость доставки.
type Notification struct {
	ID          string     `json:"id" gorm:"type:uuid;primaryKey"`
	UserID      string     `json:"userId" gorm:"type:uuid"`
	Type        string     `json:"type"`
	Version     int        `json:"version"`
	Payload     Payload    `json:"payload" gorm:"serializer:json;type:jsonb"`
	DedupKey    string     `json:"-"`
	CreatedAt   time.Time  `json:"createdAt"`
	ReadAt      *time.Time `json:"readAt"`
	PublishedAt *time.Time `json:"-"`
}

// TableName возвращает точное имя таблицы для GORM, чтобы модель не зависела от автоматического образования имени.
//
// @return:
//   - результат 1 (string): имя таблицы, используемое ORM.
func (Notification) TableName() string { return "notifications" }

// Page собирает элементы одной страницы и метаданные продолжения списка.
//   - Items: элементы страницы или порции пакетной обработки.
//   - NextCursor: непрозрачная граница следующей страницы; пустое значение завершает список.
//   - UnreadCount: число доступных непрочитанных элементов.
type Page struct {
	Items       []Notification `json:"items"`
	NextCursor  *string        `json:"nextCursor"`
	UnreadCount int64          `json:"unreadCount"`
}

// Cursor задаёт позицию следующей страницы и область, в которой курсор действителен.
//   - UserID: идентификатор пользователя, для которого выполняется операция.
//   - At: однозначное время планируемой операции; nil означает отсутствие значения, если это допускает тип.
//   - ID: уникальный идентификатор данной сущности.
type Cursor struct {
	UserID string    `json:"userId"`
	At     time.Time `json:"at"`
	ID     string    `json:"id"`
}

// DecodeCursor декодирует курсор уведомлений и проверяет привязку к пользователю, время и UUID.
//
// @args
//   - value (string): значение для проверки, нормализации или преобразования.
//   - userID (string): идентификатор пользователя, для которого выполняется операция.
//
// @return:
//   - результат 1 (*Cursor): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func DecodeCursor(value, userID string) (*Cursor, error) {
	if value == "" {
		return nil, nil
	}
	if len(value) > 512 {
		return nil, apperrors.ErrInvalidInput
	}
	b, err := base64.RawURLEncoding.Strict().DecodeString(value)
	var c Cursor
	if err != nil || json.Unmarshal(b, &c) != nil || c.UserID != userID || c.At.IsZero() {
		return nil, apperrors.ErrInvalidInput
	}
	id, err := uuid.Parse(c.ID)
	if err != nil || id == uuid.Nil {
		return nil, apperrors.ErrInvalidInput
	}
	c.ID = id.String()
	return &c, nil
}

// EncodeCursor создаёт курсор следующей страницы по времени и UUID уведомления.
//
// @args
//   - n (Notification): значение n типа Notification, используемое согласно назначению этой операции.
//
// @return:
//   - результат 1 (string): кодированная граница следующей страницы.
func EncodeCursor(n Notification) string {
	b, _ := json.Marshal(Cursor{UserID: n.UserID, At: n.CreatedAt, ID: n.ID})
	return base64.RawURLEncoding.EncodeToString(b)
}
