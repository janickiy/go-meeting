package conferences

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/janickiy/go-recorder/internal/domain/apperrors"
)

type Status string
type Role string
type ParticipantStatus string
type AdmissionState string

const (
	Created           Status            = "created"
	Scheduled         Status            = "scheduled"
	Active            Status            = "active"
	Finished          Status            = "finished"
	Cancelled         Status            = "cancelled"
	Owner             Role              = "owner"
	CoHost            Role              = "co_host"
	ParticipantRole   Role              = "participant"
	Guest             Role              = "guest"
	Joined            ParticipantStatus = "joined"
	Left              ParticipantStatus = "left"
	Waiting           ParticipantStatus = "waiting"
	Rejected          ParticipantStatus = "rejected"
	Kicked            ParticipantStatus = "kicked"
	AdmissionWaiting  AdmissionState    = "waiting"
	AdmissionAdmitted AdmissionState    = "admitted"
	AdmissionRejected AdmissionState    = "rejected"
	AdmissionKicked   AdmissionState    = "kicked"
)

var ErrInviteCollision = errors.New("invite code collision")

func CanTransition(from, to Status) bool {
	return ((from == Created || from == Scheduled) && (to == Active || to == Cancelled)) || (from == Active && to == Finished)
}

type Conference struct {
	ID                 string `gorm:"type:uuid;primaryKey"`
	OwnerID            string `gorm:"column:owner_id;type:uuid"`
	Title              string
	InviteCode         string `gorm:"column:invite_code"`
	Status             Status
	CreatedAt          time.Time
	UpdatedAt          time.Time
	StartedAt          *time.Time
	FinishedAt         *time.Time
	WaitingRoomEnabled bool
	ScheduledAt        *time.Time
	PlannedDurationMin *int
}

func (Conference) TableName() string { return "conferences" }

type Participant struct {
	ID                 string  `gorm:"type:uuid;primaryKey"`
	ConferenceID       string  `gorm:"column:conference_id;type:uuid"`
	UserID             *string `gorm:"column:user_id;type:uuid"`
	DisplayName        string
	Role               Role
	Status             ParticipantStatus
	JoinedAt           *time.Time
	LeftAt             *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
	MicrophoneEnabled  bool
	CameraEnabled      bool
	ScreenSharing      bool
	MicrophoneBlocked  bool
	CameraBlocked      bool
	ScreenBlocked      bool
	MediaPolicyVersion int64          `gorm:"default:1"`
	AdmissionState     AdmissionState `gorm:"default:admitted"`
	AdmissionDecidedAt *time.Time
	AdmissionVersion   int64 `gorm:"default:1"`
}

func (Participant) TableName() string { return "conference_participants" }

type View struct {
	ID                 string     `json:"id"`
	OwnerID            string     `json:"ownerId"`
	Title              string     `json:"title"`
	InviteCode         string     `json:"inviteCode"`
	InviteURL          string     `json:"inviteUrl"`
	Status             Status     `json:"status"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
	StartedAt          *time.Time `json:"startedAt"`
	FinishedAt         *time.Time `json:"finishedAt"`
	WaitingRoomEnabled bool       `json:"waitingRoomEnabled"`
	ScheduledAt        *time.Time `json:"scheduledAt"`
	PlannedDurationMin *int       `json:"plannedDurationMin"`
}

func (c Conference) View() View {
	return View{ID: c.ID, OwnerID: c.OwnerID, Title: c.Title, InviteCode: c.InviteCode,
		InviteURL: "/api/v1/conference-invites/" + c.InviteCode, Status: c.Status,
		CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt, StartedAt: c.StartedAt, FinishedAt: c.FinishedAt,
		WaitingRoomEnabled: c.WaitingRoomEnabled, ScheduledAt: c.ScheduledAt, PlannedDurationMin: c.PlannedDurationMin}
}

type InviteView struct {
	ID                 string     `json:"id"`
	Title              string     `json:"title"`
	Status             Status     `json:"status"`
	WaitingRoomEnabled bool       `json:"waitingRoomEnabled,omitempty"`
	ScheduledAt        *time.Time `json:"scheduledAt,omitempty"`
}

type ParticipantView struct {
	ID                 string            `json:"id"`
	ConferenceID       string            `json:"conferenceId"`
	UserID             *string           `json:"userId"`
	DisplayName        string            `json:"displayName"`
	Role               Role              `json:"role"`
	Status             ParticipantStatus `json:"status"`
	JoinedAt           *time.Time        `json:"joinedAt"`
	LeftAt             *time.Time        `json:"leftAt"`
	CreatedAt          time.Time         `json:"createdAt"`
	UpdatedAt          time.Time         `json:"updatedAt"`
	MicrophoneEnabled  bool              `json:"microphoneEnabled"`
	CameraEnabled      bool              `json:"cameraEnabled"`
	ScreenSharing      bool              `json:"screenSharing"`
	MicrophoneBlocked  bool              `json:"microphoneBlocked"`
	CameraBlocked      bool              `json:"cameraBlocked"`
	ScreenBlocked      bool              `json:"screenBlocked"`
	MediaPolicyVersion int64             `json:"mediaPolicyVersion"`
	AdmissionState     AdmissionState    `json:"admissionState"`
	AdmissionDecidedAt *time.Time        `json:"admissionDecidedAt"`
	AdmissionVersion   int64             `json:"admissionVersion"`
}

func (p Participant) View() ParticipantView {
	return ParticipantView{ID: p.ID, ConferenceID: p.ConferenceID, UserID: p.UserID,
		DisplayName: p.DisplayName, Role: p.Role, Status: p.Status, JoinedAt: p.JoinedAt,
		LeftAt: p.LeftAt, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
		MicrophoneEnabled: p.MicrophoneEnabled, CameraEnabled: p.CameraEnabled, ScreenSharing: p.ScreenSharing,
		MicrophoneBlocked: p.MicrophoneBlocked, CameraBlocked: p.CameraBlocked, ScreenBlocked: p.ScreenBlocked, MediaPolicyVersion: p.MediaPolicyVersion,
		AdmissionState: p.AdmissionState, AdmissionDecidedAt: p.AdmissionDecidedAt, AdmissionVersion: p.AdmissionVersion}
}

type CreateRequest struct {
	Title              string     `json:"title"`
	WaitingRoomEnabled bool       `json:"waitingRoomEnabled"`
	ScheduledAt        *time.Time `json:"scheduledAt"`
	PlannedDurationMin *int       `json:"plannedDurationMin"`
}
type JoinRequest struct {
	InviteCode string `json:"inviteCode"`
}

func NormalizeTitle(value string) (string, error) {
	value = strings.TrimSpace(value)
	if !utf8.ValidString(value) || value == "" || utf8.RuneCountInString(value) > 200 {
		return "", apperrors.New(apperrors.ErrInvalidInput, "title must contain 1 to 200 characters")
	}
	return value, nil
}
