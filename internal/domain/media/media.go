package media

import (
	"encoding/json"
	"time"

	"github.com/janickiy/meet-space/internal/domain/conferences"
	"github.com/janickiy/meet-space/internal/domain/realtime"
)

// Ограничения сигнализации медиа задаются отдельно от настраиваемой ретрансляции этапа 2.
const MaxSDPBytes = 49152
const MaxICEBytes = 4096

// Binding links a media peer to a verified WebSocket session. Browser payloads cannot supply this identity.
type Binding struct {
	ConferenceID           string    `json:"conferenceId"`
	ParticipantID          string    `json:"participantId"`
	SessionID              string    `json:"sessionId"`
	ConnectionID           string    `json:"connectionId"`
	UserID                 string    `json:"userId"`
	AuthorizationExpiresAt time.Time `json:"authorizationExpiresAt"`
}

// Route хранит адрес и версию владельца медиа-комнаты для защищённого внутреннего вызова.
// @params
//   - WorkerID: идентификатор воркера-владельца операции.
//   - Endpoint: адрес конечной точки вызываемого сервиса.
//   - LeaseID: идентификатор связанного ресурса, заданного параметром LeaseID.
type Route struct {
	WorkerID string `json:"workerId"`
	Endpoint string `json:"endpoint"`
	LeaseID  string `json:"leaseId"`
}

// Worker описывает присутствие медиа-воркера в распределённом реестре.
// @params
//   - ID: уникальный идентификатор данной сущности.
//   - Endpoint: адрес конечной точки вызываемого сервиса.
type Worker struct {
	Draining bool   `json:"draining,omitempty"`
	ID       string `json:"id"`
	Endpoint string `json:"endpoint"`
}

// PeerView возвращает доступное представление физического медиа-подключения.
// @params
//   - MediaPeerID: идентификатор связанного ресурса, заданного параметром MediaPeerID.
//   - ConferenceID: идентификатор конференции, ограничивающий область операции.
//   - ParticipantID: идентификатор членства участника внутри конференции.
//   - SessionID: идентификатор одной физической сессии подключения.
//   - ConnectionID: идентификатор физического медиа-соединения.
type PeerView struct {
	MediaPeerID   string `json:"mediaPeerId"`
	ConferenceID  string `json:"conferenceId"`
	ParticipantID string `json:"participantId"`
	SessionID     string `json:"sessionId"`
	ConnectionID  string `json:"connectionId"`
}

type Kind string
type Source string

const (
	KindAudio         Kind   = "audio"
	KindVideo         Kind   = "video"
	SourceMicrophone  Source = "microphone"
	SourceCamera      Source = "camera"
	SourceVideoScreen Source = "video/screen"
	SourceAudioScreen Source = "audio/screen"
)

// Track описывает опубликованную медиа-дорожку и её источник.
// @params:
//   - ID: уникальный идентификатор данной сущности.
//   - StreamID: идентификатор связанного ресурса, заданного параметром StreamID.
//   - MediaPeerID: идентификатор связанного ресурса, заданного параметром MediaPeerID.
//   - ParticipantID: идентификатор членства участника внутри конференции.
//   - Kind: тип события, ошибки или медиа, определяющий ветку обработки.
//   - Source: семантический источник медиа либо входной источник данных.
type Track struct {
	ID            string `json:"id"`
	StreamID      string `json:"streamId"`
	MediaPeerID   string `json:"mediaPeerId"`
	ParticipantID string `json:"participantId"`
	Kind          Kind   `json:"kind"`
	Source        Source `json:"source"`
}

// Publication maps SDP MID to a source. TrackID identifies a browser capture generation, not a server track.
type Publication struct {
	MID     string `json:"mid"`
	Source  Source `json:"source"`
	TrackID string `json:"trackId,omitempty"`
}

// ParticipantPolicy описывает сохранённые разрешения источников медиа и монотонную версию модерации.
// @params
//   - Version: версия изменения для защиты от устаревших операций.
//   - MicrophoneBlocked: серверный запрет микрофона.
//   - CameraBlocked: серверный запрет камеры.
//   - ScreenBlocked: серверный запрет экрана.
//   - Kicked: логический признак Kicked, управляющий соответствующей веткой обработки.
type ParticipantPolicy struct {
	Version           int64 `json:"version"`
	MicrophoneBlocked bool  `json:"microphoneBlocked"`
	CameraBlocked     bool  `json:"cameraBlocked"`
	ScreenBlocked     bool  `json:"screenBlocked"`
	Kicked            bool  `json:"kicked"`
}

// Allows проверяет, разрешает ли политика участника передачу указанного источника медиа.
//
// @args
//   - source (Source): семантический источник медиа либо входной источник данных.
//
// @return:
//   - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния.
func (p ParticipantPolicy) Allows(source Source) bool {
	if p.Kicked {
		return false
	}
	switch source {
	case SourceMicrophone:
		return !p.MicrophoneBlocked
	case SourceCamera:
		return !p.CameraBlocked
	case SourceVideoScreen:
		return !p.ScreenBlocked && !p.CameraBlocked
	case SourceAudioScreen:
		return !p.ScreenBlocked && !p.MicrophoneBlocked && !p.CameraBlocked
	default:
		return false
	}
}

// SourceKind определяет тип медиа по семантическому источнику, различая звук, камеру и экран.
//
// @args
//   - source (Source): семантический источник медиа либо входной источник данных.
//
// @return:
//   - результат 1 (Kind): значение, подготовленное операцией для вызывающей стороны.
func SourceKind(source Source) Kind {
	switch source {
	case SourceMicrophone, SourceAudioScreen:
		return KindAudio
	case SourceCamera, SourceVideoScreen:
		return KindVideo
	default:
		return ""
	}
}

// Command задаёт согласованное представление данных «Command» для защищённом управлении медиа-комнатой.
// @params
//   - RequestID: идентификатор запроса сигнализации для сопоставления ответа.
//   - Binding: проверенная идентичность медиа-подключения, назначенная сервером.
//   - Route: адрес и версия действующего владельца медиа-комнаты.
//   - MediaPeerID: идентификатор связанного ресурса, заданного параметром MediaPeerID.
//   - NegotiationID: идентификатор связанного ресурса, заданного параметром NegotiationID.
//   - SDP: значение SDP типа string, используемое согласно назначению этой операции.
//   - Candidate: проверенный кандидат ICE для WebRTC-соединения.
//   - TrackID: идентификатор связанного ресурса, заданного параметром TrackID.
//   - Ticket: одноразовый билет ограниченного подключения.
//   - Publications: набор значений Publications для последовательной или пакетной обработки.
//   - Policy: актуальные ограничения медиа и версия модерации участника.
//   - ConferenceID: идентификатор конференции, ограничивающий область операции.
//   - ParticipantID: идентификатор членства участника внутри конференции.
type Command struct {
	RequestID     string             `json:"requestId"`
	Binding       Binding            `json:"binding"`
	Route         Route              `json:"route"`
	MediaPeerID   string             `json:"mediaPeerId,omitempty"`
	NegotiationID string             `json:"negotiationId,omitempty"`
	SDP           string             `json:"sdp,omitempty"`
	Candidate     json.RawMessage    `json:"candidate,omitempty"`
	TrackID       string             `json:"trackId,omitempty"`
	Ticket        string             `json:"ticket,omitempty"`
	Publications  []Publication      `json:"publications"`
	Policy        *ParticipantPolicy `json:"policy,omitempty"`
	ConferenceID  string             `json:"conferenceId,omitempty"`
	ParticipantID string             `json:"participantId,omitempty"`
}

// VideoCaptureTarget задаёт параметры видеозахвата, применяемые к выбранному профилю качества.
// @params
//   - MaxWidth: значение MaxWidth типа int, используемое согласно назначению этой операции.
//   - MaxHeight: значение MaxHeight типа int, используемое согласно назначению этой операции.
//   - MaxFrameRate: значение MaxFrameRate типа int, используемое согласно назначению этой операции.
type VideoCaptureTarget struct {
	MaxWidth     int `json:"maxWidth"`
	MaxHeight    int `json:"maxHeight"`
	MaxFrameRate int `json:"maxFrameRate"`
}

// Result передаёт результат операции и связанные метаданные компонента.
// @params
//   - MediaPeerID: идентификатор связанного ресурса, заданного параметром MediaPeerID.
//   - WorkerID: идентификатор воркера-владельца операции.
//   - MaxPeers: значение MaxPeers типа int, используемое согласно назначению этой операции.
//   - ICEServers: набор значений ICEServers для последовательной или пакетной обработки.
//   - Tracks: набор дорожек, входящих в операцию.
//   - NegotiationID: идентификатор связанного ресурса, заданного параметром NegotiationID.
//   - SDP: значение SDP типа string, используемое согласно назначению этой операции.
//   - VideoCapture: значение VideoCapture типа VideoCaptureTarget, используемое согласно назначению этой операции.
//   - Policy: актуальные ограничения медиа и версия модерации участника.
type Result struct {
	MediaPeerID        string               `json:"mediaPeerId"`
	WorkerID           string               `json:"workerId,omitempty"`
	MaxPeers           int                  `json:"maxPeers,omitempty"`
	ICEServers         []realtime.ICEServer `json:"iceServers,omitempty"`
	ICETransportPolicy string               `json:"iceTransportPolicy,omitempty"`
	ICEExpiresAt       int64                `json:"iceExpiresAt,omitempty"`
	Tracks             []Track              `json:"tracks,omitempty"`
	NegotiationID      string               `json:"negotiationId,omitempty"`
	SDP                string               `json:"sdp,omitempty"`
	VideoCapture       VideoCaptureTarget   `json:"videoCapture"`
	Policy             *ParticipantPolicy   `json:"policy,omitempty"`
}

// Signal содержит ограниченную нагрузку сигнализации; доверенная идентичность назначается сервером.
// @params
//   - MediaPeerID: идентификатор связанного ресурса, заданного параметром MediaPeerID.
//   - NegotiationID: идентификатор связанного ресурса, заданного параметром NegotiationID.
//   - SDP: значение SDP типа string, используемое согласно назначению этой операции.
//   - Candidate: проверенный кандидат ICE для WebRTC-соединения.
//   - TrackID: идентификатор связанного ресурса, заданного параметром TrackID.
//   - Publications: набор значений Publications для последовательной или пакетной обработки.
type Signal struct {
	MediaPeerID   string          `json:"mediaPeerId,omitempty"`
	NegotiationID string          `json:"negotiationId,omitempty"`
	SDP           string          `json:"sdp,omitempty"`
	Candidate     json.RawMessage `json:"candidate,omitempty"`
	TrackID       string          `json:"trackId,omitempty"`
	Publications  []Publication   `json:"publications,omitempty"`
}

// PolicyForParticipant projects authoritative membership into versioned media
// permissions. Conference availability is enforced separately by its owner.
func PolicyForParticipant(p conferences.Participant) ParticipantPolicy {
	return ParticipantPolicy{Version: p.MediaPolicyVersion, MicrophoneBlocked: p.MicrophoneBlocked, CameraBlocked: p.CameraBlocked, ScreenBlocked: p.ScreenBlocked, Kicked: !p.CanParticipate()}
}
