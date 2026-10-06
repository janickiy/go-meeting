package postgres

import (
	"context"

	"gorm.io/gorm"
)

// WithConversationDelivery holds a shared conversation lock through a bounded
// write. It uses the same lock order as group mutations and a fresh membership
// statement after the lock, preventing stale queued delivery after revocation.
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
