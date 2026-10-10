package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/jobs"
	"gorm.io/gorm"
)

// JobRepository хранит независимую продуктовую очередь; медиакоманды RabbitMQ не изменяются.
type JobRepository struct{ db *gorm.DB }

// NewJobRepository связывает очередь с существующим ограниченным пулом PostgreSQL.
// @args db — подключение к базе с настроенными таймаутами.
// @return репозиторий фоновых заданий.
func NewJobRepository(db *gorm.DB) *JobRepository { return &JobRepository{db: db} }

// Claim атомарно получает одно задание, не ожидая блокировок другого работника.
// Последний просроченный захват допускает фиксацию окончательного отказа, но не вызов провайдера.
// @args ctx — предел SQL; kind — фиксированная категория; lease — срок владения.
// @return задание, признак наличия работы и ошибка базы.
func (r *JobRepository) Claim(ctx context.Context, kind string, lease time.Duration) (jobs.Job, bool, error) {
	var job jobs.Job
	result := r.db.WithContext(ctx).Raw(`UPDATE background_jobs SET state='processing',attempts=attempts+1,
        lease_token=?::uuid,lease_until=clock_timestamp()+(? * interval '1 millisecond'),updated_at=clock_timestamp()
        WHERE id=(SELECT id FROM background_jobs WHERE kind=? AND available_at<=clock_timestamp()
          AND (state='queued' OR (state='processing' AND lease_until<clock_timestamp()))
          ORDER BY available_at,id LIMIT 1 FOR UPDATE SKIP LOCKED) RETURNING *`, uuid.NewString(), lease.Milliseconds(), kind).Scan(&job)
	return job, result.RowsAffected != 0, result.Error
}

// Finish фиксирует результат с проверкой актуальности UUID аренды и её срока.
// @args ctx — срок записи; job — захваченная версия; state/code — безопасный исход;
// retryAt — время следующей попытки либо nil для окончательного завершения.
// @return ошибка SQL или ErrLeaseLost, если владение уже прекратилось.
func (r *JobRepository) Finish(ctx context.Context, job jobs.Job, state, code string, retryAt *time.Time) error {
	query := `UPDATE background_jobs SET state=?,error_code=NULLIF(?,''),updated_at=clock_timestamp(),
        lease_token=NULL,lease_until=NULL,finished_at=clock_timestamp()
        WHERE id=?::uuid AND state='processing' AND lease_token=?::uuid AND lease_until>clock_timestamp()`
	args := []any{state, code, job.ID, job.LeaseToken}
	if retryAt != nil {
		query = `UPDATE background_jobs SET state='queued',error_code=NULLIF(?,''),available_at=?,updated_at=clock_timestamp(),
            lease_token=NULL,lease_until=NULL,finished_at=NULL
            WHERE id=?::uuid AND state='processing' AND lease_token=?::uuid AND lease_until>clock_timestamp()`
		args = []any{code, retryAt.UTC(), job.ID, job.LeaseToken}
	}
	result := r.db.WithContext(ctx).Exec(query, args...)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return jobs.ErrLeaseLost
	}
	return nil
}

// Counts читает размер очереди по ограниченному набору категорий без содержимого заданий.
// @args ctx — общий срок SQL-запроса.
// @return агрегаты очереди и ошибка базы.
func (r *JobRepository) Counts(ctx context.Context) ([]jobs.Count, error) {
	items := []jobs.Count{}
	err := r.db.WithContext(ctx).Raw(`SELECT kind,state,count(*) AS count FROM background_jobs
        WHERE state IN ('queued','processing','failed') GROUP BY kind,state`).Scan(&items).Error
	return items, err
}
