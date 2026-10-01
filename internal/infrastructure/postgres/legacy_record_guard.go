package postgres

import (
	"context"

	"github.com/janickiy/go-recorder/internal/domain/apperrors"
)

// Anonymous legacy capture cannot acquire a lock for an authenticated platform
// conference, or bypass the owner-only composite recording workflow.
func (r *RecordRepository) LegacyConferenceAllowed(ctx context.Context, id string) error {
	var count int64
	if err := r.db.WithContext(ctx).Table("conferences").Where("id = ?", id).Count(&count).Error; err != nil {
		return err
	}
	if count != 0 {
		return apperrors.New(apperrors.ErrForbidden, "use authenticated conference recording endpoints")
	}
	return nil
}
