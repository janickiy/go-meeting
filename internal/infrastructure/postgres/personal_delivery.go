package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/janickiy/go-recorder/internal/domain/personal"

	"gorm.io/gorm"
)

// WithConversationDelivery проверяет членство под общей блокировкой беседы и
// удерживает её до завершения ограниченной отправки. Скрытая личная беседа не
// блокирует служебные события её владельцу: другие вкладки должны очистить кеш.
//
// @args
//   - ctx — контекст операции и срок отправки.
//   - id, user — идентификаторы беседы и получателя.
//   - deliver — ограниченная отправка, выполняемая только для действующего участника.
//
// @return разрешена ли отправка и ошибка проверки либо отправки.
func (r *PersonalRepository) WithConversationDelivery(ctx context.Context, id, user string, deliver func() error) (bool, error) {
	allowed := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row struct{ ID string }
		result := tx.Raw("SELECT id FROM conversations WHERE id=? AND deleted_at IS NULL FOR SHARE", id).Scan(&row)
		if result.Error != nil || row.ID == "" {
			return result.Error
		}
		var count int64
		if err := tx.Raw(`SELECT count(*) FROM conversation_members m
			JOIN users u ON u.id=m.user_id
			WHERE m.conversation_id=? AND m.user_id=? AND m.left_at IS NULL
			AND u.guest_conference_id IS NULL`, id, user).Scan(&count).Error; err != nil {
			return err
		}
		if count != 1 {
			return nil
		}
		allowed = true
		return deliver()
	})
	return allowed, err
}

// WithConversationMessageDelivery повторно проверяет сообщение непосредственно
// перед записью в сокет: удаление чата и очистка истории могли завершиться уже
// после публикации события в Redis. Та же блокировка сериализует отправку с
// изменением настроек, поэтому флаг уведомлений не берётся из устаревшей очереди.
//
// @args
//   - ctx — контекст операции и ограниченный срок отправки.
//   - id, user, messageID — беседа, получатель и сохранённое сообщение.
//   - deliver — отправка с текущим флагом уведомлений и границей очищенной истории.
//
// @return разрешена ли отправка и ошибка проверки либо отправки.
func (r *PersonalRepository) WithConversationMessageDelivery(ctx context.Context, id, user, messageID string, deliver func(bool, int64) error) (bool, error) {
	allowed := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row struct{ ID string }
		result := tx.Raw("SELECT id FROM conversations WHERE id=? AND deleted_at IS NULL FOR SHARE", id).Scan(&row)
		if result.Error != nil || row.ID == "" {
			return result.Error
		}
		var state struct {
			NotificationsEnabled  bool
			HistoryClearedThrough int64
		}
		result = tx.Raw(`SELECT m.notifications_enabled,m.history_cleared_through
			FROM conversation_members m JOIN users u ON u.id=m.user_id
			JOIN conversation_messages x ON x.conversation_id=m.conversation_id
			WHERE m.conversation_id=? AND m.user_id=? AND m.left_at IS NULL
			AND m.hidden_at IS NULL AND u.guest_conference_id IS NULL
			AND x.id=? AND x.sequence>m.history_cleared_through`, id, user, messageID).Scan(&state)
		if result.Error != nil || result.RowsAffected != 1 {
			return result.Error
		}
		allowed = true
		return deliver(state.NotificationsEnabled, state.HistoryClearedThrough)
	})
	return allowed, err
}

// WithDirectActionDelivery обновляет персональную нагрузку перед отправкой,
// потому что события разных HTTP-запросов могут публиковаться не в порядке COMMIT.
// Запоздалое скрытие пропускается, если новое сообщение уже вернуло беседу;
// старые mute/clear события получают текущую проекцию, а не прежнее превью.
//
// @args
//   - ctx — контекст операции и ограниченной записи.
//   - id, user — личная беседа и владелец настройки.
//   - kind — разрешённое событие настройки, очистки или скрытия.
//   - deliver — отправка актуальной проекции; при скрытии получает только ID, тип и границу истории.
//
// @return разрешена ли отправка и ошибка проверки либо отправки.
func (r *PersonalRepository) WithDirectActionDelivery(ctx context.Context, id, user, kind string, deliver func(*personal.Conversation) error) (bool, error) {
	if kind != "conversation.preferences.updated" && kind != "conversation.history.cleared" && kind != "conversation.hidden" {
		return false, errors.New("unsupported personal conversation event")
	}
	allowed := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var conversation struct{ ID string }
		result := tx.Raw("SELECT id FROM conversations WHERE id=? AND type='direct' AND deleted_at IS NULL FOR SHARE", id).Scan(&conversation)
		if result.Error != nil || conversation.ID == "" {
			return result.Error
		}
		var member struct {
			HiddenAt              *time.Time
			HistoryClearedThrough int64
		}
		result = tx.Raw(`SELECT m.hidden_at,m.history_cleared_through FROM conversation_members m JOIN users u ON u.id=m.user_id
			WHERE m.conversation_id=? AND m.user_id=? AND m.left_at IS NULL
			AND u.guest_conference_id IS NULL`, id, user).Scan(&member)
		if result.Error != nil || result.RowsAffected != 1 {
			return result.Error
		}
		if kind == "conversation.hidden" {
			if member.HiddenAt == nil {
				return nil
			}
			allowed = true
			return deliver(&personal.Conversation{ID: id, Type: "direct", HistoryClearedThrough: member.HistoryClearedThrough})
		}
		if member.HiddenAt != nil {
			return nil
		}
		item, err := personalView(tx, user, id)
		if err != nil {
			return err
		}
		allowed = true
		return deliver(&item)
	})
	return allowed, err
}
