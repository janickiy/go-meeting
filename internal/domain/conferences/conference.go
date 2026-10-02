package conferences

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/janickiy/go-recorder/internal/domain/apperrors"
)

type Status string
type Role string
type ParticipantStatus string
type AdmissionState string

const (
	Created           Status            = "created"
	Scheduled         Status            = "scheduled"
	Active            Status            = "active"
	Finished          Status            = "finished"
	Cancelled         Status            = "cancelled"
	Owner             Role              = "owner"
	CoHost            Role              = "co_host"
	ParticipantRole   Role              = "participant"
	Guest             Role              = "guest"
	Joined            ParticipantStatus = "joined"
	Left              ParticipantStatus = "left"
	Waiting           ParticipantStatus = "waiting"
	Rejected          ParticipantStatus = "rejected"
	Kicked            ParticipantStatus = "kicked"
	AdmissionWaiting  AdmissionState    = "waiting"
	AdmissionAdmitted AdmissionState    = "admitted"
	AdmissionRejected AdmissionState    = "rejected"
	AdmissionKicked   AdmissionState    = "kicked"
)

var ErrInviteCollision = errors.New("invite code collision")

// CanTransition проверяет, допустим ли переход жизненного цикла конференции между двумя состояниями.
//
// @args
//   - from (Status): исходное состояние или нижняя граница диапазона.
//   - to (Status): целевое состояние или верхняя граница диапазона.
//
// @return:
//   - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния.
func CanTransition(from, to Status) bool {
	return ((from == Created || from == Scheduled) && (to == Active || to == Cancelled)) || (from == Active && to == Finished)
}

// Conference сохраняет конференцию, её владельца, жизненный цикл и расписание.
//
//	@params
//	 - ID: уникальный идентификатор данной сущности.
//	 - OwnerID: идентификатор организатора или владельца ресурса.
//	 - Title: отображаемое название встречи.
//	 - InviteCode: криптографически случайный код приглашения, не заменяющий авторизацию.
//	 - Status: состояние ресурса, ответа или фильтра выборки.
//	 - CreatedAt: время создания значения.
//	 - UpdatedAt: время последнего сохранённого изменения.
//	 - StartedAt: момент начала обработки или записи.
//	 - FinishedAt: время завершения конференции.
//	 - WaitingRoomEnabled: требует решения организатора перед входом нового участника в комнату.
//	 - ScheduledAt: однозначное запланированное время встречи.
//	 - PlannedDurationMin: необязательная длительность в минутах.
type Conference struct {
	ID                 string `gorm:"type:uuid;primaryKey"`
	OwnerID            string `gorm:"column:owner_id;type:uuid"`
	Title              string
	InviteCode         string `gorm:"column:invite_code"`
	Status             Status
	CreatedAt          time.Time
	UpdatedAt          time.Time
	StartedAt          *time.Time
	FinishedAt         *time.Time
	WaitingRoomEnabled bool
	ScheduledAt        *time.Time
	PlannedDurationMin *int
}

// TableName возвращает точное имя таблицы для GORM, чтобы модель не зависела от автоматического образования имени.
//
// @return:
//   - результат 1 (string): имя таблицы, используемое ORM.
func (Conference) TableName() string { return "conferences" }

// Participant сохраняет членство пользователя в конференции отдельно от физических соединений, допуск и ограничения медиа.
// @params
//   - ID: уникальный идентификатор данной сущности.
//   - ConferenceID: идентификатор конференции, ограничивающий область операции.
//   - UserID: идентификатор пользователя, для которого выполняется операция.
//   - DisplayName: имя пользователя, отображаемое участникам встречи.
//   - Role: роль участника, определяющая полномочия.
//   - Status: состояние ресурса, ответа или фильтра выборки.
//   - JoinedAt: время присоединения членства.
//   - LeftAt: время последнего выхода участника.
//   - CreatedAt: время создания значения.
//   - UpdatedAt: время последнего сохранённого изменения.
//   - MicrophoneEnabled: сохранённый признак включённого микрофона.
//   - CameraEnabled: сохранённый признак включённой камеры.
//   - ScreenSharing: сохранённый признак демонстрации экрана.
//   - MicrophoneBlocked: серверный запрет микрофона.
//   - CameraBlocked: серверный запрет камеры.
//   - ScreenBlocked: серверный запрет экрана.
//   - MediaPolicyVersion: версия сохранённой политики медиа.
//   - AdmissionState: состояние ожидания, допуска, отклонения либо исключения.
//   - AdmissionDecidedAt: время сохранённого решения о допуске.
//   - AdmissionVersion: монотонная версия решения допуска.
type Participant struct {
	ID                 string  `gorm:"type:uuid;primaryKey"`
	ConferenceID       string  `gorm:"column:conference_id;type:uuid"`
	UserID             *string `gorm:"column:user_id;type:uuid"`
	DisplayName        string
	Role               Role
	Status             ParticipantStatus
	JoinedAt           *time.Time
	LeftAt             *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
	MicrophoneEnabled  bool
	CameraEnabled      bool
	ScreenSharing      bool
	MicrophoneBlocked  bool
	CameraBlocked      bool
	ScreenBlocked      bool
	MediaPolicyVersion int64          `gorm:"default:1"`
	AdmissionState     AdmissionState `gorm:"default:admitted"`
	AdmissionDecidedAt *time.Time
	AdmissionVersion   int64 `gorm:"default:1"`
}

// TableName возвращает точное имя таблицы для GORM, чтобы модель не зависела от автоматического образования имени.
//
// @return:
//   - результат 1 (string): имя таблицы, используемое ORM.
func (Participant) TableName() string { return "conference_participants" }

// View представляет безопасную публичную проекцию доменной модели для API.
// @params:
//   - ID: уникальный идентификатор данной сущности.
//   - OwnerID: идентификатор организатора или владельца ресурса.
//   - Title: отображаемое название встречи.
//   - InviteCode: криптографически случайный код приглашения, не заменяющий авторизацию.
//   - InviteURL: ссылка приглашения на встречу.
//   - Status: состояние ресурса, ответа или фильтра выборки.
//   - CreatedAt: время создания значения.
//   - UpdatedAt: время последнего сохранённого изменения.
//   - StartedAt: момент начала обработки или записи.
//   - FinishedAt: время завершения конференции.
//   - WaitingRoomEnabled: требует решения организатора перед входом нового участника в комнату.
//   - ScheduledAt: однозначное запланированное время встречи.
//   - PlannedDurationMin: необязательная длительность в минутах.
type View struct {
	ID                 string     `json:"id"`
	OwnerID            string     `json:"ownerId"`
	Title              string     `json:"title"`
	InviteCode         string     `json:"inviteCode"`
	InviteURL          string     `json:"inviteUrl"`
	Status             Status     `json:"status"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
	StartedAt          *time.Time `json:"startedAt"`
	FinishedAt         *time.Time `json:"finishedAt"`
	WaitingRoomEnabled bool       `json:"waitingRoomEnabled"`
	ScheduledAt        *time.Time `json:"scheduledAt"`
	PlannedDurationMin *int       `json:"plannedDurationMin"`
}

// View собирает публичное представление модели для ответа API.
//
// @return:
//   - результат 1 (View): значение, подготовленное операцией для вызывающей стороны.
func (c Conference) View() View {
	return View{ID: c.ID, OwnerID: c.OwnerID, Title: c.Title, InviteCode: c.InviteCode,
		InviteURL: "/api/v1/conference-invites/" + c.InviteCode, Status: c.Status,
		CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt, StartedAt: c.StartedAt, FinishedAt: c.FinishedAt,
		WaitingRoomEnabled: c.WaitingRoomEnabled, ScheduledAt: c.ScheduledAt, PlannedDurationMin: c.PlannedDurationMin}
}

// InviteView передаёт ограниченные сведения встречи по приглашению без доступа к защищённым данным комнаты.
// @params
//   - ID: уникальный идентификатор данной сущности.
//   - Title: отображаемое название встречи.
//   - Status: состояние ресурса, ответа или фильтра выборки.
//   - WaitingRoomEnabled: требует решения организатора перед входом нового участника в комнату.
//   - ScheduledAt: однозначное запланированное время встречи.
type InviteView struct {
	ID                 string     `json:"id"`
	Title              string     `json:"title"`
	Status             Status     `json:"status"`
	WaitingRoomEnabled bool       `json:"waitingRoomEnabled,omitempty"`
	ScheduledAt        *time.Time `json:"scheduledAt,omitempty"`
}

// ParticipantView передаёт разрешённые внешнему клиенту сведения о членстве и состоянии участника.
//
//	@params
//	 - ID: уникальный идентификатор данной сущности.
//	 - ConferenceID: идентификатор конференции, ограничивающий область операции.
//	 - UserID: идентификатор пользователя, для которого выполняется операция.
//	 - DisplayName: имя пользователя, отображаемое участникам встречи.
//	 - Role: роль участника, определяющая полномочия.
//	 - Status: состояние ресурса, ответа или фильтра выборки.
//	 - JoinedAt: время присоединения членства.
//	 - LeftAt: время последнего выхода участника.
//	 - CreatedAt: время создания значения.
//	 - UpdatedAt: время последнего сохранённого изменения.
//	 - MicrophoneEnabled: сохранённый признак включённого микрофона.
//	 - CameraEnabled: сохранённый признак включённой камеры.
//	 - ScreenSharing: сохранённый признак демонстрации экрана.
//	 - MicrophoneBlocked: серверный запрет микрофона.
//	 - CameraBlocked: серверный запрет камеры.
//	 - ScreenBlocked: серверный запрет экрана.
//	 - MediaPolicyVersion: версия сохранённой политики медиа.
//	 - AdmissionState: состояние ожидания, допуска, отклонения либо исключения.
//	 - AdmissionDecidedAt: время сохранённого решения о допуске.
//	 - AdmissionVersion: монотонная версия решения допуска.
type ParticipantView struct {
	ID                 string            `json:"id"`
	ConferenceID       string            `json:"conferenceId"`
	UserID             *string           `json:"userId"`
	DisplayName        string            `json:"displayName"`
	Role               Role              `json:"role"`
	Status             ParticipantStatus `json:"status"`
	JoinedAt           *time.Time        `json:"joinedAt"`
	LeftAt             *time.Time        `json:"leftAt"`
	CreatedAt          time.Time         `json:"createdAt"`
	UpdatedAt          time.Time         `json:"updatedAt"`
	MicrophoneEnabled  bool              `json:"microphoneEnabled"`
	CameraEnabled      bool              `json:"cameraEnabled"`
	ScreenSharing      bool              `json:"screenSharing"`
	MicrophoneBlocked  bool              `json:"microphoneBlocked"`
	CameraBlocked      bool              `json:"cameraBlocked"`
	ScreenBlocked      bool              `json:"screenBlocked"`
	MediaPolicyVersion int64             `json:"mediaPolicyVersion"`
	AdmissionState     AdmissionState    `json:"admissionState"`
	AdmissionDecidedAt *time.Time        `json:"admissionDecidedAt"`
	AdmissionVersion   int64             `json:"admissionVersion"`
}

// View собирает публичное представление модели для ответа API.
//
// @return:
//   - результат 1 (ParticipantView): значение, подготовленное операцией для вызывающей стороны.
func (p Participant) View() ParticipantView {
	return ParticipantView(p)
}

// CreateRequest передаёт входные параметры создания конференции, включая расписание и зал ожидания.
//
//	@params
//	 - Title: отображаемое название встречи.
//	 - WaitingRoomEnabled: требует решения организатора перед входом нового участника в комнату.
//	 - ScheduledAt: однозначное запланированное время встречи.
//	 - PlannedDurationMin: необязательная длительность в минутах.
type CreateRequest struct {
	Title              string     `json:"title"`
	WaitingRoomEnabled bool       `json:"waitingRoomEnabled"`
	ScheduledAt        *time.Time `json:"scheduledAt"`
	PlannedDurationMin *int       `json:"plannedDurationMin"`
}

// JoinRequest передаёт сведения запроса присоединения к конференции.
//   - InviteCode: криптографически случайный код приглашения, не заменяющий авторизацию.
type JoinRequest struct {
	InviteCode string `json:"inviteCode"`
}

// NormalizeTitle обрезает лишние пробелы и проверяет допустимую длину названия встречи.
//
// @args
//   - value (string): значение для проверки, нормализации или преобразования.
//
// @return:
//   - результат 1 (string): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func NormalizeTitle(value string) (string, error) {
	value = strings.TrimSpace(value)
	if !utf8.ValidString(value) || value == "" || utf8.RuneCountInString(value) > 200 {
		return "", apperrors.New(apperrors.ErrInvalidInput, "title must contain 1 to 200 characters")
	}
	return value, nil
}
