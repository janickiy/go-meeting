package chat

// Info is the member-specific view of a conference chat.
type Info struct {
	ConferenceID         string `json:"conferenceId"`
	Title                string `json:"title"`
	Description          string `json:"description"`
	InviteURL            string `json:"inviteUrl"`
	ParticipantCount     int64  `json:"participantCount"`
	NotificationsEnabled bool   `json:"notificationsEnabled"`
	CanEdit              bool   `json:"canEdit"`
	CanInvite            bool   `json:"canInvite"`
}

type Preferences struct {
	NotificationsEnabled bool `json:"notificationsEnabled"`
}

type UpdateInfoRequest struct {
	Title       *string `json:"title,omitempty"`
	Description *string `json:"description,omitempty"`
}

type Material struct {
	Message    Message     `json:"message"`
	Attachment *Attachment `json:"attachment,omitempty"`
	URL        string      `json:"url,omitempty"`
}

type MessagePage struct {
	Items      []Message `json:"items"`
	NextCursor string    `json:"nextCursor,omitempty"`
}

type MaterialPage struct {
	Items      []Material `json:"items"`
	NextCursor string     `json:"nextCursor,omitempty"`
}
