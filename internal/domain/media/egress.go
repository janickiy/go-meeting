package media

// EgressRequest is accepted only on the authenticated internal media endpoint.
// The conference owner lease fences recording input with its SFU room.
type EgressRequest struct {
	RequestID          string `json:"requestId"`
	ConferenceID       string `json:"conferenceId"`
	RecordingID        string `json:"recordingId"`
	Route              Route  `json:"route"`
	SegmentDurationSec int    `json:"segmentDurationSec"`
}

type EgressTrack struct {
	Track
	MimeType    string `json:"mimeType"`
	ClockRate   uint32 `json:"clockRate"`
	Channels    uint16 `json:"channels"`
	PayloadType uint8  `json:"payloadType"`
	SSRC        uint32 `json:"ssrc"`
}

// CapturedAt is worker Unix nanoseconds, sharing the hello frame's clock. RTP
// contains the original encoded RTP packet (JSON encodes []byte as base64).
type EgressFrame struct {
	Type       string       `json:"type"`
	Sequence   uint64       `json:"sequence"`
	CapturedAt int64        `json:"capturedAt"`
	Track      *EgressTrack `json:"track,omitempty"`
	TrackID    string       `json:"trackId,omitempty"`
	RTP        []byte       `json:"rtp,omitempty"`
	Code       string       `json:"code,omitempty"`
}

type EgressSubscription interface {
	Frames() <-chan EgressFrame
	Done() <-chan struct{}
	Err() error
	Close()
	Keyframes()
	Ping()
}
