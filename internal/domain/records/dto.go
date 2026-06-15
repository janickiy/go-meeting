package records

import "time"

// Response описывает единый JSON-ответ ошибок.
type Response struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// StartRequest описывает тело POST /api/v1/records/start.
type StartRequest struct {
	ConferenceID       string  `json:"conferenceId"`
	RequestedBy        *string `json:"requestedBy"`
	Quality            string  `json:"quality"`
	QualityMode        string  `json:"qualityMode"`
	MinQuality         string  `json:"minQuality"`
	SegmentDurationSec int     `json:"segmentDurationSec"`
}

// StartResponse описывает успешный ответ старта записи.
type StartResponse struct {
	Status       string     `json:"status"`
	Message      string     `json:"message"`
	RecordID     string     `json:"recordId"`
	ConferenceID string     `json:"conferenceId"`
	WebRTC       WebRTCInfo `json:"webrtc"`
}

// EndRequest описывает тело POST /api/v1/records/end.
type EndRequest struct {
	RecordID string `json:"recordId"`
	Reason   string `json:"reason"`
}

// Command описывает внутреннюю команду API -> worker.
type Command struct {
	Type               string `json:"type"`
	RecordID           string `json:"recordId"`
	Reason             string `json:"reason,omitempty"`
	SegmentDurationSec int    `json:"segmentDurationSec,omitempty"`
}

// WebRTCInfo описывает параметры browser signaling для debug/frontend.
type WebRTCInfo struct {
	OfferURL   string        `json:"offerUrl"`
	ICEServers []ICEServer   `json:"iceServers"`
	Video      VideoSettings `json:"video"`
}

// ICEServer описывает ICE server в формате browser RTCPeerConnection.
type ICEServer struct {
	URLs []string `json:"urls"`
}

// WebRTCOfferRequest описывает SDP offer браузера.
type WebRTCOfferRequest struct {
	Type string `json:"type"`
	SDP  string `json:"sdp"`
}

// WebRTCAnswerResponse описывает SDP answer worker-а.
type WebRTCAnswerResponse struct {
	Type string `json:"type"`
	SDP  string `json:"sdp"`
}

// RecordDetails содержит запись и связанные сущности из БД/MinIO.
type RecordDetails struct {
	Record   Record
	Files    []RecordFile
	Segments []RecordSegment
	Events   []RecordEvent
}

// RecordCard описывает JSON-карточку записи для API.
type RecordCard struct {
	Record
	Files    []RecordFileView    `json:"files"`
	Segments []RecordSegmentView `json:"segments"`
	Events   []RecordEvent       `json:"events,omitempty"`
}

// RecordFileView описывает файл записи с MinIO ссылкой.
type RecordFileView struct {
	RecordFile
	URL string `json:"url,omitempty"`
}

// RecordSegmentView описывает сегмент записи с MinIO ссылкой.
type RecordSegmentView struct {
	RecordSegment
	Bucket string `json:"bucket,omitempty"`
	URL    string `json:"url,omitempty"`
}

// ConferenceRecordSummary описывает записи одной конференции и их количество.
type ConferenceRecordSummary struct {
	ConferenceID string                 `json:"conferenceId"`
	RecordsCount int64                  `json:"recordsCount"`
	Records      []ConferenceRecordItem `json:"records"`
}

// ConferenceRecordItem описывает одну запись конференции в сводном endpoint-е.
type ConferenceRecordItem struct {
	RecordID    string     `json:"recordId"`
	Status      string     `json:"status"`
	FinalURL    string     `json:"finalUrl,omitempty"`
	PreviewURL  string     `json:"previewUrl,omitempty"`
	DurationSec *int       `json:"durationSec,omitempty"`
	StartedAt   *time.Time `json:"startedAt,omitempty"`
	StoppedAt   *time.Time `json:"stoppedAt,omitempty"`
	EndedAt     *time.Time `json:"endedAt,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
}
