package records

import "time"

// ConferenceStartRequest передаёт настройки общей записи защищённой конференции.
//   - SegmentDurationSec: плановая длительность сегмента записи в секундах.
type ConferenceStartRequest struct {
	SegmentDurationSec int `json:"segmentDurationSec,omitempty"`
}

// OutboxCommand сохраняет команду записи для надёжной доставки из PostgreSQL в RabbitMQ с арендами повторных попыток.
//   - ID: уникальный идентификатор данной сущности.
//   - RecordID: внешний UUID задачи записи.
//   - CommandType: значение CommandType типа string, используемое согласно назначению этой операции.
//   - Reason: причина завершения, отказа или изменения состояния.
//   - ClaimToken: значение ClaimToken типа *string, используемое согласно назначению этой операции.
//   - ClaimedUntil: временная отметка ClaimedUntil; указатель допускает отсутствие значения.
//   - PublishedAt: момент подтверждённой публикации; nil сохраняет необходимость доставки.
//   - CreatedAt: время создания значения.
type OutboxCommand struct {
	ID           int64
	RecordID     string
	CommandType  string
	Reason       string
	ClaimToken   *string
	ClaimedUntil *time.Time
	PublishedAt  *time.Time
	CreatedAt    time.Time
}

// TableName возвращает точное имя таблицы для GORM, чтобы модель не зависела от автоматического образования имени.
//
// @return:
//   - результат 1 (string): имя таблицы, используемое ORM.
func (OutboxCommand) TableName() string { return "recording_outbox" }

// IsComposite определяет, относится ли задача к общей записи конференции через SFU.
//
// @parameters:
//   - record (Record): задача записи с её сохранённым состоянием.
//
// @return:
//   - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния.
func IsComposite(record Record) bool {
	return record.Mode == ModeComposite || record.SourceType == "conference" || record.PlatformConferenceID != nil
}

// PublicStatus переводит внутреннее состояние записи в состояние, используемое внешним API.
//
// @parameters:
//   - status (string): состояние ресурса, ответа или фильтра выборки.
//
// @return:
//   - результат 1 (string): значение, подготовленное операцией для вызывающей стороны.
func PublicStatus(status string) string {
	if status == StatusFinalizing || status == StatusUploading {
		return "processing"
	}
	return status
}
