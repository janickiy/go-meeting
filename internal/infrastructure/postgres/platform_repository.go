package postgres

import (
	"context"
	"time"

	"github.com/janickiy/go-recorder/internal/domain/platform"
	"gorm.io/gorm"
)

// PlatformRepository читает глобальные полномочия и операционные данные без приватного содержимого.
type PlatformRepository struct{ db *gorm.DB }

func NewPlatformRepository(db *gorm.DB) *PlatformRepository { return &PlatformRepository{db: db} }

// IsAdmin проверяет текущие сохранённые полномочия, поэтому отзыв роли лишает доступа и с действующим JWT.
func (r *PlatformRepository) IsAdmin(ctx context.Context, userID string) (bool, error) {
	var allowed bool
	err := r.db.WithContext(ctx).Raw(`SELECT EXISTS(SELECT 1 FROM users WHERE id = ? AND is_admin)`, userID).Scan(&allowed).Error
	return allowed, err
}

// Summary читает ограниченные агрегаты и недавние безопасные коды сбоев, без полезной нагрузки и содержимого.
func (r *PlatformRepository) Summary(ctx context.Context) (platform.Summary, error) {
	var counts struct {
		ActiveConferences       int64 `gorm:"column:active_conferences"`
		JoinedParticipants      int64 `gorm:"column:joined_participants"`
		ActiveRecordings        int64 `gorm:"column:active_recordings"`
		QueuedJobs              int64 `gorm:"column:queued_jobs"`
		FailedJobs24h           int64 `gorm:"column:failed_jobs24h"`
		FailedRecordings24h     int64 `gorm:"column:failed_recordings24h"`
		FailedTranscriptions24h int64 `gorm:"column:failed_transcriptions24h"`
	}
	err := r.db.WithContext(ctx).Raw(`SELECT
		(SELECT COUNT(*) FROM conferences WHERE status = 'active') AS active_conferences,
		(SELECT COUNT(*) FROM conference_participants p JOIN conferences c ON c.id = p.conference_id WHERE c.status = 'active' AND p.status = 'joined') AS joined_participants,
		(SELECT COUNT(*) FROM record WHERE platform_conference_id IS NOT NULL AND deleted_at IS NULL AND status IN ('starting','recording','degraded','stopping','finalizing','uploading')) AS active_recordings,
		(SELECT COUNT(*) FROM background_jobs WHERE state IN ('queued','processing')) AS queued_jobs,
		(SELECT COUNT(*) FROM background_jobs WHERE state = 'failed' AND updated_at >= NOW() - INTERVAL '24 hours') AS failed_jobs24h,
		(SELECT COUNT(*) FROM record WHERE platform_conference_id IS NOT NULL AND deleted_at IS NULL AND status = 'failed' AND updated_at >= NOW() - INTERVAL '24 hours') AS failed_recordings24h,
		(SELECT COUNT(*) FROM transcripts WHERE status = 'failed' AND updated_at >= NOW() - INTERVAL '24 hours') AS failed_transcriptions24h`).Scan(&counts).Error
	if err != nil {
		return platform.Summary{}, err
	}
	value := platform.Summary{AsOf: time.Now().UTC(), ActiveConferences: counts.ActiveConferences, JoinedParticipants: counts.JoinedParticipants,
		ActiveRecordings: counts.ActiveRecordings, QueuedJobs: counts.QueuedJobs, FailedJobs24h: counts.FailedJobs24h,
		FailedRecordings24h: counts.FailedRecordings24h, FailedTranscriptions24h: counts.FailedTranscriptions24h,
		RecentFailures: []platform.Failure{}}
	var rows []struct {
		Kind string
		Code string
		At   time.Time
	}
	err = r.db.WithContext(ctx).Raw(`SELECT kind, code, at FROM (
		SELECT kind, COALESCE(error_code, 'failed') AS code, updated_at AS at FROM background_jobs WHERE state = 'failed' AND updated_at >= NOW() - INTERVAL '24 hours'
		UNION ALL
		SELECT 'recording' AS kind, 'failed' AS code, updated_at AS at FROM record WHERE platform_conference_id IS NOT NULL AND deleted_at IS NULL AND status = 'failed' AND updated_at >= NOW() - INTERVAL '24 hours'
		UNION ALL
		SELECT 'transcription' AS kind, COALESCE(error_code, 'failed') AS code, updated_at AS at FROM transcripts WHERE status = 'failed' AND updated_at >= NOW() - INTERVAL '24 hours'
	) failures ORDER BY at DESC LIMIT 10`).Scan(&rows).Error
	if err != nil {
		return platform.Summary{}, err
	}
	for _, row := range rows {
		value.RecentFailures = append(value.RecentFailures, platform.Failure{Kind: safeOperationCode(row.Kind), Code: safeOperationCode(row.Code), At: row.At})
	}
	return value, nil
}

// safeOperationCode не позволяет некорректному сохранённому значению стать произвольным сообщением интерфейса.
func safeOperationCode(value string) string {
	if len(value) == 0 || len(value) > 64 {
		return "failed"
	}
	for _, char := range value {
		if char != '.' && char != '_' && char != '-' && (char < 'a' || char > 'z') && (char < '0' || char > '9') {
			return "failed"
		}
	}
	return value
}
