package postgres

import (
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/chat"
	"github.com/janickiy/go-recorder/internal/domain/conferences"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
)

// NewDirectChatRepository changes storage and authorization, sharing all message/file operations.
func NewDirectChatRepository(db *gorm.DB) *ChatRepository {
	r := NewChatRepository(db)
	r.direct = true
	return r
}
func (r *ChatRepository) sql(s string) string {
	if !r.direct {
		return s
	}
	s = strings.ReplaceAll(s, "conference_id", "conversation_id")
	return strings.ReplaceAll(s, "chat_messages", "conversation_messages")
}
func (r *ChatRepository) messages(tx *gorm.DB) *gorm.DB {
	if r.direct {
		return tx.Model(&chat.Message{}).Table("conversation_messages").Omit("ConferenceID", "SenderName")
	}
	return tx.Model(&chat.Message{}).Omit("ConversationID", "SenderName")
}
func (r *ChatRepository) attachments(tx *gorm.DB) *gorm.DB {
	if r.direct {
		return tx.Model(&chat.Attachment{}).Table("conversation_attachments").Omit("ConferenceID", "SenderName")
	}
	return tx.Model(&chat.Attachment{}).Omit("ConversationID", "SenderName")
}
func (r *ChatRepository) readTable() string {
	if r.direct {
		return "conversation_members"
	}
	return "chat_read_states"
}
func (r *ChatRepository) messageScope(m chat.Message) string {
	if r.direct {
		return m.ConversationID
	}
	return m.ConferenceID
}
func (r *ChatRepository) setMessageScope(m *chat.Message, id string) {
	if r.direct {
		m.ConferenceID = ""
		m.ConversationID = id
	}
}
func (r *ChatRepository) setAttachmentScope(a *chat.Attachment, id string) {
	if r.direct {
		a.ConferenceID = ""
		a.ConversationID = id
	}
}
func (r *ChatRepository) authorize(tx *gorm.DB, user, id string, locked, write bool) (conferences.Participant, error) {
	if !r.direct {
		return authorizeChat(tx, user, id, locked, write)
	}
	if err := authorizeDirect(tx, user, id, locked); err != nil {
		return conferences.Participant{}, err
	}
	// Direct conversations have no moderator. The ID is used only to address the reader's account stream.
	return conferences.Participant{ID: user, Role: conferences.ParticipantRole}, nil
}
func authorizeDirect(tx *gorm.DB, user, id string, locked bool) error {
	var row struct{ ID string }
	q := tx.Table("conversations").Select("conversations.id").Where("conversations.id=? AND EXISTS(SELECT 1 FROM conversation_members m JOIN users u ON u.id=m.user_id WHERE m.conversation_id=conversations.id AND m.user_id=? AND u.guest_conference_id IS NULL)", id, user)
	if locked {
		q = q.Clauses(clause.Locking{Strength: "UPDATE", Table: clause.Table{Name: "conversations"}})
	}
	if err := q.Take(&row).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return apperrors.ErrForbidden
		}
		return err
	}
	return nil
}

func (r *ChatRepository) attachmentScope(a chat.Attachment) string {
	if r.direct {
		return a.ConversationID
	}
	return a.ConferenceID
}
