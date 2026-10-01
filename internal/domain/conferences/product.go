package conferences

import (
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
)

// IsAdmitted проверяет допуск участника; для старых адаптеров учитывает также ограничения состояния членства.
//
// @return:
//   - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния.
func (p Participant) IsAdmitted() bool {
	return (p.AdmissionState == AdmissionAdmitted || p.AdmissionState == "") && p.Status != Waiting && p.Status != Rejected && p.Status != Kicked
}

// CanParticipate разрешает участие только допущенному участнику со статусом joined.
//
// @return:
//   - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния.
func (p Participant) CanParticipate() bool { return p.IsAdmitted() && p.Status == Joined }

// CanReadHistory разрешает историю допущенному участнику, который находится во встрече или ранее вышел из неё.
//
// @return:
//   - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния.
func (p Participant) CanReadHistory() bool {
	return p.IsAdmitted() && (p.Status == Joined || p.Status == Left)
}

// CanAdmit проверяет, присоединился ли допущенный владелец или соорганизатор, имеющий право решать запросы входа.
//
// @return:
//   - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния.
func (p Participant) CanAdmit() bool {
	return p.CanParticipate() && (p.Role == Owner || p.Role == CoHost)
}

// AdmissionRequest передаёт решение организатора о допуске или отклонении участника.
//   - Decision: действие admit либо reject для ожидающего участника.
type AdmissionRequest struct {
	Decision string `json:"decision"`
}

// Validate проверяет ограничения и согласованность полей текущего значения перед его использованием.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r AdmissionRequest) Validate() error {
	if r.Decision != "admit" && r.Decision != "reject" {
		return apperrors.New(apperrors.ErrInvalidInput, "decision must be admit or reject")
	}
	return nil
}

// ScheduleRequest передаёт однозначное время встречи и необязательную плановую длительность.
// Состав:
//   - ScheduledAt: однозначное запланированное время встречи.
//   - PlannedDurationMin: необязательная длительность в минутах.
type ScheduleRequest struct {
	ScheduledAt        time.Time `json:"scheduledAt"`
	PlannedDurationMin *int      `json:"plannedDurationMin"`
}

// ValidateSchedule проверяет время расписания и плановую длительность относительно переданного текущего времени.
//
// @parameters:
//   - at (*time.Time): однозначное время планируемой операции; nil означает отсутствие значения, если это допускает тип.
//   - duration (*int): плановая длительность или интервал в единицах, заданных типом.
//   - now (time.Time): текущее время для проверки сроков и воспроизводимых тестов.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func ValidateSchedule(at *time.Time, duration *int, now time.Time) error {
	if duration != nil && (*duration < 1 || *duration > 1440) {
		return apperrors.New(apperrors.ErrInvalidInput, "plannedDurationMin must be between 1 and 1440")
	}
	if at == nil {
		if duration != nil {
			return apperrors.New(apperrors.ErrInvalidInput, "planned duration requires scheduledAt")
		}
		return nil
	}
	if at.IsZero() || !at.After(now) || at.Year() > 9999 {
		return apperrors.New(apperrors.ErrInvalidInput, "scheduledAt must be a future RFC3339 instant with an explicit timezone")
	}
	return nil
}

// TimelineQuery задаёт фильтры и курсор серверного списка встреч пользователя.
// Состав:
//   - View: раздел будущих, активных или прошедших встреч.
//   - Scope: область собственных и участвующих встреч пользователя.
//   - Status: состояние ресурса, ответа или фильтра выборки.
//   - From: исходное состояние или нижняя граница диапазона.
//   - To: целевое состояние или верхняя граница диапазона.
//   - Cursor: непрозрачная граница продолжения предыдущей страницы.
//   - Limit: предел количества обрабатываемых элементов.
type TimelineQuery struct {
	View   string
	Scope  string
	Status Status
	From   *time.Time
	To     *time.Time
	Cursor string
	Limit  int
}

// TimelineCursor хранит дату и UUID границы страницы списка встреч.
// Состав:
//   - At: однозначное время планируемой операции; nil означает отсутствие значения, если это допускает тип.
//   - ID: уникальный идентификатор данной сущности.
type TimelineCursor struct {
	At time.Time `json:"at"`
	ID string    `json:"id"`
}

// Validate проверяет ограничения и согласованность полей текущего значения перед его использованием.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (q TimelineQuery) Validate() error {
	if q.View != "upcoming" && q.View != "active" && q.View != "past" {
		return apperrors.New(apperrors.ErrInvalidInput, "view must be upcoming, active or past")
	}
	if q.Scope != "all" && q.Scope != "owned" && q.Scope != "participating" {
		return apperrors.ErrInvalidInput
	}
	if q.Limit < 1 || q.Limit > 100 || (q.From != nil && q.To != nil && q.From.After(*q.To)) {
		return apperrors.ErrInvalidInput
	}
	if q.Status != "" && q.Status != Created && q.Status != Scheduled && q.Status != Active && q.Status != Finished && q.Status != Cancelled {
		return apperrors.ErrInvalidInput
	}
	_, err := DecodeTimelineCursor(q.Cursor)
	return err
}

// DecodeTimelineCursor разбирает курсор списка встреч и проверяет его дату и UUID.
//
// @parameters:
//   - value (string): значение для проверки, нормализации или преобразования.
//
// @return:
//   - результат 1 (*TimelineCursor): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func DecodeTimelineCursor(value string) (*TimelineCursor, error) {
	if value == "" {
		return nil, nil
	}
	if len(value) > 256 {
		return nil, apperrors.ErrInvalidInput
	}
	data, err := base64.RawURLEncoding.Strict().DecodeString(value)
	var c TimelineCursor
	if err != nil || json.Unmarshal(data, &c) != nil || c.At.IsZero() {
		return nil, apperrors.ErrInvalidInput
	}
	id, err := uuid.Parse(c.ID)
	if err != nil || id == uuid.Nil {
		return nil, apperrors.ErrInvalidInput
	}
	c.ID = id.String()
	return &c, nil
}

// EncodeTimelineCursor кодирует дату сортировки и UUID встречи для следующей страницы истории.
//
// @parameters:
//   - c (Conference): контекст HTTP-запроса Gin с параметрами, авторизацией и ответом.
//
// @return:
//   - результат 1 (string): кодированная граница следующей страницы.
func EncodeTimelineCursor(c Conference) string {
	at := c.CreatedAt
	if c.ScheduledAt != nil {
		at = *c.ScheduledAt
	}
	if c.FinishedAt != nil {
		at = *c.FinishedAt
	}
	data, _ := json.Marshal(TimelineCursor{At: at, ID: c.ID})
	return base64.RawURLEncoding.EncodeToString(data)
}

// TimelinePage возвращает страницу отфильтрованных встреч и курсор продолжения.
// Состав:
//   - Items: элементы страницы или порции пакетной обработки.
//   - NextCursor: непрозрачная граница следующей страницы; пустое значение завершает список.
type TimelinePage struct {
	Items      []View  `json:"items"`
	NextCursor *string `json:"nextCursor"`
}

// RecordingSummary обобщает состояния записей без дублирования метаданных файлов в конференции.
// Состав:
//   - Total: значение Total типа int64, используемое согласно назначению этой операции.
//   - Ready: значение Ready типа int64, используемое согласно назначению этой операции.
//   - Processing: значение Processing типа int64, используемое согласно назначению этой операции.
//   - Failed: значение Failed типа int64, используемое согласно назначению этой операции.
type RecordingSummary struct {
	Total      int64 `json:"total"`
	Ready      int64 `json:"ready"`
	Processing int64 `json:"processing"`
	Failed     int64 `json:"failed"`
}

// HistoryOwner задаёт согласованное представление данных «история владелец» для жизненном цикле конференций и правах участников.
// Состав:
//   - ID: уникальный идентификатор данной сущности.
//   - DisplayName: имя пользователя, отображаемое участникам встречи.
type HistoryOwner struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
}

// HistoryView собирает сведения завершённой встречи, видимые членства и сводку записей.
//   - Conference: конференция либо её идентификатор, ограничивающий область операции.
//   - Owner: значение Owner типа HistoryOwner, используемое согласно назначению этой операции.
//   - DurationSec: длительность в секундах.
//   - ParticipantCount: число видимых исторических членств.
//   - Participants: набор значений Participants для последовательной или пакетной обработки.
//   - ParticipantsTruncated: признак ограниченной первой части истории участников.
//   - Recordings: значение Recordings типа RecordingSummary, используемое согласно назначению этой операции.
//   - ChatAvailable: разрешает текущему пользователю читать постоянный чат.
//   - ChatReadOnly: запрещает изменение чата после завершения конференции.
type HistoryView struct {
	Conference            View              `json:"conference"`
	Owner                 HistoryOwner      `json:"owner"`
	DurationSec           *int64            `json:"durationSec"`
	ParticipantCount      int64             `json:"participantCount"`
	Participants          []ParticipantView `json:"participants"`
	ParticipantsTruncated bool              `json:"participantsTruncated"`
	Recordings            RecordingSummary  `json:"recordings"`
	ChatAvailable         bool              `json:"chatAvailable"`
	ChatReadOnly          bool              `json:"chatReadOnly"`
}
