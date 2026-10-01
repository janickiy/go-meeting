package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	domain "github.com/janickiy/go-recorder/internal/domain/notifications"
	"gorm.io/gorm"
)

// NotificationRepository реализует постоянное хранение личных уведомлений пользователя через GORM.
//   - db: подключение или текущая транзакция GORM, задающая контекст доступа к базе.
type NotificationRepository struct{ db *gorm.DB }

// NewNotificationRepository создаёт и связывает зависимости компонента NotificationRepository, используемого в личных уведомлениях и их фоновой доставке.
//
// @parameters:
//   - db (*gorm.DB): подключение или текущая транзакция GORM, задающая контекст доступа к базе.
//
// @return:
//   - результат 1 (*NotificationRepository): созданный компонент с переданными зависимостями.
func NewNotificationRepository(db *gorm.DB) *NotificationRepository {
	return &NotificationRepository{db: db}
}

// List возвращает ограниченный список личных уведомлений пользователя с принятыми в данном слое фильтрами.
//
// @parameters:
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - userID (string): идентификатор пользователя, для которого выполняется операция.
//   - cursor (string): непрозрачная граница продолжения предыдущей страницы.
//   - limit (int): максимальное число элементов страницы или порции обработки.
//
// @return:
//   - результат 1 (domain.Page): страница элементов и метаданные продолжения.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r *NotificationRepository) List(ctx context.Context, userID, cursor string, limit int) (domain.Page, error) {
	page := domain.Page{Items: []domain.Notification{}}
	if limit < 1 || limit > 100 {
		return page, apperrors.ErrInvalidInput
	}
	c, err := domain.DecodeCursor(cursor, userID)
	if err != nil {
		return page, err
	}
	q := r.db.WithContext(ctx).Where("user_id = ?", userID)
	if c != nil {
		q = q.Where("(created_at, id) < (?, ?::uuid)", c.At, c.ID)
	}
	if err := q.Order("created_at DESC, id DESC").Limit(limit + 1).Find(&page.Items).Error; err != nil {
		return page, err
	}
	if len(page.Items) > limit {
		page.Items = page.Items[:limit]
		next := domain.EncodeCursor(page.Items[limit-1])
		page.NextCursor = &next
	}
	err = r.db.WithContext(ctx).Model(&domain.Notification{}).Where("user_id = ? AND read_at IS NULL", userID).Count(&page.UnreadCount).Error
	return page, err
}

// Read читает состояние личных уведомлений пользователя для дальнейшей обработки или ответа.
//
// @parameters:
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - userID (string): идентификатор пользователя, для которого выполняется операция.
//   - id (string): идентификатор обрабатываемого ресурса.
//
// @return:
//   - результат 1 (domain.Notification): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r *NotificationRepository) Read(ctx context.Context, userID, id string) (domain.Notification, error) {
	var item domain.Notification
	result := r.db.WithContext(ctx).Raw("UPDATE notifications SET read_at = COALESCE(read_at, now()) WHERE id = ? AND user_id = ? RETURNING *", id, userID).Scan(&item)
	if result.Error != nil {
		return item, result.Error
	}
	if result.RowsAffected == 0 {
		return item, apperrors.ErrNotFound
	}
	return item, nil
}

// Generate создаёт постоянные уведомления из расписания и транзакционных заданий, ограничивая порцию обработки.
// Операции с базой данных объединяет в транзакцию.
//
// @parameters:
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r *NotificationRepository) Generate(ctx context.Context) error {
	// Timezone-independent epoch-microseconds are part of the soon dedup key.
	soon := `WITH candidates AS (
            SELECT p.user_id, 'soon:' || c.id::text || ':' || (extract(epoch from c.scheduled_at)*1000000)::bigint::text AS dedup_key,
              jsonb_build_object('conferenceId',c.id,'scheduledAt',c.scheduled_at) AS payload
            FROM conferences c JOIN conference_participants p ON p.conference_id=c.id
            WHERE c.status='scheduled' AND c.scheduled_at > now() AND c.scheduled_at <= now()+interval '15 minutes'
              AND p.user_id IS NOT NULL AND p.admission_state IN ('admitted','waiting') AND p.status NOT IN ('kicked','rejected')
              AND NOT EXISTS(SELECT 1 FROM notifications n WHERE n.user_id=p.user_id AND n.dedup_key='soon:' || c.id::text || ':' || (extract(epoch from c.scheduled_at)*1000000)::bigint::text)
            ORDER BY c.scheduled_at,c.id,p.id LIMIT 100)
          INSERT INTO notifications(id,user_id,type,payload,dedup_key) SELECT gen_random_uuid(),user_id,'conference.soon',payload,dedup_key FROM candidates ON CONFLICT(user_id,dedup_key) DO NOTHING`
	if err := r.db.WithContext(ctx).Exec(soon).Error; err != nil {
		return err
	}
	// job задаёт согласованное представление данных «задание» для личных уведомлениях и их фоновой доставке.
	// Состав:
	//   - ID: уникальный идентификатор данной сущности.
	//   - Kind: тип события, ошибки или медиа, определяющий ветку обработки.
	//   - ConferenceID: идентификатор конференции, ограничивающий область операции.
	//   - RecordingID: идентификатор записи конференции.
	//   - CursorParticipantID: идентификатор связанного ресурса, заданного параметром CursorParticipantID.
	//   - CreatedAt: время создания значения.
	type job struct {
		ID                  int64
		Kind                string
		ConferenceID        string
		RecordingID         *string
		CursorParticipantID *string
		CreatedAt           time.Time
	}
	return r.db.WithContext(ctx).Transaction( /* Вложенный обработчик выполняет часть операции в текущей транзакции базы данных, сохраняя её общий результат.

		@parameters:
		  - tx (*gorm.DB): подключение или текущая транзакция GORM, задающая контекст доступа к базе.

		@return:
		  - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение. */func(tx *gorm.DB) error {
			jobs := []job{}
			if err := tx.Raw(`SELECT id,kind,conference_id,recording_id,cursor_participant_id,created_at FROM notification_jobs WHERE processed_at IS NULL ORDER BY available_at,id LIMIT 100 FOR UPDATE SKIP LOCKED`).Scan(&jobs).Error; err != nil {
				return err
			}
			budget := 1000 // Also bound fanout, not only the number of source events.
			for _, item := range jobs {
				if budget == 0 {
					break
				}
				done := true
				updates := map[string]any{}
				if item.Kind == "admission.decided" {
					if err := tx.Exec(`INSERT INTO notifications(id,user_id,type,payload,dedup_key,created_at)
                    SELECT gen_random_uuid(),user_id,kind,jsonb_build_object('conferenceId',conference_id,'admissionState',admission_state),
                      'admission:' || entity_id::text || ':' || entity_version::text,created_at
                    FROM notification_jobs WHERE id=? ON CONFLICT(user_id,dedup_key) DO NOTHING`, item.ID).Error; err != nil {
						return err
					}
					budget--
				} else {
					limit := min(100, budget)
					ids := []string{}
					q := tx.Table("conference_participants p").Joins("JOIN record r ON r.uuid = ? AND r.platform_conference_id = p.conference_id AND r.mode = 'composite' AND r.status IN ('ready','partial_ready') AND r.deleted_at IS NULL", item.RecordingID).
						Where("p.conference_id = ? AND p.user_id IS NOT NULL AND p.admission_state='admitted' AND p.status IN ('joined','left')", item.ConferenceID).
						Where("p.created_at <= ? AND (p.admission_decided_at IS NULL OR p.admission_decided_at <= ?)", item.CreatedAt, item.CreatedAt)
					if item.CursorParticipantID != nil {
						q = q.Where("p.id > ?", *item.CursorParticipantID)
					}
					if err := q.Order("p.id").Limit(limit).Pluck("p.id", &ids).Error; err != nil {
						return err
					}
					if len(ids) > 0 {
						// Recheck live authorization in the insertion statement, including
						// deletion/kick racing the preceding bounded candidate lookup.
						if err := tx.Exec(`INSERT INTO notifications(id,user_id,type,payload,dedup_key,created_at)
                        SELECT gen_random_uuid(),p.user_id,j.kind,jsonb_build_object('conferenceId',j.conference_id,'recordingId',j.recording_id),
                          'recording:' || j.recording_id::text,j.created_at
                        FROM notification_jobs j JOIN record r ON r.uuid=j.recording_id
                        JOIN conference_participants p ON p.conference_id=j.conference_id
                        WHERE j.id=? AND p.id IN ? AND p.user_id IS NOT NULL AND p.admission_state='admitted' AND p.status IN ('joined','left')
                          AND p.created_at <= j.created_at AND (p.admission_decided_at IS NULL OR p.admission_decided_at <= j.created_at)
                          AND r.mode='composite' AND r.status IN ('ready','partial_ready') AND r.deleted_at IS NULL
                        ON CONFLICT(user_id,dedup_key) DO NOTHING`, item.ID, ids).Error; err != nil {
							return err
						}
						updates["cursor_participant_id"] = ids[len(ids)-1]
					}
					budget -= len(ids)
					done = len(ids) < limit
				}
				if done {
					updates["processed_at"] = time.Now().UTC()
				} else {
					updates["available_at"] = time.Now().UTC()
				}
				if err := tx.Table("notification_jobs").Where("id=?", item.ID).Updates(updates).Error; err != nil {
					return err
				}
			}
			return nil
		})
}

// Pending возвращает порцию сохранённых уведомлений, ещё не отмеченных как опубликованные.
//
// @parameters:
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//
// @return:
//   - результат 1 ([]domain.Notification): собранные элементы результата; состав ограничивается параметрами операции.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r *NotificationRepository) Pending(ctx context.Context) ([]domain.Notification, error) {
	items := []domain.Notification{}
	err := r.db.WithContext(ctx).Where("published_at IS NULL").Order("created_at,id").Limit(100).Find(&items).Error
	return items, err
}

// Published фиксирует успешную публикацию уведомления, сохраняя возможность безопасного повторения после сбоя.
//
// @parameters:
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - id (string): идентификатор обрабатываемого ресурса.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r *NotificationRepository) Published(ctx context.Context, id string) error {
	err := r.db.WithContext(ctx).Model(&domain.Notification{}).Where("id=? AND published_at IS NULL", id).Update("published_at", time.Now().UTC()).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	return err
}
