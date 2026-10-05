package postgres

import (
	"context"
	"errors"

	"github.com/janickiy/go-recorder/internal/domain/records"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// The week starts at completion, including processing/upload time. Historical
// terminal records without ended_at use their last recorded terminal timestamp.
const expiredRecording = `status IN ('ready','partial_ready','failed','cancelled')
	AND COALESCE(ended_at,stopped_at,updated_at,created_at) <= clock_timestamp() - INTERVAL '7 days'`

func availableRecordings(db *gorm.DB) *gorm.DB {
	return db.Where("deleted_at IS NULL AND NOT (" + expiredRecording + ")")
}

// ExpireNextRecording holds a row lock through idempotent object removal. The
// deletion marker and artifact references commit only after storage succeeds.
// A process crash or partial storage failure leaves the record retryable.
func (r *RecordRepository) ExpireNextRecording(ctx context.Context, attempted []int64, remove func(context.Context, records.Record) error) (int64, error) {
	var id int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var record records.Record
		query := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("deleted_at IS NULL AND (" + expiredRecording + ")").
			Where("recorder_lease_until IS NULL OR recorder_lease_until <= clock_timestamp()")
		if len(attempted) > 0 {
			query = query.Where("id NOT IN ?", attempted)
		}
		err := query.Order("COALESCE(ended_at,stopped_at,updated_at,created_at), id").Take(&record).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		id = record.ID
		if err := remove(ctx, record); err != nil {
			return err
		}
		if err := tx.Where("record_id = ?", record.ID).Delete(&records.RecordFile{}).Error; err != nil {
			return err
		}
		if err := tx.Where("record_id = ?", record.ID).Delete(&records.RecordSegment{}).Error; err != nil {
			return err
		}
		if err := tx.Model(&records.Record{}).Where("id = ?", record.ID).Updates(map[string]any{
			"deleted_at": gorm.Expr("clock_timestamp()"), "storage_object_key": nil,
			"preview_object_key": nil, "size_bytes": nil,
		}).Error; err != nil {
			return err
		}
		return nil
	})
	return id, err
}
