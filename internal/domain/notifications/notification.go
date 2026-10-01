package notifications

import (
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
)

// Payload contains references only. Authorization is rechecked when opening
// the conference/recording; notifications never carry chat or file contents.
type Payload struct {
	ConferenceID   string     `json:"conferenceId"`
	RecordingID    string     `json:"recordingId,omitempty"`
	ScheduledAt    *time.Time `json:"scheduledAt,omitempty"`
	AdmissionState string     `json:"admissionState,omitempty"`
}
type Notification struct {
	ID          string     `json:"id" gorm:"type:uuid;primaryKey"`
	UserID      string     `json:"userId" gorm:"type:uuid"`
	Type        string     `json:"type"`
	Version     int        `json:"version"`
	Payload     Payload    `json:"payload" gorm:"serializer:json;type:jsonb"`
	DedupKey    string     `json:"-"`
	CreatedAt   time.Time  `json:"createdAt"`
	ReadAt      *time.Time `json:"readAt"`
	PublishedAt *time.Time `json:"-"`
}

func (Notification) TableName() string { return "notifications" }

type Page struct {
	Items       []Notification `json:"items"`
	NextCursor  *string        `json:"nextCursor"`
	UnreadCount int64          `json:"unreadCount"`
}
type Cursor struct {
	UserID string    `json:"userId"`
	At     time.Time `json:"at"`
	ID     string    `json:"id"`
}

func DecodeCursor(value, userID string) (*Cursor, error) {
	if value == "" {
		return nil, nil
	}
	if len(value) > 512 {
		return nil, apperrors.ErrInvalidInput
	}
	b, err := base64.RawURLEncoding.Strict().DecodeString(value)
	var c Cursor
	if err != nil || json.Unmarshal(b, &c) != nil || c.UserID != userID || c.At.IsZero() {
		return nil, apperrors.ErrInvalidInput
	}
	id, err := uuid.Parse(c.ID)
	if err != nil || id == uuid.Nil {
		return nil, apperrors.ErrInvalidInput
	}
	c.ID = id.String()
	return &c, nil
}
func EncodeCursor(n Notification) string {
	b, _ := json.Marshal(Cursor{UserID: n.UserID, At: n.CreatedAt, ID: n.ID})
	return base64.RawURLEncoding.EncodeToString(b)
}
