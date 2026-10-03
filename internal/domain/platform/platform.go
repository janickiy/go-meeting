// Пакет platform описывает безопасные возможности продукта и доступные только для чтения операционные сводки.
package platform

import "time"

// Capabilities описывает серверные возможности без адресов провайдеров, токенов и секретов.
type Capabilities struct {
	LiveCaptions     bool     `json:"liveCaptions"`
	Transcription    bool     `json:"transcription"`
	AISummary        bool     `json:"aiSummary"`
	SemanticSearch   bool     `json:"semanticSearch"`
	MeetingAnalytics bool     `json:"meetingAnalytics"`
	RecordingModes   []string `json:"recordingModes"`
}

// Failure представляет ограниченное операционное событие без ID пользователей, встреч и содержимого.
type Failure struct {
	Kind string    `json:"kind"`
	Code string    `json:"code"`
	At   time.Time `json:"at"`
}

// Summary содержит агрегированный снимок для администратора с глобальными полномочиями.
type Summary struct {
	AsOf                    time.Time       `json:"asOf"`
	ActiveConferences       int64           `json:"activeConferences"`
	JoinedParticipants      int64           `json:"joinedParticipants"`
	ActiveRecordings        int64           `json:"activeRecordings"`
	QueuedJobs              int64           `json:"queuedJobs"`
	FailedJobs24h           int64           `json:"failedJobs24h"`
	FailedRecordings24h     int64           `json:"failedRecordings24h"`
	FailedTranscriptions24h int64           `json:"failedTranscriptions24h"`
	APIReady                bool            `json:"apiReady"`
	MediaWorkerReady        bool            `json:"mediaWorkerReady"`
	Dependencies            map[string]bool `json:"dependencies"`
	RecentFailures          []Failure       `json:"recentFailures"`
}
