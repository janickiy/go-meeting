package postgres

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/personal"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"strings"
	"time"
	"unicode/utf8"
)

type PersonalRepository struct{ db *gorm.DB }

func NewPersonalRepository(db *gorm.DB) *PersonalRepository {
	return &PersonalRepository{db: db.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})}
}
func (r *PersonalRepository) Account(ctx context.Context, user string) error {
	var n int64
	err := r.db.WithContext(ctx).Table("users").Where("id=? AND guest_conference_id IS NULL", user).Count(&n).Error
	if err != nil {
		return err
	}
	if n != 1 {
		return apperrors.ErrForbidden
	}
	return nil
}
func (r *PersonalRepository) GetOrCreate(ctx context.Context, user, target string) (personal.Conversation, bool, error) {
	if user == target {
		return personal.Conversation{}, false, apperrors.New(apperrors.ErrInvalidInput, "self messaging is unavailable")
	}
	lo, hi := user, target
	if lo > hi {
		lo, hi = hi, lo
	}
	id := uuid.NewString()
	created := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var n int64
		if err := tx.Table("users").Where("id IN ? AND guest_conference_id IS NULL", []string{lo, hi}).Count(&n).Error; err != nil {
			return err
		}
		if n != 2 {
			return apperrors.ErrNotFound
		}
		result := tx.Exec("INSERT INTO conversations(id,user_low_id,user_high_id) VALUES(?,?,?) ON CONFLICT(user_low_id,user_high_id) DO NOTHING", id, lo, hi)
		if result.Error != nil {
			return result.Error
		}
		created = result.RowsAffected == 1
		if !created {
			var row struct{ ID string }
			if err := tx.Table("conversations").Select("id").Where("user_low_id=? AND user_high_id=?", lo, hi).Take(&row).Error; err != nil {
				return err
			}
			id = row.ID
			return nil
		}
		return tx.Exec("INSERT INTO conversation_members(conversation_id,user_id) VALUES(?,?),(?,?)", id, lo, id, hi).Error
	})
	if err != nil {
		return personal.Conversation{}, false, err
	}
	item, err := r.Get(ctx, user, id)
	return item, created, err
}

// A bounded page uses one projection query (including unread), rather than a query per peer.
const personalProjection = `SELECT c.id,c.type,c.created_at,c.last_message_at,c.last_message_id,
 COALESCE(c.last_message_at,c.created_at) AS activity_at,u.id AS peer_id,
 COALESCE(NULLIF(u.display_name,''),'Пользователь') AS peer_name,
 CASE WHEN lm.deleted_at IS NOT NULL THEN 'Сообщение удалено' ELSE COALESCE(NULLIF(left(lm.text,160),''),CASE WHEN lm.id IS NULL THEN '' ELSE 'Вложение' END) END AS preview,
 (SELECT count(*) FROM conversation_messages x WHERE x.conversation_id=c.id AND x.sequence>m.last_read_sequence AND x.sender_user_id<>m.user_id AND x.deleted_at IS NULL) AS unread_count
 FROM conversation_members m JOIN conversations c ON c.id=m.conversation_id
 JOIN users u ON u.id=CASE WHEN c.user_low_id=m.user_id THEN c.user_high_id ELSE c.user_low_id END
 LEFT JOIN conversation_messages lm ON lm.id=c.last_message_id WHERE m.user_id=?`

type personalRow struct {
	ID, Type, PeerID, PeerName, Preview string
	CreatedAt, ActivityAt               time.Time
	LastMessageAt                       *time.Time
	LastMessageID                       *string
	UnreadCount                         int64
}

func (row personalRow) view() personal.Conversation {
	return personal.Conversation{ID: row.ID, Type: row.Type, Peer: personal.Peer{ID: row.PeerID, DisplayName: row.PeerName}, CreatedAt: row.CreatedAt, ActivityAt: row.ActivityAt, LastMessageAt: row.LastMessageAt, LastMessageID: row.LastMessageID, Preview: row.Preview, UnreadCount: row.UnreadCount}
}
func (r *PersonalRepository) Get(ctx context.Context, user, id string) (personal.Conversation, error) {
	var row personalRow
	result := r.db.WithContext(ctx).Raw(personalProjection+" AND c.id=?", user, id).Scan(&row)
	if result.Error != nil {
		return personal.Conversation{}, result.Error
	}
	if result.RowsAffected != 1 {
		return personal.Conversation{}, apperrors.ErrForbidden
	}
	return row.view(), nil
}

type personalCursor struct {
	User string
	At   time.Time
	ID   string
}

func (r *PersonalRepository) List(ctx context.Context, user, cursor string, limit int) (personal.Page, error) {
	page := personal.Page{Items: []personal.Conversation{}}
	if limit < 1 || limit > 100 {
		return page, apperrors.ErrInvalidInput
	}
	query := personalProjection
	args := []any{user}
	if cursor != "" {
		var c personalCursor
		raw, err := base64.RawURLEncoding.DecodeString(cursor)
		if len(cursor) > 512 || err != nil || json.Unmarshal(raw, &c) != nil || c.User != user || c.At.IsZero() {
			return page, apperrors.ErrInvalidInput
		}
		if _, err := uuid.Parse(c.ID); err != nil {
			return page, apperrors.ErrInvalidInput
		}
		query += " AND (COALESCE(c.last_message_at,c.created_at),c.id)<(?,?)"
		args = append(args, c.At, c.ID)
	}
	query += " ORDER BY COALESCE(c.last_message_at,c.created_at) DESC,c.id DESC LIMIT ?"
	args = append(args, limit+1)
	var rows []personalRow
	if err := r.db.WithContext(ctx).Raw(query, args...).Scan(&rows).Error; err != nil {
		return page, err
	}
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		raw, _ := json.Marshal(personalCursor{User: user, At: last.ActivityAt, ID: last.ID})
		page.NextCursor = base64.RawURLEncoding.EncodeToString(raw)
	}
	for _, row := range rows {
		page.Items = append(page.Items, row.view())
	}
	err := r.db.WithContext(ctx).Raw(`SELECT count(*) FROM conversation_members m JOIN conversation_messages x ON x.conversation_id=m.conversation_id AND x.sequence>m.last_read_sequence AND x.sender_user_id<>m.user_id AND x.deleted_at IS NULL WHERE m.user_id=?`, user).Scan(&page.UnreadCount).Error
	return page, err
}
func (r *PersonalRepository) Members(ctx context.Context, id string) ([]string, error) {
	var ids []string
	err := r.db.WithContext(ctx).Table("conversation_members").Where("conversation_id=?", id).Order("user_id").Pluck("user_id", &ids).Error
	return ids, err
}
func (r *PersonalRepository) Search(ctx context.Context, user, query string) ([]personal.Peer, error) {
	query = strings.TrimSpace(query)
	if !utf8.ValidString(query) || utf8.RuneCountInString(query) < 2 || utf8.RuneCountInString(query) > 100 || strings.ContainsRune(query, 0) {
		return nil, apperrors.New(apperrors.ErrInvalidInput, "search requires 2 to 100 characters")
	}
	query = strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(query)
	peers := []personal.Peer{}
	err := r.db.WithContext(ctx).Table("users").Select("id,display_name").Where("id<>? AND guest_conference_id IS NULL AND display_name ILIKE ?", user, "%"+query+"%").Order("display_name,id").Limit(20).Scan(&peers).Error
	return peers, err
}
