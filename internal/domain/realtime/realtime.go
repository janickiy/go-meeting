package realtime

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/conferences"
)

// Session описывает историю одного физического WebSocket-соединения; членство участника переживает его закрытие.
// @params
//   - ID: уникальный идентификатор данной сущности.
//   - ConferenceID: идентификатор конференции, ограничивающий область операции.
//   - ParticipantID: идентификатор членства участника внутри конференции.
//   - UserID: идентификатор пользователя, для которого выполняется операция.
//   - ConnectionID: идентификатор физического медиа-соединения.
//   - Status: состояние ресурса, ответа или фильтра выборки.
//   - ConnectedAt: временная отметка ConnectedAt; указатель допускает отсутствие значения.
//   - DisconnectedAt: временная отметка DisconnectedAt; указатель допускает отсутствие значения.
//   - LastSeenAt: временная отметка LastSeenAt; указатель допускает отсутствие значения.
type Session struct {
	ID             string     `json:"id" gorm:"type:uuid;primaryKey"`
	ConferenceID   string     `json:"conferenceId" gorm:"type:uuid"`
	ParticipantID  string     `json:"participantId" gorm:"type:uuid"`
	UserID         string     `json:"userId" gorm:"type:uuid"`
	ConnectionID   string     `json:"connectionId" gorm:"type:uuid"`
	Status         string     `json:"status"`
	ConnectedAt    time.Time  `json:"connectedAt"`
	DisconnectedAt *time.Time `json:"disconnectedAt,omitempty"`
	LastSeenAt     time.Time  `json:"lastSeenAt"`
}

// TableName возвращает точное имя таблицы для GORM, чтобы модель не зависела от автоматического образования имени.
//
// @return:
//   - результат 1 (string): имя таблицы, используемое ORM.
func (Session) TableName() string { return "participant_sessions" }

// Identity связывает подтверждённого пользователя с участником и конференцией.
//   - UserID: идентификатор пользователя, для которого выполняется операция.
//   - ExpiresAt: момент окончания действия сессии, токена или аренды.
type Identity struct {
	UserID        string    `json:"userId"`
	ExpiresAt     time.Time `json:"expiresAt"`
	AuthSessionID string    `json:"authSessionId,omitempty"`
}

// Envelope описывает конверт realtime-события с типом, идентификаторами и полезной нагрузкой.
// @params
//   - Version: версия изменения для защиты от устаревших операций.
//   - ID: уникальный идентификатор данной сущности.
//   - Type: значение Type типа string, используемое согласно назначению этой операции.
//   - ConferenceID: идентификатор конференции, ограничивающий область операции.
//   - Timestamp: временная отметка Timestamp; указатель допускает отсутствие значения.
//   - Data: полезная нагрузка события или байты обрабатываемого содержимого.
//   - ReplyTo: идентификатор исходного запроса или сообщения, на которое даётся ответ.
type Envelope struct {
	Version      int             `json:"version"`
	ID           string          `json:"id"`
	Type         string          `json:"type"`
	ConferenceID string          `json:"conferenceId"`
	Timestamp    time.Time       `json:"timestamp"`
	Data         json.RawMessage `json:"data"`
	ReplyTo      string          `json:"replyTo,omitempty"`
}

// Event формирует серверный конверт события с типом, конференцией и полезной нагрузкой.
//
// @parameter:
//   - kind (string): тип события, ошибки или медиа, определяющий ветку обработки.
//   - conferenceID (string): идентификатор конференции, ограничивающий область операции.
//   - data (any): полезная нагрузка события или байты обрабатываемого содержимого.
//
// @return:
//   - результат 1 (Envelope): значение, подготовленное операцией для вызывающей стороны.
func Event(kind, conferenceID string, data any) Envelope {
	raw, _ := json.Marshal(data)
	return Envelope{Version: 1, ID: uuid.NewString(), Type: kind, ConferenceID: conferenceID, Timestamp: time.Now().UTC(), Data: raw}
}

// Presence передаёт агрегированное присутствие участника и число его действующих соединений.
// @params:
//   - conferences.ParticipantView: встроенный тип, добавляющий свой контракт или данные.
//   - Online: логический признак Online, управляющий соответствующей веткой обработки.
//   - Connections: значение Connections типа int, используемое согласно назначению этой операции.
//   - ConnectionIDs: идентификаторы связанных ресурсов для пакетной операции.
type Presence struct {
	conferences.ParticipantView
	Online        bool     `json:"online"`
	Connections   int      `json:"connections"`
	ConnectionIDs []string `json:"connectionIds"`
}

// State собирает начальный снимок комнаты: конференцию и видимых участников.
// @params:
//   - ConnectionID: идентификатор физического медиа-соединения.
//   - ParticipantID: идентификатор членства участника внутри конференции.
//   - Status: состояние ресурса, ответа или фильтра выборки.
//   - Participants: набор значений Participants для последовательной или пакетной обработки.
type State struct {
	ConnectionID  string             `json:"connectionId"`
	ParticipantID string             `json:"participantId"`
	Status        conferences.Status `json:"status"`
	Participants  []Presence         `json:"participants"`
}

// Signal содержит ограниченную нагрузку сигнализации; доверенная идентичность назначается сервером.
// @params:
//   - TargetConnectionID: идентификатор связанного ресурса, заданного параметром TargetConnectionID.
//   - SDP: значение SDP типа string, используемое согласно назначению этой операции.
//   - Candidate: проверенный кандидат ICE для WebRTC-соединения.
//   - SenderConnectionID: идентификатор связанного ресурса, заданного параметром SenderConnectionID.
//   - SenderParticipantID: идентификатор связанного ресурса, заданного параметром SenderParticipantID.
type Signal struct {
	TargetConnectionID  string          `json:"targetConnectionId"`
	SDP                 string          `json:"sdp,omitempty"`
	Candidate           json.RawMessage `json:"candidate,omitempty"`
	SenderConnectionID  string          `json:"senderConnectionId,omitempty"`
	SenderParticipantID string          `json:"senderParticipantId,omitempty"`
}

// ICEConfig задаёт список ICE-серверов, необходимый для установки WebRTC-соединения.
// @params:
//   - ICEServers: набор значений ICEServers для последовательной или пакетной обработки.
type ICEConfig struct {
	ICEServers         []ICEServer `json:"iceServers"`
	ICETransportPolicy string      `json:"iceTransportPolicy,omitempty"`
	ExpiresAt          int64       `json:"expiresAt,omitempty"`
}

// ICEServer описывает адреса и при необходимости учётные данные одного ICE-сервера.
// @params:
//   - URLs: набор значений URLs для последовательной или пакетной обработки.
//   - Username: значение Username типа string, используемое согласно назначению этой операции.
//   - Credential: значение Credential типа string, используемое согласно назначению этой операции.
type ICEServer struct {
	URLs       []string `json:"urls"`
	Username   string   `json:"username,omitempty"`
	Credential string   `json:"credential,omitempty"`
}

// Bus передаёт доверенное событие и параметры адресации между экземплярами API.
// @params:
//   - Kind: тип события, ошибки или медиа, определяющий ветку обработки.
//   - Session: историческая физическая сессия или состояние текущего соединения.
//   - ConferenceID: идентификатор конференции, ограничивающий область операции.
//   - ParticipantID: идентификатор членства участника внутри конференции.
//   - ConnectionID: идентификатор физического медиа-соединения.
//   - Event: конверт входящего или публикуемого события.
type Bus struct {
	Kind          string    `json:"kind"`
	Session       *Session  `json:"session,omitempty"`
	ConferenceID  string    `json:"conferenceId"`
	ParticipantID string    `json:"participantId,omitempty"`
	ConnectionID  string    `json:"connectionId,omitempty"`
	Event         *Envelope `json:"event,omitempty"`
}

// Subscription задаёт контракт зависимого компонента Subscription в присутствии участников и доставке realtime-событий; позволяет заменять реализацию хранилища или транспорта без изменения вызывающего кода.
// @params:
//   - Receive: операция Receive с контрактом, описанным у метода.
//   - Close: операция закрытие с контрактом, описанным у метода.
type Subscription interface {
	// Receive ожидает и разбирает следующее сообщение активной подписки.
	//
	// @parameter:
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//
	// @return:
	//   - результат 1 (Bus): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Receive(context.Context) (Bus, error)
	// Close закрывает принадлежащие компоненту ресурсы и завершает связанный жизненный цикл.
	//
	//
	// @return:
	//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Close() error
}
