package records

import "time"

type ConferenceStartRequest struct {
	SegmentDurationSec int `json:"segmentDurationSec,omitempty"`
}
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

func (OutboxCommand) TableName() string { return "recording_outbox" }

func IsComposite(record Record) bool {
	return record.Mode == ModeComposite || record.SourceType == "conference" || record.PlatformConferenceID != nil
}
func PublicStatus(status string) string {
	if status == StatusFinalizing || status == StatusUploading {
		return "processing"
	}
	return status
}
