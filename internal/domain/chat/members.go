package chat

import "github.com/janickiy/go-recorder/internal/domain/conferences"

// MemberView keeps conference membership separate from confirmed live presence.
type MemberView struct {
	conferences.ParticipantView
	Online  *bool `json:"online" gorm:"-"`
	IsGuest bool  `json:"isGuest"`
}
