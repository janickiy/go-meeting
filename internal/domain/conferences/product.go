package conferences

import (
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
)

// Empty admission exists only in pre-stage-five in-memory adapters. PostgreSQL
// always stores a nonempty, constrained state; negative membership states deny
// access even in a legacy adapter.
func (p Participant) IsAdmitted() bool {
	return (p.AdmissionState == AdmissionAdmitted || p.AdmissionState == "") && p.Status != Waiting && p.Status != Rejected && p.Status != Kicked
}
func (p Participant) CanParticipate() bool { return p.IsAdmitted() && p.Status == Joined }
func (p Participant) CanReadHistory() bool {
	return p.IsAdmitted() && (p.Status == Joined || p.Status == Left)
}
func (p Participant) CanAdmit() bool {
	return p.CanParticipate() && (p.Role == Owner || p.Role == CoHost)
}

type AdmissionRequest struct {
	Decision string `json:"decision"`
}

func (r AdmissionRequest) Validate() error {
	if r.Decision != "admit" && r.Decision != "reject" {
		return apperrors.New(apperrors.ErrInvalidInput, "decision must be admit or reject")
	}
	return nil
}

type ScheduleRequest struct {
	ScheduledAt        time.Time `json:"scheduledAt"`
	PlannedDurationMin *int      `json:"plannedDurationMin"`
}

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

type TimelineQuery struct {
	View   string
	Scope  string
	Status Status
	From   *time.Time
	To     *time.Time
	Cursor string
	Limit  int
}
type TimelineCursor struct {
	At time.Time `json:"at"`
	ID string    `json:"id"`
}

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

type TimelinePage struct {
	Items      []View  `json:"items"`
	NextCursor *string `json:"nextCursor"`
}
type RecordingSummary struct {
	Total      int64 `json:"total"`
	Ready      int64 `json:"ready"`
	Processing int64 `json:"processing"`
	Failed     int64 `json:"failed"`
}
type HistoryOwner struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
}
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
