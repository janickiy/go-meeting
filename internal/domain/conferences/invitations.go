package conferences

import (
	"time"

	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/users"
)

// ValidateInvitationEmail rejects addresses requiring SMTPUTF8 before they enter the queue.
func ValidateInvitationEmail(email string) error {
	if err := users.ValidateEmail(email); err != nil {
		return err
	}
	for _, character := range email {
		if character > 127 {
			return apperrors.New(apperrors.ErrInvalidInput, "Укажите email латинскими буквами. Международные адреса email пока не поддерживаются.")
		}
	}
	return nil
}

// InvitationRequest contains addresses or registered accounts selected by an organizer.
type InvitationRequest struct {
	Emails  []string `json:"emails,omitempty"`
	UserIDs []string `json:"userIds,omitempty"`
}

// InvitationUser exposes only the account information needed to select an invitee.
type InvitationUser struct {
	ID          string  `json:"id"`
	Email       string  `json:"email"`
	DisplayName *string `json:"displayName,omitempty"`
}

// Invitation stores a durable delivery request, separately from actual room presence.
type Invitation struct {
	ID           string `gorm:"type:uuid;primaryKey"`
	ConferenceID string
	RequestedBy  string
	Email        string
	UserID       *string
	Status       string
	ErrorCode    string
	CreatedAt    time.Time
	SentAt       *time.Time
}

func (Invitation) TableName() string { return "conference_invitations" }

// InvitationResult reports acceptance, without claiming that SMTP has delivered the message.
type InvitationResult struct {
	ID     string  `json:"id"`
	Email  string  `json:"email"`
	UserID *string `json:"userId"`
	Status string  `json:"status"`
}
