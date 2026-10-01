package postgres

import (
	"context"
	"time"

	"github.com/janickiy/go-recorder/internal/domain/records"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r *RecordRepository) ListActiveComposite(ctx context.Context) ([]records.Record, error) {
	var result []records.Record
	err := r.db.WithContext(ctx).Where("mode = 'composite' AND status IN ?", []string{records.StatusStarting, records.StatusRecording, records.StatusStopping, records.StatusFinalizing, records.StatusUploading}).Order("created_at ASC").Limit(100).Find(&result).Error
	return result, err
}

// ClaimComposite uses the database clock and a unique incarnation token. A
// replacement process cannot share ownership with an expired process, and the
// same fence is checked inside the transaction that publishes final artifacts.
func (r *RecordRepository) ClaimComposite(ctx context.Context, id, token, workerID string, ttl time.Duration) (bool, error) {
	result := r.db.WithContext(ctx).Model(&records.Record{}).
		Where("uuid = ? AND mode = 'composite' AND status IN ?", id, []string{records.StatusStarting, records.StatusRecording, records.StatusStopping, records.StatusFinalizing, records.StatusUploading}).
		Where("recorder_token IS NULL OR recorder_lease_until < clock_timestamp()").
		Updates(map[string]any{"recorder_token": token, "recorder_lease_until": gorm.Expr("clock_timestamp() + (? * interval '1 millisecond')", ttl.Milliseconds()), "worker_id": workerID})
	return result.RowsAffected == 1, result.Error
}

func (r *RecordRepository) RenewComposite(ctx context.Context, id, token string, ttl time.Duration) (bool, error) {
	result := r.db.WithContext(ctx).Model(&records.Record{}).
		Where("uuid = ? AND mode = 'composite' AND recorder_token = ? AND recorder_lease_until > clock_timestamp() AND status IN ?", id, token, []string{records.StatusStarting, records.StatusRecording, records.StatusStopping, records.StatusFinalizing, records.StatusUploading}).
		Update("recorder_lease_until", gorm.Expr("clock_timestamp() + (? * interval '1 millisecond')", ttl.Milliseconds()))
	return result.RowsAffected == 1, result.Error
}

func (r *RecordRepository) ReleaseComposite(ctx context.Context, id, token string) error {
	return r.db.WithContext(ctx).Model(&records.Record{}).Where("uuid = ? AND recorder_token = ?", id, token).Updates(map[string]any{"recorder_token": nil, "recorder_lease_until": nil}).Error
}

func (r *RecordRepository) TransitionComposite(ctx context.Context, id, token, status string, cause error) error {
	var from []string
	updates := map[string]any{"status": status}
	switch status {
	case records.StatusRecording:
		from = []string{records.StatusStarting}
		updates["started_at"] = gorm.Expr("clock_timestamp()")
	case records.StatusFinalizing:
		from = []string{records.StatusStopping, records.StatusFinalizing, records.StatusUploading}
	case records.StatusUploading:
		from = []string{records.StatusFinalizing, records.StatusUploading}
	case records.StatusFailed:
		from = []string{records.StatusStarting, records.StatusRecording, records.StatusStopping, records.StatusFinalizing, records.StatusUploading}
		updates["ended_at"] = gorm.Expr("clock_timestamp()")
		if cause != nil {
			updates["error_message"] = cause.Error()
		}
	default:
		return records.ErrRecordStateChanged
	}
	result := r.db.WithContext(ctx).Model(&records.Record{}).Where("uuid = ? AND mode = 'composite' AND recorder_token = ? AND recorder_lease_until > clock_timestamp() AND status IN ?", id, token, from).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return records.ErrRecordStateChanged
	}
	return nil
}

func (r *RecordRepository) SaveCompositeArtifacts(ctx context.Context, id, token string, final records.RecordFile, preview *records.RecordFile, segments []records.RecordSegment) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var record records.Record
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("uuid = ? AND mode = 'composite' AND recorder_token = ? AND recorder_lease_until > clock_timestamp() AND status = ?", id, token, records.StatusUploading).First(&record).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return records.ErrRecordStateChanged
			}
			return err
		}
		final.RecordID = record.ID
		if err := upsertRecordFile(tx, &final); err != nil {
			return err
		}
		updates := map[string]any{"status": records.StatusReady, "storage_bucket": final.Bucket, "storage_object_key": final.ObjectKey, "size_bytes": final.SizeBytes, "duration_sec": final.DurationSec, "ended_at": gorm.Expr("clock_timestamp()"), "error_message": nil}
		if preview != nil {
			preview.RecordID = record.ID
			if err := upsertRecordFile(tx, preview); err != nil {
				return err
			}
			updates["preview_object_key"] = preview.ObjectKey
		}
		for i := range segments {
			segments[i].RecordID = record.ID
			if err := upsertRecordSegment(tx, &segments[i]); err != nil {
				return err
			}
		}
		result := tx.Model(&records.Record{}).Where("id = ? AND recorder_token = ? AND recorder_lease_until > clock_timestamp()", record.ID, token).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return records.ErrRecordStateChanged
		}
		return nil
	})
}
