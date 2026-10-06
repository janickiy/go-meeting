package personal

import "time"

// Peer deliberately excludes email, account roles and password/session metadata.
type Peer struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
}
type Conversation struct {
	ID            string     `json:"id"`
	Type          string     `json:"type"`
	Peer          Peer       `json:"peer"`
	CreatedAt     time.Time  `json:"createdAt"`
	LastMessageAt *time.Time `json:"lastMessageAt"`
	LastMessageID *string    `json:"lastMessageId"`
	Preview       string     `json:"preview"`
	UnreadCount   int64      `json:"unreadCount"`
	ActivityAt    time.Time  `json:"-"`
}
type Page struct {
	Items       []Conversation `json:"items"`
	NextCursor  string         `json:"nextCursor,omitempty"`
	UnreadCount int64          `json:"unreadCount"`
}
