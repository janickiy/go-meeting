package media

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/janickiy/go-recorder/internal/domain/realtime"
)

// Media signaling is capped separately from the configurable Stage 2 relay.
const MaxSDPBytes = 49152
const MaxICEBytes = 4096

// Binding is constructed by the API from an authenticated WebSocket session,
// never from browser supplied identity fields.
type Binding struct {
	ConferenceID           string    `json:"conferenceId"`
	ParticipantID          string    `json:"participantId"`
	SessionID              string    `json:"sessionId"`
	ConnectionID           string    `json:"connectionId"`
	UserID                 string    `json:"userId"`
	AuthorizationExpiresAt time.Time `json:"authorizationExpiresAt"`
}

type Route struct {
	WorkerID string `json:"workerId"`
	Endpoint string `json:"endpoint"`
	LeaseID  string `json:"leaseId"`
}

type Worker struct {
	ID       string `json:"id"`
	Endpoint string `json:"endpoint"`
}

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

type Track struct {
	ID            string `json:"id"`
	StreamID      string `json:"streamId"`
	MediaPeerID   string `json:"mediaPeerId"`
	ParticipantID string `json:"participantId"`
	Kind          Kind   `json:"kind"`
	Source        Source `json:"source"`
}

// Publication binds a browser sending m-section to its semantic source. MID is
// stable across replaceTrack; browser track IDs are not.
type Publication struct {
	MID     string `json:"mid"`
	Source  Source `json:"source"`
	TrackID string `json:"trackId,omitempty"`
}

type ParticipantPolicy struct {
	Version           int64 `json:"version"`
	MicrophoneBlocked bool  `json:"microphoneBlocked"`
	CameraBlocked     bool  `json:"cameraBlocked"`
	ScreenBlocked     bool  `json:"screenBlocked"`
	Kicked            bool  `json:"kicked"`
}

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

// Command is used exclusively on the protected API-to-worker transport.
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

type VideoCaptureTarget struct {
	MaxWidth     int `json:"maxWidth"`
	MaxHeight    int `json:"maxHeight"`
	MaxFrameRate int `json:"maxFrameRate"`
}

type Result struct {
	MediaPeerID   string               `json:"mediaPeerId"`
	WorkerID      string               `json:"workerId,omitempty"`
	MaxPeers      int                  `json:"maxPeers,omitempty"`
	ICEServers    []realtime.ICEServer `json:"iceServers,omitempty"`
	Tracks        []Track              `json:"tracks,omitempty"`
	NegotiationID string               `json:"negotiationId,omitempty"`
	SDP           string               `json:"sdp,omitempty"`
	VideoCapture  VideoCaptureTarget   `json:"videoCapture"`
	Policy        *ParticipantPolicy   `json:"policy,omitempty"`
}

// Signal is the bounded browser payload. Its identity is always server assigned.
type Signal struct {
	MediaPeerID   string          `json:"mediaPeerId,omitempty"`
	NegotiationID string          `json:"negotiationId,omitempty"`
	SDP           string          `json:"sdp,omitempty"`
	Candidate     json.RawMessage `json:"candidate,omitempty"`
	TrackID       string          `json:"trackId,omitempty"`
	Publications  []Publication   `json:"publications,omitempty"`
}

var (
	ErrUnavailable    = errors.New("media_unavailable")
	ErrInvalid        = errors.New("invalid_media_signal")
	ErrUnauthorized   = errors.New("media_unauthorized")
	ErrOwnership      = errors.New("media_ownership_lost")
	ErrLimit          = errors.New("media_limit_exceeded")
	ErrPeerNotFound   = errors.New("media_peer_not_found")
	ErrNegotiation    = errors.New("media_negotiation_conflict")
	ErrScreenConflict = errors.New("screen_sharing_conflict")
	ErrPolicy         = errors.New("media_policy_blocked")
)

func ErrorCode(err error) string {
	for _, known := range []error{ErrUnavailable, ErrInvalid, ErrUnauthorized, ErrOwnership, ErrLimit, ErrPeerNotFound, ErrNegotiation, ErrScreenConflict, ErrPolicy} {
		if errors.Is(err, known) {
			return known.Error()
		}
	}
	return ErrUnavailable.Error()
}
