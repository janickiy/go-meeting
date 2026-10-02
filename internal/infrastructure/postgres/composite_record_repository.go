package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/janickiy/go-recorder/internal/domain/records"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ListActiveComposite возвращает активные задачи общей записи для восстановления обработки.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//
// @return:
//   - результат 1 ([]records.Record): собранные элементы результата; состав ограничивается параметрами операции.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r *RecordRepository) ListActiveComposite(ctx context.Context) ([]records.Record, error) {
	var result []records.Record
	err := r.db.WithContext(ctx).Where("mode IN ('composite','audio_only','individual_tracks','screen_focus') AND status IN ?", []string{records.StatusStarting, records.StatusRecording, records.StatusStopping, records.StatusFinalizing, records.StatusUploading}).Order("created_at ASC").Limit(100).Find(&result).Error
	return result, err
}

// ClaimComposite захватывает версионную аренду задачи общей записи за конкретным воркером.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - id (string): идентификатор обрабатываемого ресурса.
//   - token (string): подписанный токен или токен владения, который необходимо проверить.
//   - workerID (string): идентификатор воркера-владельца операции.
//   - ttl (time.Duration): срок жизни сохраняемого значения или выданного разрешения.
//
// @return:
//   - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r *RecordRepository) ClaimComposite(ctx context.Context, id, token, workerID string, ttl time.Duration) (bool, error) {
	result := r.db.WithContext(ctx).Model(&records.Record{}).
		Where("uuid = ? AND mode IN ('composite','audio_only','individual_tracks','screen_focus') AND status IN ?", id, []string{records.StatusStarting, records.StatusRecording, records.StatusStopping, records.StatusFinalizing, records.StatusUploading}).
		Where("recorder_token IS NULL OR recorder_lease_until < clock_timestamp()").
		Updates(map[string]any{"recorder_token": token, "recorder_lease_until": gorm.Expr("clock_timestamp() + (? * interval '1 millisecond')", ttl.Milliseconds()), "worker_id": workerID})
	return result.RowsAffected == 1, result.Error
}

// RenewComposite продлевает аренду общей записи при совпадении владельца и версии.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - id (string): идентификатор обрабатываемого ресурса.
//   - token (string): подписанный токен или токен владения, который необходимо проверить.
//   - ttl (time.Duration): срок жизни сохраняемого значения или выданного разрешения.
//
// @return:
//   - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r *RecordRepository) RenewComposite(ctx context.Context, id, token string, ttl time.Duration) (bool, error) {
	result := r.db.WithContext(ctx).Model(&records.Record{}).
		Where("uuid = ? AND mode IN ('composite','audio_only','individual_tracks','screen_focus') AND recorder_token = ? AND recorder_lease_until > clock_timestamp() AND status IN ?", id, token, []string{records.StatusStarting, records.StatusRecording, records.StatusStopping, records.StatusFinalizing, records.StatusUploading}).
		Update("recorder_lease_until", gorm.Expr("clock_timestamp() + (? * interval '1 millisecond')", ttl.Milliseconds()))
	return result.RowsAffected == 1, result.Error
}

// ReleaseComposite освобождает только действующую аренду общей записи.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - id (string): идентификатор обрабатываемого ресурса.
//   - token (string): подписанный токен или токен владения, который необходимо проверить.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r *RecordRepository) ReleaseComposite(ctx context.Context, id, token string) error {
	return r.db.WithContext(ctx).Model(&records.Record{}).Where("uuid = ? AND recorder_token = ?", id, token).Updates(map[string]any{"recorder_token": nil, "recorder_lease_until": nil}).Error
}

// TransitionComposite условно меняет состояние общей записи, проверяя владельца и версию аренды.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - id (string): идентификатор обрабатываемого ресурса.
//   - token (string): подписанный токен или токен владения, который необходимо проверить.
//   - status (string): состояние ресурса, ответа или фильтра выборки.
//   - cause (error): значение cause типа error, используемое согласно назначению этой операции.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
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
	result := r.db.WithContext(ctx).Model(&records.Record{}).Where("uuid = ? AND mode IN ('composite','audio_only','individual_tracks','screen_focus') AND recorder_token = ? AND recorder_lease_until > clock_timestamp() AND status IN ?", id, token, from).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return records.ErrRecordStateChanged
	}
	return nil
}

// SaveCompositeArtifacts сохраняет итоговые артефакты общей записи с защитой от устаревшего воркера.
// Операции с базой данных объединяет в транзакцию.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - id (string): идентификатор обрабатываемого ресурса.
//   - token (string): подписанный токен или токен владения, который необходимо проверить.
//   - final (records.RecordFile): значение final типа records.RecordFile, используемое согласно назначению этой операции.
//   - preview (*records.RecordFile): значение preview типа *records.RecordFile, используемое согласно назначению этой операции.
//   - segments ([]records.RecordSegment): доступные сегменты записи для итоговой сборки.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r *RecordRepository) SaveCompositeArtifacts(ctx context.Context, id, token string, final records.RecordFile, preview *records.RecordFile, segments []records.RecordSegment) error {
	return r.db.WithContext(ctx).Transaction( /* Вложенный обработчик выполняет часть операции в текущей транзакции базы данных, сохраняя её общий результат.

		@args
		  - tx (*gorm.DB): подключение или текущая транзакция GORM, задающая контекст доступа к базе.

		@return:
		  - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение. */func(tx *gorm.DB) error {
			var record records.Record
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("uuid = ? AND mode IN ('composite','audio_only','individual_tracks','screen_focus') AND recorder_token = ? AND recorder_lease_until > clock_timestamp() AND status = ?", id, token, records.StatusUploading).First(&record).Error; err != nil {
				if err == gorm.ErrRecordNotFound {
					return records.ErrRecordStateChanged
				}
				return err
			}
			final.RecordID = record.ID
			for i := range final.Related {
				final.Related[i].RecordID = record.ID
				if err := upsertRecordFile(tx, &final.Related[i]); err != nil {
					return err
				}
			}
			if err := upsertRecordFile(tx, &final); err != nil {
				return err
			}
			updates := map[string]any{"status": records.StatusReady, "storage_bucket": final.Bucket, "storage_object_key": final.ObjectKey, "size_bytes": final.SizeBytes, "duration_sec": final.DurationSec, "ended_at": gorm.Expr("clock_timestamp()"), "error_message": nil}
			var metadata struct {
				TimelineOriginNS int64 `json:"timelineOriginNs"`
			}
			if json.Unmarshal(final.MetadataJSON, &metadata) == nil && metadata.TimelineOriginNS > 0 {
				updates["media_started_at"] = time.Unix(0, metadata.TimelineOriginNS).UTC()
			}
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
