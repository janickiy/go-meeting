package postgres

import (
	"context"
	"encoding/base64"
	"regexp"
	"strconv"
	"strings"

	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/chat"
	"github.com/janickiy/go-recorder/internal/domain/conferences"
	"gorm.io/gorm"
)

var chatURL = regexp.MustCompile(`(?i)https?://[^\s<>"']+`)

// The item index keeps pagination inside a message with several files or links.
func materialCursor(conferenceID string, sequence int64, index int) string {
	return base64.RawURLEncoding.EncodeToString([]byte(conferenceID + ":" + strconv.FormatInt(sequence, 10) + ":" + strconv.Itoa(index)))
}

func parseMaterialCursor(conferenceID, cursor string) (int64, int, error) {
	if cursor == "" {
		return 0, 0, nil
	}
	if len(cursor) > 160 {
		return 0, 0, apperrors.ErrInvalidInput
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return 0, 0, apperrors.ErrInvalidInput
	}
	parts := strings.Split(string(raw), ":")
	if len(parts) != 3 || parts[0] != conferenceID {
		return 0, 0, apperrors.ErrInvalidInput
	}
	sequence, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || sequence < 1 {
		return 0, 0, apperrors.ErrInvalidInput
	}
	index, err := strconv.Atoi(parts[2])
	if err != nil || index < 1 || index > 4000 {
		return 0, 0, apperrors.ErrInvalidInput
	}
	return sequence, index, nil
}

func memberCursor(conferenceID, participantID string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(conferenceID + ":" + participantID))
}

func parseMemberCursor(conferenceID, cursor string) (string, error) {
	if cursor == "" {
		return "", nil
	}
	if len(cursor) > 128 {
		return "", apperrors.ErrInvalidInput
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return "", apperrors.ErrInvalidInput
	}
	parts := strings.Split(string(raw), ":")
	if len(parts) != 2 || parts[0] != conferenceID {
		return "", apperrors.ErrInvalidInput
	}
	id, err := chat.UUID(parts[1])
	if err != nil {
		return "", err
	}
	return id, nil
}

func (r *ChatRepository) chatInfo(tx *gorm.DB, userID, conferenceID string) (chat.Info, error) {
	p, err := authorizeChat(tx, userID, conferenceID, false, false)
	if err != nil {
		return chat.Info{}, err
	}
	var row struct {
		Title       string
		Description string
		InviteCode  string
		OwnerID     string
		Status      conferences.Status
	}
	if err := tx.Table("conferences").Select("title,chat_description AS description,invite_code,owner_id,status").Where("id=?", conferenceID).Take(&row).Error; err != nil {
		return chat.Info{}, mapNotFound(err)
	}
	var count int64
	if err := tx.Table("conference_participants p").Where("p.conference_id=? AND p.user_id IS NOT NULL AND p.admission_state='admitted' AND p.status IN ('joined','left')", conferenceID).
		Where("NOT EXISTS (SELECT 1 FROM conference_chat_preferences cp WHERE cp.conference_id=p.conference_id AND cp.user_id=p.user_id AND cp.left_at IS NOT NULL)").Count(&count).Error; err != nil {
		return chat.Info{}, err
	}
	var pref struct{ NotificationsEnabled bool }
	lookup := tx.Table("conference_chat_preferences").Select("notifications_enabled").Where("conference_id=? AND user_id=?", conferenceID, userID).Scan(&pref)
	if lookup.Error != nil {
		return chat.Info{}, lookup.Error
	}
	open := row.Status == conferences.Created || row.Status == conferences.Scheduled || row.Status == conferences.Active
	canInvite := open && (row.OwnerID == userID || (row.Status == conferences.Active && p.Role == conferences.CoHost && p.CanParticipate()))
	return chat.Info{ConferenceID: conferenceID, Title: row.Title, Description: row.Description,
		InviteURL: "/i/" + row.InviteCode, ParticipantCount: count,
		NotificationsEnabled: lookup.RowsAffected == 0 || pref.NotificationsEnabled, CanEdit: row.OwnerID == userID, CanInvite: canInvite}, nil
}

func (r *ChatRepository) ChatInfo(ctx context.Context, userID, conferenceID string) (chat.Info, error) {
	var result chat.Info
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		result, err = r.chatInfo(tx, userID, conferenceID)
		return err
	})
	return result, err
}

func (r *ChatRepository) ChatMembers(ctx context.Context, userID, conferenceID, cursor string, limit int) ([]conferences.ParticipantView, string, error) {
	items := []conferences.ParticipantView{}
	after, err := parseMemberCursor(conferenceID, cursor)
	if err != nil {
		return items, "", err
	}
	next := ""
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := authorizeChat(tx, userID, conferenceID, false, false); err != nil {
			return err
		}
		rows := []conferences.Participant{}
		q := tx.Table("conference_participants p").Select("p.*").
			Where("p.conference_id=? AND p.user_id IS NOT NULL AND p.admission_state='admitted' AND p.status IN ('joined','left')", conferenceID).
			Where("NOT EXISTS (SELECT 1 FROM conference_chat_preferences cp WHERE cp.conference_id=p.conference_id AND cp.user_id=p.user_id AND cp.left_at IS NOT NULL)")
		if after != "" {
			q = q.Where("p.id>?::uuid", after)
		}
		if err := q.Order("p.id").Limit(limit + 1).Scan(&rows).Error; err != nil {
			return err
		}
		if len(rows) > limit {
			rows = rows[:limit]
			next = memberCursor(conferenceID, rows[len(rows)-1].ID)
		}
		for _, row := range rows {
			items = append(items, row.View())
		}
		return nil
	})
	return items, next, err
}

func (r *ChatRepository) UpdateChatInfo(ctx context.Context, userID, conferenceID string, request chat.UpdateInfoRequest) (chat.Info, error) {
	var result chat.Info
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := authorizeChat(tx, userID, conferenceID, true, false); err != nil {
			return err
		}
		var owner string
		if err := tx.Table("conferences").Select("owner_id").Where("id=?", conferenceID).Scan(&owner).Error; err != nil {
			return err
		}
		if owner != userID {
			return apperrors.ErrForbidden
		}
		updates := map[string]any{}
		if request.Title != nil {
			updates["title"] = *request.Title
		}
		if request.Description != nil {
			updates["chat_description"] = *request.Description
		}
		if len(updates) == 0 {
			return apperrors.ErrInvalidInput
		}
		if err := tx.Table("conferences").Where("id=?", conferenceID).Updates(updates).Error; err != nil {
			return err
		}
		var err error
		result, err = r.chatInfo(tx, userID, conferenceID)
		return err
	})
	return result, err
}

func (r *ChatRepository) ChatPreferences(ctx context.Context, userID, conferenceID string) (chat.Preferences, error) {
	info, err := r.ChatInfo(ctx, userID, conferenceID)
	return chat.Preferences{NotificationsEnabled: info.NotificationsEnabled}, err
}

func (r *ChatRepository) SetChatPreferences(ctx context.Context, userID, conferenceID string, enabled bool) (chat.Preferences, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := authorizeChat(tx, userID, conferenceID, true, false); err != nil {
			return err
		}
		return tx.Exec(`INSERT INTO conference_chat_preferences(conference_id,user_id,notifications_enabled)
			VALUES(?::uuid,?::uuid,?) ON CONFLICT(conference_id,user_id) DO UPDATE
			SET notifications_enabled=EXCLUDED.notifications_enabled,updated_at=clock_timestamp()`, conferenceID, userID, enabled).Error
	})
	return chat.Preferences{NotificationsEnabled: enabled}, err
}

func (r *ChatRepository) LeaveChat(ctx context.Context, userID, conferenceID string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := findConference(tx, conferenceID, true); err != nil {
			return err
		}
		participant, err := findMembership(tx, conferenceID, userID)
		if err != nil {
			if err == apperrors.ErrNotFound {
				return apperrors.ErrForbidden
			}
			return err
		}
		if !participant.CanReadHistory() {
			return apperrors.ErrForbidden
		}
		var left int64
		if err := tx.Table("conference_chat_preferences").Where("conference_id=? AND user_id=? AND left_at IS NOT NULL", conferenceID, userID).Count(&left).Error; err != nil {
			return err
		}
		if left > 0 {
			return nil
		}
		if _, err := authorizeChat(tx, userID, conferenceID, true, false); err != nil {
			return err
		}
		return tx.Exec(`INSERT INTO conference_chat_preferences(conference_id,user_id,left_at)
			VALUES(?::uuid,?::uuid,clock_timestamp()) ON CONFLICT(conference_id,user_id) DO UPDATE
			SET left_at=clock_timestamp(),updated_at=clock_timestamp()`, conferenceID, userID).Error
	})
}

func (r *ChatRepository) SearchMessages(ctx context.Context, userID, conferenceID, query, cursor string, limit int) (chat.MessagePage, error) {
	page := chat.MessagePage{Items: []chat.Message{}}
	before, err := parseChatCursor(conferenceID, cursor)
	if err != nil {
		return page, err
	}
	escaped := strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(query)
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := authorizeChat(tx, userID, conferenceID, false, false); err != nil {
			return err
		}
		q := r.messageQuery(tx).Where("chat_messages.conference_id=? AND chat_messages.deleted_at IS NULL AND chat_messages.text ILIKE ? ESCAPE '!'", conferenceID, "%"+escaped+"%")
		if before > 0 {
			q = q.Where("chat_messages.sequence<?", before)
		}
		if err := q.Order("chat_messages.sequence DESC").Limit(limit + 1).Find(&page.Items).Error; err != nil {
			return err
		}
		if len(page.Items) > limit {
			page.Items = page.Items[:limit]
			page.NextCursor = chatCursor(conferenceID, page.Items[len(page.Items)-1].Sequence)
		}
		if err := r.decorateMessages(tx, page.Items); err != nil {
			return err
		}
		return r.markImportant(tx, userID, conferenceID, page.Items)
	})
	return page, err
}

// ChatMessageContext returns a bounded chronological window around a result.
// The next cursor opens the ordinary older-message paging path.
func (r *ChatRepository) ChatMessageContext(ctx context.Context, userID, conferenceID, messageID string) (chat.Page, error) {
	page := chat.Page{Items: []chat.Message{}}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := authorizeChat(tx, userID, conferenceID, false, false); err != nil {
			return err
		}
		var target chat.Message
		if err := r.messages(tx).Where("conference_id=? AND id=? AND deleted_at IS NULL", conferenceID, messageID).Take(&target).Error; err != nil {
			return mapNotFound(err)
		}
		older := []chat.Message{}
		if err := r.messageQuery(tx).Where("chat_messages.conference_id=? AND chat_messages.sequence<=?", conferenceID, target.Sequence).
			Order("chat_messages.sequence DESC").Limit(25).Find(&older).Error; err != nil {
			return err
		}
		newer := []chat.Message{}
		if err := r.messageQuery(tx).Where("chat_messages.conference_id=? AND chat_messages.sequence>?", conferenceID, target.Sequence).
			Order("chat_messages.sequence ASC").Limit(25).Find(&newer).Error; err != nil {
			return err
		}
		for i := len(older) - 1; i >= 0; i-- {
			page.Items = append(page.Items, older[i])
		}
		page.Items = append(page.Items, newer...)
		if len(page.Items) > 0 {
			var earlier int64
			if err := r.messages(tx).Where("conference_id=? AND sequence<?", conferenceID, page.Items[0].Sequence).Limit(1).Count(&earlier).Error; err != nil {
				return err
			}
			if earlier > 0 {
				page.NextCursor = chatCursor(conferenceID, page.Items[0].Sequence)
			}
		}
		if err := r.decorateMessages(tx, page.Items); err != nil {
			return err
		}
		if err := r.markImportant(tx, userID, conferenceID, page.Items); err != nil {
			return err
		}
		state, err := r.readState(tx, userID, conferenceID)
		page.UnreadCount, page.LastReadMessageID = state.UnreadCount, state.LastReadMessageID
		return err
	})
	return page, err
}

func (r *ChatRepository) ChatMaterials(ctx context.Context, userID, conferenceID, kind, cursor string, limit int) (chat.MaterialPage, error) {
	page := chat.MaterialPage{Items: []chat.Material{}}
	before, offset, err := parseMaterialCursor(conferenceID, cursor)
	if err != nil {
		return page, err
	}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := authorizeChat(tx, userID, conferenceID, false, false); err != nil {
			return err
		}
		q := r.messageQuery(tx).Where("chat_messages.conference_id=? AND chat_messages.deleted_at IS NULL", conferenceID)
		switch kind {
		case "image":
			q = q.Where("EXISTS (SELECT 1 FROM chat_attachments a WHERE a.message_id=chat_messages.id AND a.status='attached' AND a.mime_type LIKE 'image/%')")
		case "file":
			q = q.Where("EXISTS (SELECT 1 FROM chat_attachments a WHERE a.message_id=chat_messages.id AND a.status='attached' AND a.mime_type NOT LIKE 'image/%')")
		case "link":
			q = q.Where("chat_messages.text ~* 'https?://[^[:space:]]+'")
		}
		if before > 0 {
			q = q.Where("chat_messages.sequence<=?", before)
		}
		messages := []chat.Message{}
		if err := q.Order("chat_messages.sequence DESC").Limit(limit + 1).Find(&messages).Error; err != nil {
			return err
		}
		if err := r.decorateMessages(tx, messages); err != nil {
			return err
		}
		if err := r.markImportant(tx, userID, conferenceID, messages); err != nil {
			return err
		}
		var lastSequence int64
		lastIndex := 0
		for _, message := range messages {
			items := make([]chat.Material, 0, 5)
			if kind == "link" {
				for _, raw := range chatURL.FindAllString(message.Text, -1) {
					url := strings.TrimRight(raw, ".,;:!?)")
					if url != "" {
						items = append(items, chat.Material{Message: message, URL: url})
					}
				}
			} else {
				for _, attachment := range message.Attachments {
					isImage := strings.HasPrefix(attachment.MimeType, "image/")
					if (kind == "image") == isImage {
						file := attachment
						items = append(items, chat.Material{Message: message, Attachment: &file})
					}
				}
			}
			start := 0
			if before == message.Sequence {
				start = offset
			}
			for index := start; index < len(items); index++ {
				if len(page.Items) == limit {
					page.NextCursor = materialCursor(conferenceID, lastSequence, lastIndex)
					return nil
				}
				page.Items = append(page.Items, items[index])
				lastSequence, lastIndex = message.Sequence, index+1
			}
		}
		return nil
	})
	return page, err
}

func (r *ChatRepository) ChatPins(ctx context.Context, userID, conferenceID, cursor string, limit int) (chat.MessagePage, error) {
	page := chat.MessagePage{Items: []chat.Message{}}
	before, err := parseChatCursor(conferenceID, cursor)
	if err != nil {
		return page, err
	}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := authorizeChat(tx, userID, conferenceID, false, false); err != nil {
			return err
		}
		q := r.messageQuery(tx).Joins("JOIN conference_chat_pins pins ON pins.conference_id=chat_messages.conference_id AND pins.message_id=chat_messages.id AND pins.user_id=?", userID).
			Where("chat_messages.conference_id=? AND chat_messages.deleted_at IS NULL", conferenceID)
		if before > 0 {
			q = q.Where("chat_messages.sequence<?", before)
		}
		if err := q.Order("chat_messages.sequence DESC").Limit(limit + 1).Find(&page.Items).Error; err != nil {
			return err
		}
		if len(page.Items) > limit {
			page.Items = page.Items[:limit]
			page.NextCursor = chatCursor(conferenceID, page.Items[len(page.Items)-1].Sequence)
		}
		if err := r.decorateMessages(tx, page.Items); err != nil {
			return err
		}
		return r.markImportant(tx, userID, conferenceID, page.Items)
	})
	return page, err
}

func (r *ChatRepository) SetChatPin(ctx context.Context, userID, conferenceID, messageID string, important bool) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := authorizeChat(tx, userID, conferenceID, true, false); err != nil {
			return err
		}
		if important {
			var count int64
			if err := tx.Table("chat_messages").Where("conference_id=? AND id=? AND deleted_at IS NULL", conferenceID, messageID).Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				return apperrors.ErrNotFound
			}
			return tx.Exec(`INSERT INTO conference_chat_pins(conference_id,user_id,message_id) VALUES(?::uuid,?::uuid,?::uuid) ON CONFLICT DO NOTHING`, conferenceID, userID, messageID).Error
		}
		return tx.Exec("DELETE FROM conference_chat_pins WHERE conference_id=? AND user_id=? AND message_id=?", conferenceID, userID, messageID).Error
	})
}
