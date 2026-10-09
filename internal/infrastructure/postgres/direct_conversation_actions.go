package postgres

import (
	"context"

	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/personal"
	"gorm.io/gorm"
)

// lockDirectConversation сериализует персональные действия и отправку сообщений.
// @args: tx — транзакция; user — текущий аккаунт; id — идентификатор переписки.
// @return: ошибка доступа, типа переписки или запроса.
func lockDirectConversation(tx *gorm.DB, user, id string) error {
	if err := authorizeDirect(tx, user, id, true); err != nil {
		return err
	}
	var kind string
	if err := tx.Table("conversations").Select("type").Where("id=?", id).Scan(&kind).Error; err != nil {
		return err
	}
	if kind != "direct" {
		return apperrors.ErrForbidden
	}
	return nil
}

// SetDirectPreferences сохраняет уведомления только для текущего собеседника.
// @args: ctx — контекст; user — владелец настройки; id — переписка; enabled — разрешение уведомлений.
// @return: актуальная персональная проекция переписки и ошибка.
func (r *PersonalRepository) SetDirectPreferences(ctx context.Context, user, id string, enabled bool) (personal.Conversation, error) {
	var item personal.Conversation
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockDirectConversation(tx, user, id); err != nil {
			return err
		}
		if err := tx.Table("conversation_members").Where("conversation_id=? AND user_id=?", id, user).
			Updates(map[string]any{"notifications_enabled": enabled, "updated_at": gorm.Expr("clock_timestamp()")}).Error; err != nil {
			return err
		}
		var err error
		item, err = personalView(tx, user, id)
		return err
	})
	return item, err
}

// clearDirectHistory ставит персональную границу истории без удаления общих данных.
// Блокировка переписки гарантирует, что параллельная отправка окажется до или после границы.
// @args: tx — транзакция с блокировкой; user — владелец истории; id — переписка; hide — скрыть также сам чат.
// @return: ошибка сохранения; повторная очистка без новых сообщений безопасна.
func clearDirectHistory(tx *gorm.DB, user, id string, hide bool) error {
	var latest struct {
		ID       string
		Sequence int64
	}
	result := tx.Table("conversation_messages").Select("id,sequence").Where("conversation_id=?", id).
		Order("sequence DESC").Limit(1).Scan(&latest)
	if result.Error != nil {
		return result.Error
	}
	updates := map[string]any{"history_cleared_through": gorm.Expr("GREATEST(history_cleared_through,?)", latest.Sequence), "updated_at": gorm.Expr("clock_timestamp()")}
	if latest.ID != "" {
		updates["last_read_message_id"] = latest.ID
		updates["last_read_sequence"] = latest.Sequence
	}
	if hide {
		updates["hidden_at"] = gorm.Expr("COALESCE(hidden_at,clock_timestamp())")
	}
	return tx.Table("conversation_members").Where("conversation_id=? AND user_id=?", id, user).Updates(updates).Error
}

// ClearDirectHistory очищает видимую историю только у вызвавшего действие аккаунта.
// @args: ctx — контекст; user — текущий аккаунт; id — личная переписка.
// @return: переписка без старых превью и непрочитанных сообщений, либо ошибка.
func (r *PersonalRepository) ClearDirectHistory(ctx context.Context, user, id string) (personal.Conversation, error) {
	var item personal.Conversation
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockDirectConversation(tx, user, id); err != nil {
			return err
		}
		if err := clearDirectHistory(tx, user, id, false); err != nil {
			return err
		}
		var err error
		item, err = personalView(tx, user, id)
		return err
	})
	return item, err
}

// HideDirectConversation убирает чат и прежнюю историю только у текущего аккаунта.
// Новое сообщение или явное создание диалога возвращает чат, но не очищенную историю.
// @args: ctx — контекст; user — текущий аккаунт; id — личная переписка.
// @return: точная персональная граница скрытой истории и ошибка; повторное скрытие идемпотентно.
func (r *PersonalRepository) HideDirectConversation(ctx context.Context, user, id string) (int64, error) {
	var cutoff int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockDirectConversation(tx, user, id); err != nil {
			return err
		}
		if err := clearDirectHistory(tx, user, id, true); err != nil {
			return err
		}
		return tx.Table("conversation_members").Select("history_cleared_through").Where("conversation_id=? AND user_id=?", id, user).Scan(&cutoff).Error
	})
	return cutoff, err
}
