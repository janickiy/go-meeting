// Package analytics описывает технические агрегаты встреч без оценки личности или продуктивности.
package analytics

import "time"

// Interval хранит наблюдаемое присутствие; пересекающиеся подключения одного участника объединяются.
type Interval struct {
	ParticipantID string
	Start, End    time.Time
}

// Point задаёт число одновременно присутствующих участников на общей шкале времени встречи.
type Point struct {
	AtMS  int64 `json:"atMs"`
	Count int   `json:"count"`
}

// Participant содержит объективные счётчики и приблизительную активность аудио.
type Participant struct {
	ParticipantID   string `json:"participantId"`
	DisplayName     string `json:"displayName"`
	ParticipationMS int64  `json:"participationMs"`
	SpeakingMS      int64  `json:"speakingMs"`
	ObservedAudioMS int64  `json:"observedAudioMs"`
	ScreenMS        int64  `json:"screenMs"`
	MessageCount    int64  `json:"messageCount"`
	HandRaises      int64  `json:"handRaises"`
}

// Conference возвращает агрегаты с явным указанием приближённости речевой активности.
type Conference struct {
	ConferenceID        string        `json:"conferenceId"`
	Enabled             bool          `json:"enabled"`
	DurationMS          int64         `json:"durationMs"`
	ParticipantCount    int           `json:"participantCount"`
	Timeline            []Point       `json:"timeline" gorm:"-"`
	Participants        []Participant `json:"participants" gorm:"-"`
	RecordingAvailable  bool          `json:"recordingAvailable"`
	TranscriptAvailable bool          `json:"transcriptAvailable"`
	ApproximateSpeaking bool          `json:"approximateSpeaking"`
	UpdatedAt           time.Time     `json:"updatedAt"`
}
