package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/personal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Every mutation takes this row lock before a fresh membership/role query.
func lockGroup(tx *gorm.DB, actor, id string) (personal.Role, error) {
	var row struct {
		ID, Type  string
		DeletedAt *string
	}
	err := tx.Table("conversations").Select("id,type,deleted_at::text AS deleted_at").Where("id=?", id).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", apperrors.ErrForbidden
	}
	if err != nil {
		return "", err
	}
	if row.Type != "group" || row.DeletedAt != nil {
		return "", apperrors.ErrForbidden
	}
	var member struct{ Role personal.Role }
	err = tx.Table("conversation_members m").Select("m.role").Joins("JOIN users u ON u.id=m.user_id").Where("m.conversation_id=? AND m.user_id=? AND m.left_at IS NULL AND u.guest_conference_id IS NULL", id, actor).Take(&member).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", apperrors.ErrForbidden
	}
	return member.Role, err
}
func groupManager(role personal.Role) bool { return role == personal.Owner || role == personal.Admin }
func groupTouch(tx *gorm.DB, id string) error {
	return tx.Table("conversations").Where("id=?", id).Updates(map[string]any{"updated_at": gorm.Expr("clock_timestamp()"), "metadata_version": gorm.Expr("metadata_version+1")}).Error
}

func groupWriteView(tx *gorm.DB, actor, id string, item *personal.Conversation) error {
	view, err := personalView(tx, actor, id)
	if err == nil {
		*item = view
	}
	return err
}
func registeredGroupUsers(tx *gorm.DB, ids []string) error {
	var count int64
	if err := tx.Table("users").Where("id IN ? AND guest_conference_id IS NULL", ids).Count(&count).Error; err != nil {
		return err
	}
	if count != int64(len(ids)) {
		return apperrors.New(apperrors.ErrNotFound, "registered account is unavailable")
	}
	return nil
}

func (r *PersonalRepository) CreateGroup(ctx context.Context, actor string, request personal.CreateGroupRequest) (personal.Conversation, bool, error) {
	request, fingerprint, err := personal.NormalizeCreateGroup(actor, request)
	if err != nil {
		return personal.Conversation{}, false, err
	}
	id := uuid.NewString()
	created := false
	var item personal.Conversation
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		ids := append([]string{actor}, request.MemberIDs...)
		if err := registeredGroupUsers(tx, ids); err != nil {
			return err
		}
		result := tx.Exec(`INSERT INTO conversations(id,type,name,description,created_by,group_create_request_id,group_request_fingerprint) VALUES(?,'group',?,?,?,?,?) ON CONFLICT(created_by,group_create_request_id) DO NOTHING`, id, request.Name, request.Description, actor, request.ClientRequestID, fingerprint)
		if result.Error != nil {
			return result.Error
		}
		created = result.RowsAffected == 1
		if !created {
			var prior struct{ ID, GroupRequestFingerprint string }
			if err := tx.Table("conversations").Select("id,group_request_fingerprint").Where("created_by=? AND group_create_request_id=?", actor, request.ClientRequestID).Take(&prior).Error; err != nil {
				return err
			}
			if prior.GroupRequestFingerprint != fingerprint {
				return apperrors.New(apperrors.ErrConflict, "clientRequestId was used for a different group")
			}
			id = prior.ID
			return groupWriteView(tx, actor, id, &item)
		}
		for _, user := range ids {
			role := personal.MemberRole
			if user == actor {
				role = personal.Owner
			}
			if err := tx.Exec(`INSERT INTO conversation_members(conversation_id,user_id,role) VALUES(?,?,?)`, id, user, role).Error; err != nil {
				return err
			}
		}
		return groupWriteView(tx, actor, id, &item)
	})
	if err != nil {
		return personal.Conversation{}, false, err
	}
	return item, created, nil
}

func (r *PersonalRepository) UpdateGroup(ctx context.Context, actor, id string, request personal.UpdateGroupRequest) (personal.Conversation, error) {
	request, err := personal.NormalizeUpdateGroup(request)
	if err != nil {
		return personal.Conversation{}, err
	}
	var item personal.Conversation
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		role, err := lockGroup(tx, actor, id)
		if err != nil {
			return err
		}
		if !groupManager(role) {
			return apperrors.ErrForbidden
		}
		updates := map[string]any{"updated_at": gorm.Expr("clock_timestamp()"), "metadata_version": gorm.Expr("metadata_version+1")}
		if request.Name != nil {
			updates["name"] = *request.Name
		}
		if request.Description != nil {
			updates["description"] = *request.Description
		}
		if err = tx.Table("conversations").Where("id=?", id).Updates(updates).Error; err != nil {
			return err
		}
		return groupWriteView(tx, actor, id, &item)
	})
	if err != nil {
		return personal.Conversation{}, err
	}
	return item, nil
}

func (r *PersonalRepository) GroupMembers(ctx context.Context, actor, id string) ([]personal.Member, error) {
	items := []personal.Member{}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := lockGroup(tx, actor, id); err != nil {
			return err
		}
		return tx.Table("conversation_members m").Select("m.user_id AS id,COALESCE(NULLIF(u.display_name,''),'Пользователь') AS display_name,m.role").Joins("JOIN users u ON u.id=m.user_id").Where("m.conversation_id=? AND m.left_at IS NULL", id).Order("CASE m.role WHEN 'owner' THEN 0 WHEN 'admin' THEN 1 ELSE 2 END,u.display_name,m.user_id").Limit(personal.MaxGroupMembers).Scan(&items).Error
	})
	return items, err
}

func (r *PersonalRepository) AddGroupMembers(ctx context.Context, actor, id string, rawIDs []string) (personal.Conversation, []personal.Member, error) {
	ids, err := personal.NormalizeMemberIDs(rawIDs)
	if err != nil || len(ids) == 0 {
		return personal.Conversation{}, nil, apperrors.ErrInvalidInput
	}
	added := []personal.Member{}
	var item personal.Conversation
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		role, err := lockGroup(tx, actor, id)
		if err != nil {
			return err
		}
		if !groupManager(role) {
			return apperrors.ErrForbidden
		}
		if err = registeredGroupUsers(tx, ids); err != nil {
			return err
		}
		var active []string
		if err = tx.Table("conversation_members").Where("conversation_id=? AND left_at IS NULL", id).Pluck("user_id", &active).Error; err != nil {
			return err
		}
		seen := map[string]bool{}
		for _, user := range active {
			seen[user] = true
		}
		needed := 0
		for _, user := range ids {
			if !seen[user] {
				needed++
			}
		}
		if len(active)+needed > personal.MaxGroupMembers {
			return apperrors.New(apperrors.ErrConflict, "group member limit is 100")
		}
		for _, user := range ids {
			if seen[user] {
				continue
			}
			// New members start at the current history frontier. Returning members retain their read cursor.
			err = tx.Exec(`INSERT INTO conversation_members(conversation_id,user_id,role,last_read_message_id,last_read_sequence)
			 SELECT ?,?,'member',lm.id,COALESCE(lm.sequence,0) FROM (SELECT 1) seed
			 LEFT JOIN LATERAL(SELECT id,sequence FROM conversation_messages WHERE conversation_id=? ORDER BY sequence DESC LIMIT 1) lm ON true
			 ON CONFLICT(conversation_id,user_id) DO UPDATE SET role='member',left_at=NULL,joined_at=clock_timestamp(),updated_at=clock_timestamp()`, id, user, id).Error
			if err != nil {
				return err
			}
			added = append(added, personal.Member{ID: user, Role: personal.MemberRole})
		}
		if len(added) == 0 {
			return groupWriteView(tx, actor, id, &item)
		}
		if err = groupTouch(tx, id); err != nil {
			return err
		}
		return groupWriteView(tx, actor, id, &item)
	})
	if err != nil {
		return personal.Conversation{}, nil, err
	}
	return item, added, nil
}

func (r *PersonalRepository) RemoveGroupMember(ctx context.Context, actor, id, target string) (personal.Conversation, bool, error) {
	changed := false
	var item personal.Conversation
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		role, err := lockGroup(tx, actor, id)
		if err != nil {
			return err
		}
		if !groupManager(role) {
			return apperrors.ErrForbidden
		}
		if actor == target {
			return apperrors.New(apperrors.ErrConflict, "use leave or transfer ownership")
		}
		var member struct {
			Role   personal.Role
			LeftAt *string
		}
		err = tx.Table("conversation_members").Select("role,left_at::text AS left_at").Where("conversation_id=? AND user_id=?", id, target).Take(&member).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperrors.ErrNotFound
		}
		if err != nil {
			return err
		}
		if member.LeftAt != nil {
			return groupWriteView(tx, actor, id, &item)
		}
		if member.Role == personal.Owner || (role == personal.Admin && member.Role != personal.MemberRole) {
			return apperrors.ErrForbidden
		}
		if err = tx.Table("conversation_members").Where("conversation_id=? AND user_id=?", id, target).Updates(map[string]any{"left_at": gorm.Expr("clock_timestamp()"), "updated_at": gorm.Expr("clock_timestamp()")}).Error; err != nil {
			return err
		}
		changed = true
		if err = groupTouch(tx, id); err != nil {
			return err
		}
		return groupWriteView(tx, actor, id, &item)
	})
	if err != nil {
		return personal.Conversation{}, false, err
	}
	return item, changed, nil
}

func (r *PersonalRepository) ChangeGroupRole(ctx context.Context, actor, id, target string, newRole personal.Role) (personal.Conversation, error) {
	if newRole != personal.Admin && newRole != personal.MemberRole {
		return personal.Conversation{}, apperrors.ErrInvalidInput
	}
	var item personal.Conversation
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		role, err := lockGroup(tx, actor, id)
		if err != nil {
			return err
		}
		if role != personal.Owner || actor == target {
			return apperrors.ErrForbidden
		}
		var member struct{ Role personal.Role }
		err = tx.Table("conversation_members").Select("role").Where("conversation_id=? AND user_id=? AND left_at IS NULL", id, target).Take(&member).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperrors.ErrNotFound
		}
		if err != nil {
			return err
		}
		if member.Role == personal.Owner {
			return apperrors.ErrForbidden
		}
		if err = tx.Table("conversation_members").Where("conversation_id=? AND user_id=?", id, target).Updates(map[string]any{"role": newRole, "updated_at": gorm.Expr("clock_timestamp()")}).Error; err != nil {
			return err
		}
		if err = groupTouch(tx, id); err != nil {
			return err
		}
		return groupWriteView(tx, actor, id, &item)
	})
	if err != nil {
		return personal.Conversation{}, err
	}
	return item, nil
}

func (r *PersonalRepository) TransferGroupOwnership(ctx context.Context, actor, id, target string) (personal.Conversation, error) {
	var item personal.Conversation
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		role, err := lockGroup(tx, actor, id)
		if err != nil {
			return err
		}
		if role != personal.Owner || actor == target {
			return apperrors.ErrForbidden
		}
		var count int64
		if err = tx.Table("conversation_members").Where("conversation_id=? AND user_id=? AND left_at IS NULL", id, target).Count(&count).Error; err != nil {
			return err
		}
		if count != 1 {
			return apperrors.ErrNotFound
		}
		if err = tx.Table("conversation_members").Where("conversation_id=? AND user_id=?", id, actor).Updates(map[string]any{"role": personal.Admin, "updated_at": gorm.Expr("clock_timestamp()")}).Error; err != nil {
			return err
		}
		if err = tx.Table("conversation_members").Where("conversation_id=? AND user_id=?", id, target).Updates(map[string]any{"role": personal.Owner, "updated_at": gorm.Expr("clock_timestamp()")}).Error; err != nil {
			return err
		}
		if err = groupTouch(tx, id); err != nil {
			return err
		}
		return groupWriteView(tx, actor, id, &item)
	})
	if err != nil {
		return personal.Conversation{}, err
	}
	return item, nil
}

func (r *PersonalRepository) LeaveGroup(ctx context.Context, actor, id string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		role, err := lockGroup(tx, actor, id)
		if err != nil {
			return err
		}
		if role == personal.Owner {
			var count int64
			if err = tx.Table("conversation_members").Where("conversation_id=? AND left_at IS NULL", id).Count(&count).Error; err != nil {
				return err
			}
			if count > 1 {
				return apperrors.New(apperrors.ErrConflict, "transfer ownership before leaving")
			}
			if err = tx.Table("conversations").Where("id=?", id).Update("deleted_at", gorm.Expr("clock_timestamp()")).Error; err != nil {
				return err
			}
		}
		if err = tx.Table("conversation_members").Where("conversation_id=? AND user_id=?", id, actor).Updates(map[string]any{"left_at": gorm.Expr("clock_timestamp()"), "updated_at": gorm.Expr("clock_timestamp()")}).Error; err != nil {
			return err
		}
		return groupTouch(tx, id)
	})
}

func (r *PersonalRepository) DeleteGroup(ctx context.Context, actor, id string) ([]string, error) {
	former := []string{}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		role, err := lockGroup(tx, actor, id)
		if err != nil {
			return err
		}
		if role != personal.Owner {
			return apperrors.ErrForbidden
		}
		if err = tx.Table("conversation_members").Where("conversation_id=? AND left_at IS NULL", id).Order("user_id").Pluck("user_id", &former).Error; err != nil {
			return err
		}
		if err = tx.Table("conversations").Where("id=?", id).Updates(map[string]any{"deleted_at": gorm.Expr("clock_timestamp()"), "updated_at": gorm.Expr("clock_timestamp()"), "metadata_version": gorm.Expr("metadata_version+1")}).Error; err != nil {
			return err
		}
		return tx.Table("conversation_members").Where("conversation_id=? AND left_at IS NULL", id).Updates(map[string]any{"left_at": gorm.Expr("clock_timestamp()"), "updated_at": gorm.Expr("clock_timestamp()")}).Error
	})
	return former, err
}
