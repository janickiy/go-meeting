package realtime

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/conferences"
)

// Membership survives sockets. Session is the history of one physical connection.
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

func (Session) TableName() string { return "participant_sessions" }

type Identity struct {
	UserID    string    `json:"userId"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type Envelope struct {
	Version      int             `json:"version"`
	ID           string          `json:"id"`
	Type         string          `json:"type"`
	ConferenceID string          `json:"conferenceId"`
	Timestamp    time.Time       `json:"timestamp"`
	Data         json.RawMessage `json:"data"`
	ReplyTo      string          `json:"replyTo,omitempty"`
}

func Event(kind, conferenceID string, data any) Envelope {
	raw, _ := json.Marshal(data)
	return Envelope{Version: 1, ID: uuid.NewString(), Type: kind, ConferenceID: conferenceID, Timestamp: time.Now().UTC(), Data: raw}
}

type Presence struct {
	conferences.ParticipantView
	Online        bool     `json:"online"`
	Connections   int      `json:"connections"`
	ConnectionIDs []string `json:"connectionIds"`
}
type State struct {
	ConnectionID  string             `json:"connectionId"`
	ParticipantID string             `json:"participantId"`
	Status        conferences.Status `json:"status"`
	Participants  []Presence         `json:"participants"`
	Hands         []Hand             `json:"hands"`
}
type Signal struct {
	TargetConnectionID  string          `json:"targetConnectionId"`
	SDP                 string          `json:"sdp,omitempty"`
	Candidate           json.RawMessage `json:"candidate,omitempty"`
	SenderConnectionID  string          `json:"senderConnectionId,omitempty"`
	SenderParticipantID string          `json:"senderParticipantId,omitempty"`
}
type ICEConfig struct {
	ICEServers []ICEServer `json:"iceServers"`
}
type ICEServer struct {
	URLs       []string `json:"urls"`
	Username   string   `json:"username,omitempty"`
	Credential string   `json:"credential,omitempty"`
}

// Bus contains only trusted, server-created envelopes, never client destinations.
type Bus struct {
	Kind          string    `json:"kind"`
	Session       *Session  `json:"session,omitempty"`
	ConferenceID  string    `json:"conferenceId"`
	ParticipantID string    `json:"participantId,omitempty"`
	ConnectionID  string    `json:"connectionId,omitempty"`
	Event         *Envelope `json:"event,omitempty"`
}

type Subscription interface {
	Receive(context.Context) (Bus, error)
	Close() error
}
