package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/chat"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r *ChatRepository) InitAttachment(ctx context.Context, userID, conferenceID string, request chat.InitRequest) (chat.Attachment, bool, error) {
	var attachment chat.Attachment
	created := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := r.authorize(tx, userID, conferenceID, true, true); err != nil {
			return err
		}
		err := r.attachments(tx).Where(r.sql("conference_id=? AND owner_user_id=? AND client_request_id=?"), conferenceID, userID, request.ClientRequestID).Take(&attachment).Error
		if err == nil {
			if attachment.Filename != request.Filename || attachment.Size != request.Size || attachment.MimeType != request.MimeType {
				return apperrors.New(apperrors.ErrConflict, "clientRequestId was used for a different attachment")
			}
			if r.direct && attachment.Status == "attached" && attachment.MessageID != nil {
				cutoff, err := r.historyCutoff(tx, userID, conferenceID)
				if err != nil {
					return err
				}
				var visible int64
				if err := r.messages(tx).Where("conversation_id=? AND id=? AND sequence>?", conferenceID, *attachment.MessageID, cutoff).Count(&visible).Error; err != nil {
					return err
				}
				if visible != 1 {
					return apperrors.ErrNotFound
				}
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var count int64
		if err := r.attachments(tx).Where(r.sql("conference_id=? AND owner_user_id=? AND status IN ('pending','ready') AND expires_at>clock_timestamp()"), conferenceID, userID).Count(&count).Error; err != nil {
			return err
		}
		if count >= 20 {
			return apperrors.New(apperrors.ErrConflict, "too many pending attachments")
		}
		attachment = chat.Attachment{ID: uuid.NewString(), ConferenceID: conferenceID, OwnerID: userID, ClientRequestID: request.ClientRequestID, Filename: request.Filename, MimeType: request.MimeType, Size: request.Size, Status: "pending", ExpiresAt: time.Now().UTC().Add(chat.AttachmentTTL)}
		created = true
		r.setAttachmentScope(&attachment, conferenceID)
		return r.attachments(tx).Create(&attachment).Error
	})
	return attachment, created, err
}
func (r *ChatRepository) attachmentOwned(tx *gorm.DB, userID, conferenceID, id string, write bool) (chat.Attachment, error) {
	if _, err := r.authorize(tx, userID, conferenceID, true, write); err != nil {
		return chat.Attachment{}, err
	}
	var attachment chat.Attachment
	err := r.attachments(tx).Clauses(clause.Locking{Strength: "UPDATE"}).Where(r.sql("id=? AND conference_id=?"), id, conferenceID).Take(&attachment).Error
	if err != nil {
		return attachment, mapNotFound(err)
	}
	if attachment.OwnerID != userID {
		return attachment, apperrors.ErrForbidden
	}
	return attachment, nil
}
func (r *ChatRepository) ClaimUpload(ctx context.Context, userID, conferenceID, id, token string) (chat.Attachment, error) {
	var attachment chat.Attachment
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		attachment, err = r.attachmentOwned(tx, userID, conferenceID, id, true)
		if err != nil {
			return err
		}
		if attachment.Status != "pending" || !attachment.ExpiresAt.After(time.Now()) {
			return apperrors.New(apperrors.ErrConflict, "attachment upload is no longer pending")
		}
		if attachment.UploadedAt != nil {
			return nil
		}
		if attachment.UploadLeaseUntil != nil && attachment.UploadLeaseUntil.After(time.Now()) {
			return apperrors.New(apperrors.ErrConflict, "attachment upload is already in progress")
		}
		until := time.Now().UTC().Add(chat.UploadLease)
		attachment.UploadToken = &token
		attachment.UploadLeaseUntil = &until
		attachment.ObjectKey = attachment.Prefix() + token
		return r.attachments(tx).Where("id=?", id).Updates(map[string]any{"upload_token": token, "upload_lease_until": until, "object_key": attachment.ObjectKey, "updated_at": gorm.Expr("clock_timestamp()")}).Error
	})
	return attachment, err
}
func (r *ChatRepository) CompleteUpload(ctx context.Context, userID, conferenceID, id, token, checksum string) (chat.Attachment, error) {
	var attachment chat.Attachment
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		attachment, err = r.attachmentOwned(tx, userID, conferenceID, id, true)
		if err != nil {
			return err
		}
		if attachment.Status != "pending" || attachment.UploadToken == nil || *attachment.UploadToken != token || attachment.UploadLeaseUntil == nil || !attachment.UploadLeaseUntil.After(time.Now()) || !attachment.ExpiresAt.After(time.Now()) {
			return apperrors.New(apperrors.ErrConflict, "upload lease expired")
		}
		result := r.attachments(tx).Where("id=? AND upload_token=? AND upload_lease_until>clock_timestamp()", id, token).Updates(map[string]any{"checksum": checksum, "uploaded_at": gorm.Expr("clock_timestamp()"), "upload_lease_until": nil, "updated_at": gorm.Expr("clock_timestamp()")})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return apperrors.ErrConflict
		}
		return r.attachments(tx).Where("id=?", id).Take(&attachment).Error
	})
	return attachment, err
}
func (r *ChatRepository) AttachmentForFinalize(ctx context.Context, userID, conferenceID, id string) (chat.Attachment, error) {
	var a chat.Attachment
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		a, err = r.attachmentOwned(tx, userID, conferenceID, id, true)
		if err != nil {
			return err
		}
		if (a.Status != "pending" && a.Status != "ready") || a.UploadedAt == nil || !a.ExpiresAt.After(time.Now()) {
			return apperrors.New(apperrors.ErrConflict, "attachment is not uploaded or has expired")
		}
		return nil
	})
	return a, err
}
func (r *ChatRepository) FinalizeAttachment(ctx context.Context, userID, conferenceID, id, objectKey string) (chat.Attachment, error) {
	var a chat.Attachment
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		a, err = r.attachmentOwned(tx, userID, conferenceID, id, true)
		if err != nil {
			return err
		}
		if (a.Status != "pending" && a.Status != "ready") || a.UploadedAt == nil || a.ObjectKey != objectKey || !a.ExpiresAt.After(time.Now()) {
			return apperrors.New(apperrors.ErrConflict, "attachment is not uploaded or has expired")
		}
		if a.Status == "ready" {
			return nil
		}
		if err := r.attachments(tx).Where("id=?", id).Updates(map[string]any{"status": "ready", "updated_at": gorm.Expr("clock_timestamp()")}).Error; err != nil {
			return err
		}
		a.Status = "ready"
		return nil
	})
	return a, err
}
func (r *ChatRepository) DownloadAttachment(ctx context.Context, userID, conferenceID, id string) (chat.Attachment, error) {
	var a chat.Attachment
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := r.authorize(tx, userID, conferenceID, false, false); err != nil {
			return err
		}
		if err := r.attachments(tx).Where(r.sql("id=? AND conference_id=?"), id, conferenceID).Take(&a).Error; err != nil {
			return mapNotFound(err)
		}
		if a.Status == "ready" && a.OwnerID == userID && a.ExpiresAt.After(time.Now()) {
			return nil
		}
		if a.Status != "attached" || a.MessageID == nil {
			return apperrors.ErrNotFound
		}
		var count int64
		cutoff, err := r.historyCutoff(tx, userID, conferenceID)
		if err != nil {
			return err
		}
		if err := r.messages(tx).Where(r.sql("id=? AND conference_id=? AND deleted_at IS NULL AND sequence>?"), *a.MessageID, conferenceID, cutoff).Count(&count).Error; err != nil {
			return err
		}
		if count != 1 {
			return apperrors.ErrNotFound
		}
		return nil
	})
	return a, err
}
func (r *ChatRepository) CleanupCandidates(ctx context.Context, limit int) ([]chat.Attachment, error) {
	var rows []chat.Attachment
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := r.attachments(tx).Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).Where("cleaned_at IS NULL AND expires_at<clock_timestamp() AND (upload_lease_until IS NULL OR upload_lease_until<clock_timestamp()-INTERVAL '2 minutes')").Order("expires_at,id").Limit(limit).Find(&rows).Error; err != nil {
			return err
		}
		for i := range rows {
			if rows[i].Status == "pending" || rows[i].Status == "ready" {
				if err := r.attachments(tx).Where("id=?", rows[i].ID).Updates(map[string]any{"status": "expired", "updated_at": gorm.Expr("clock_timestamp()")}).Error; err != nil {
					return err
				}
				rows[i].Status = "expired"
			}
		}
		return nil
	})
	return rows, err
}
func (r *ChatRepository) CompleteCleanup(ctx context.Context, id, key string) error {
	return r.attachments(r.db.WithContext(ctx)).Where("id=? AND object_key=? AND status IN ('expired','attached')", id, key).Update("cleaned_at", gorm.Expr("clock_timestamp()")).Error
}
func (r *ChatRepository) AbortUpload(ctx context.Context, id, token string) error {
	return r.attachments(r.db.WithContext(ctx)).Where("id=? AND upload_token=? AND uploaded_at IS NULL AND status='pending'", id, token).Update("upload_lease_until", nil).Error
}
