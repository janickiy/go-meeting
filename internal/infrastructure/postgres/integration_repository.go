package postgres

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	d "github.com/janickiy/go-recorder/internal/domain/integrations"
	"github.com/janickiy/go-recorder/internal/domain/jobs"
	u "github.com/janickiy/go-recorder/internal/usecase/integrations"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// IntegrationRepository хранит настройки, шифрованные учётные данные, постоянную доставку и календарные соответствия отдельно от обработки медиа.
type IntegrationRepository struct{ db *gorm.DB }

// NewIntegrationRepository связывает хранилище интеграций с текущим пулом подключений к БД.
// @args db — GORM подключение PostgreSQL.
// @return: репозиторий интеграций.
func NewIntegrationRepository(db *gorm.DB) *IntegrationRepository {
	return &IntegrationRepository{db: db}
}

// Preferences возвращает настройки пользователя или безопасные для приватности значения по умолчанию.
// @args ctx — отмена; userID — владелец.
// @return: настройки либо ошибка DB.
func (r *IntegrationRepository) Preferences(ctx context.Context, userID string) (d.Preferences, error) {
	p := d.DefaultPreferences(userID)
	err := r.db.WithContext(ctx).Where("user_id=?", userID).Take(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return d.DefaultPreferences(userID), nil
	}
	return p, err
}

// SavePreferences атомарно обновляет только boolean категории и каналы текущего пользователя.
// @args ctx — отмена; p — проверенные настройки с серверным UserID.
// @return: ошибка DB.
func (r *IntegrationRepository) SavePreferences(ctx context.Context, p d.Preferences) error {
	p.UpdatedAt = time.Now().UTC()
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "user_id"}}, DoUpdates: clause.AssignmentColumns([]string{"invitation", "reminder", "recording", "summary", "email", "push", "updated_at"})}).Create(&p).Error
}

// Devices возвращает ограниченную страницу собственных устройств, опционально только активные.
// @args ctx — отмена; userID — владелец; active — исключить revoked/disabled.
// @return: не более 100 устройств либо ошибка.
func (r *IntegrationRepository) Devices(ctx context.Context, userID string, active bool) ([]d.Device, error) {
	items := []d.Device{}
	q := r.db.WithContext(ctx).Where("user_id=?", userID)
	if active {
		q = q.Where("revoked_at IS NULL AND disabled_at IS NULL")
	}
	err := q.Order("created_at,id").Limit(100).Find(&items).Error
	return items, err
}

// SaveDevice идемпотентно регистрирует зашифрованный токен с пределом 100 активных устройств пользователя.
// @args ctx — отмена; device — стабильный ID/fingerprint и ciphertext.
// @return: ошибка quota или DB.
func (r *IntegrationRepository) SaveDevice(ctx context.Context, device d.Device) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", device.UserID+":devices").Error; err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&d.Device{}).Where("user_id=? AND revoked_at IS NULL AND disabled_at IS NULL AND id<>?", device.UserID, device.ID).Count(&count).Error; err != nil {
			return err
		}
		if count >= 100 {
			return apperrors.ErrConflict
		}
		return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoUpdates: clause.Assignments(map[string]any{"token_ciphertext": device.TokenCiphertext, "platform": device.Platform, "updated_at": time.Now().UTC(), "revoked_at": nil, "disabled_at": nil})}).Create(&device).Error
	})
}

// RevokeDevice отзывает собственную регистрацию и очищает ciphertext.
// @args ctx — отмена; userID/id — владелец и device UUID.
// @return: not found при чужом ID либо ошибка DB.
func (r *IntegrationRepository) RevokeDevice(ctx context.Context, userID, id string) error {
	result := r.db.WithContext(ctx).Model(&d.Device{}).Where("user_id=? AND id=?", userID, id).Updates(map[string]any{"revoked_at": time.Now().UTC(), "token_ciphertext": ""})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return apperrors.ErrNotFound
	}
	return nil
}

// Connections читает только собственные календарные подключения; JSON скрывает секреты.
// @args ctx — отмена; userID — владелец.
// @return: ограниченный список либо ошибка.
func (r *IntegrationRepository) Connections(ctx context.Context, userID string) ([]d.CalendarConnection, error) {
	items := []d.CalendarConnection{}
	err := r.db.WithContext(ctx).Where("user_id=?", userID).Order("created_at,id").Limit(10).Find(&items).Error
	return items, err
}

// SaveConnection сохраняет зашифрованные учётные данные OAuth или демонстрационное подключение.
// @args ctx — отмена; c — подключение с серверным owner.
// @return: ошибка DB.
func (r *IntegrationRepository) SaveConnection(ctx context.Context, c d.CalendarConnection) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoUpdates: clause.AssignmentColumns([]string{"status", "access_ciphertext", "refresh_ciphertext", "scopes", "expires_at", "updated_at"})}).Create(&c).Error
}

// RefreshConnection сохраняет обновлённые токены только действующего прежнего разрешения, не возрождая параллельно отозванный токен.
// @args ctx — отмена; c — новые encrypted credentials; previous — прежний ciphertext refresh token.
// @return: ошибка конфликта при revoke/параллельной rotation либо ошибка DB.
func (r *IntegrationRepository) RefreshConnection(ctx context.Context, c d.CalendarConnection, previous string) error {
	result := r.db.WithContext(ctx).Model(&d.CalendarConnection{}).Where("id=? AND user_id=? AND status='connected' AND refresh_ciphertext=?", c.ID, c.UserID, previous).Updates(map[string]any{"access_ciphertext": c.AccessCiphertext, "refresh_ciphertext": c.RefreshCiphertext, "expires_at": c.ExpiresAt, "scopes": c.Scopes})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return jobs.Error{Code: "calendar_grant_changed", Retryable: true}
	}
	return nil
}

// RevokeConnection атомарно отключает подключение и удаляет сохранённые зашифрованные токены.
// @args ctx — отмена; userID/id — owner и connection UUID.
// @return: not found при чужом ресурсе либо ошибка.
func (r *IntegrationRepository) RevokeConnection(ctx context.Context, userID, id string) error {
	result := r.db.WithContext(ctx).Model(&d.CalendarConnection{}).Where("user_id=? AND id=?", userID, id).Updates(map[string]any{"status": "revoked", "access_ciphertext": "", "refresh_ciphertext": "", "expires_at": nil})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return apperrors.ErrNotFound
	}
	return nil
}

// SaveOAuthState ограничивает одноразовые OAuth попытки и удаляет истёкшие временные секреты.
// @args ctx — отмена; state — привязка пользователя к провайдеру, хеш состояния и зашифрованное проверочное значение PKCE.
// @return: ошибка quota/DB.
func (r *IntegrationRepository) SaveOAuthState(ctx context.Context, state d.OAuthState) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", state.UserID+":oauth").Error; err != nil {
			return err
		}
		if err := tx.Exec(`DELETE FROM calendar_oauth_states WHERE id IN (SELECT id FROM calendar_oauth_states WHERE expires_at<now() OR used_at IS NOT NULL ORDER BY expires_at,id LIMIT 1000)`).Error; err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&d.OAuthState{}).Where("user_id=?", state.UserID).Count(&count).Error; err != nil {
			return err
		}
		if count >= 10 {
			return apperrors.ErrConflict
		}
		return tx.Create(&state).Error
	})
}

// TakeOAuthState одноразово забирает состояние лишь при совпадении владельца и провайдера и неистёкшем сроке.
// @args ctx — отмена; userID/provider/hash — защищённая область OAuth.
// @return: прежний encrypted verifier либо ошибка проверки, без утечки существования чужих states.
func (r *IntegrationRepository) TakeOAuthState(ctx context.Context, userID, provider, hash string) (d.OAuthState, error) {
	var value d.OAuthState
	result := r.db.WithContext(ctx).Raw(`UPDATE calendar_oauth_states SET used_at=now() WHERE user_id=? AND provider=? AND state_hash=? AND used_at IS NULL AND expires_at>now() RETURNING *`, userID, provider, hash).Scan(&value)
	if result.Error != nil {
		return value, result.Error
	}
	if result.RowsAffected == 0 {
		return value, apperrors.ErrInvalidInput
	}
	return value, nil
}

// CalendarMappings показывает состояние внешнего календаря только допущенному owner/cohost истории.
// @args ctx — отмена; userID/conferenceID — actor и область встречи.
// @return: mappings либо ошибка авторизации.
func (r *IntegrationRepository) CalendarMappings(ctx context.Context, userID, conferenceID string) ([]d.CalendarMapping, error) {
	var count int64
	if err := r.db.WithContext(ctx).Table("conference_participants").Where("user_id=? AND conference_id=? AND role IN ('owner','co_host') AND admission_state='admitted' AND status IN ('joined','left')", userID, conferenceID).Count(&count).Error; err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, apperrors.ErrForbidden
	}
	items := []d.CalendarMapping{}
	err := r.db.WithContext(ctx).Table("calendar_event_mappings m").Where("m.conference_id=?", conferenceID).Where(`EXISTS(SELECT 1 FROM conference_participants p WHERE p.conference_id=m.conference_id AND p.user_id=? AND p.role IN ('owner','co_host') AND p.admission_state='admitted' AND p.status IN ('joined','left'))`, userID).Limit(10).Find(&items).Error
	return items, err
}

// Conference читает актуальное versioned расписание для проверки устаревших jobs.
// @args ctx — отмена; id — conference UUID.
// @return: ограниченная проекция либо ошибка.
func (r *IntegrationRepository) Conference(ctx context.Context, id string) (u.ConferenceSnapshot, error) {
	var value u.ConferenceSnapshot
	err := r.db.WithContext(ctx).Table("conferences").Where("id=?", id).Take(&value).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = apperrors.ErrNotFound
	}
	return value, err
}

// MappingsForSync читает только mappings заданной конференции для доверенного worker.
// @args ctx — отмена; conferenceID — область синхронизации.
// @return: ограниченный список либо ошибка DB.
func (r *IntegrationRepository) MappingsForSync(ctx context.Context, conferenceID string) ([]d.CalendarMapping, error) {
	items := []d.CalendarMapping{}
	err := r.db.WithContext(ctx).Where("conference_id=?", conferenceID).Limit(10).Find(&items).Error
	return items, err
}

// integrationLease запрещает записи async результата от истёкшего или перехваченного worker.
// @args tx — текущая транзакция; job — снимок аренды.
// @return: ErrLeaseLost либо ошибка DB; блокировка сохраняется до commit.
func integrationLease(tx *gorm.DB, job jobs.Job) error {
	var id string
	result := tx.Raw(`SELECT id FROM background_jobs WHERE id=? AND state='processing' AND lease_token=? AND lease_until>clock_timestamp() FOR UPDATE`, job.ID, job.LeaseToken).Scan(&id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return jobs.ErrLeaseLost
	}
	return nil
}

// SaveMapping фиксирует только текущую аренду и не позволяет старой версии перезаписать новую.
// @args ctx — отмена; job — исходные версия и аренда; m — безопасное соответствие внешнего ресурса.
// @return: ошибка fenced DB commit.
func (r *IntegrationRepository) SaveMapping(ctx context.Context, job jobs.Job, m d.CalendarMapping) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := integrationLease(tx, job); err != nil {
			return err
		}
		var allowed int64
		if err := tx.Table("conferences c").Joins("JOIN calendar_connections conn ON conn.user_id=c.owner_id AND conn.id=? AND conn.status='connected'", m.ConnectionID).Where("c.id=? AND c.integration_version=?", job.ConferenceID, job.Version).Count(&allowed).Error; err != nil {
			return err
		}
		if allowed == 0 {
			return jobs.ErrSkip
		}
		return tx.Exec(`INSERT INTO calendar_event_mappings(conference_id,connection_id,provider,external_calendar_id,external_event_id,sync_status,source_version,last_synced_at) VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(conference_id,connection_id) DO UPDATE SET external_event_id=EXCLUDED.external_event_id,sync_status=EXCLUDED.sync_status,source_version=EXCLUDED.source_version,last_synced_at=EXCLUDED.last_synced_at WHERE calendar_event_mappings.source_version<=EXCLUDED.source_version`, m.ConferenceID, m.ConnectionID, m.Provider, m.ExternalCalendarID, m.ExternalEventID, m.SyncStatus, m.SourceVersion, m.LastSyncedAt).Error
	})
}

// AcquireCalendar сериализует только внешнюю продуктовую синхронизацию одной конференции на отдельном подключении БД.
// Неблокирующая рекомендательная блокировка не удерживает строки или состояние медиа во время HTTP-вызова провайдера.
// @args ctx — ограниченный срок работы; conferenceID — стабильная область календаря.
// @return: обязательная функция освобождения либо retryable ошибка занятости.
func (r *IntegrationRepository) AcquireCalendar(ctx context.Context, conferenceID string) (func(), error) {
	db, err := r.db.DB()
	if err != nil {
		return nil, err
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	key := "calendar-sync:" + conferenceID
	var acquired bool
	if err = conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock(hashtextextended($1,0))", key).Scan(&acquired); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if !acquired {
		_ = conn.Close()
		return nil, jobs.Error{Code: "calendar_sync_busy", Retryable: true, RetryAfter: time.Second}
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			cleanup, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if _, err := conn.ExecContext(cleanup, "SELECT pg_advisory_unlock(hashtextextended($1,0))", key); err != nil {
				// Не возвращаем в общий пул сессию с неизвестным состоянием рекомендательной блокировки.
				_ = conn.Raw(func(any) error { return driver.ErrBadConn })
			}
			_ = conn.Close()
		})
	}, nil
}

// FailCalendars отмечает последний окончательный сбой синхронизации только с действующей арендой и без отката версии.
// @args ctx — отмена; job — конференция, версия и аренда; code — безопасный технический код, не сохраняемый как тело ответа провайдера.
// @return: ошибка fenced DB commit.
func (r *IntegrationRepository) FailCalendars(ctx context.Context, job jobs.Job, code string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := integrationLease(tx, job); err != nil {
			return err
		}
		return tx.Exec(`INSERT INTO calendar_event_mappings(conference_id,connection_id,provider,external_calendar_id,sync_status,source_version)
	SELECT c.id,conn.id,conn.provider,conn.calendar_id,'failed',? FROM conferences c JOIN calendar_connections conn ON conn.user_id=c.owner_id AND conn.status='connected'
	WHERE c.id=? AND c.integration_version=? ON CONFLICT(conference_id,connection_id) DO UPDATE SET sync_status='failed',source_version=EXCLUDED.source_version WHERE calendar_event_mappings.source_version<=EXCLUDED.source_version`, job.Version, job.ConferenceID, job.Version).Error
	})
}

// Fanout создаёт личные ссылочные уведомления с живой проверкой допуска; терминальные failures направляются только owner.
// @args ctx — отмена; job — событие и аренда; event — разрешённый тип события.
// @return: ошибка DB либо ErrSkip для устаревшего schedule.
func (r *IntegrationRepository) Fanout(ctx context.Context, job jobs.Job, event string) error {
	switch event {
	case "conference.invited", "conference.rescheduled", "conference.cancelled", "conference.soon", "transcript.ready", "summary.ready", "transcript.failed", "summary.failed":
	default:
		return jobs.Error{Code: "integration_event"}
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := integrationLease(tx, job); err != nil {
			return err
		}
		var c u.ConferenceSnapshot
		if err := tx.Table("conferences").Where("id=?", job.ConferenceID).Take(&c).Error; err != nil {
			return err
		}
		isSchedule := event == "conference.invited" || event == "conference.rescheduled" || event == "conference.cancelled" || event == "conference.soon"
		if isSchedule && c.IntegrationVersion != job.Version {
			return jobs.ErrSkip
		}
		if event == "conference.soon" && (c.Status != "scheduled" || c.ScheduledAt == nil || !c.ScheduledAt.After(time.Now())) {
			return jobs.ErrSkip
		}
		var payload map[string]any
		if json.Unmarshal(job.Payload, &payload) != nil {
			return jobs.Error{Code: "integration_payload"}
		}
		payload["conferenceId"] = job.ConferenceID
		if isSchedule && c.ScheduledAt != nil {
			payload["scheduledAt"] = c.ScheduledAt
		}
		delete(payload, "event")
		kind := event
		if event == "transcript.failed" || event == "summary.failed" {
			kind = "processing.failed"
		}
		guard := ""
		var guardArgs []any
		if event == "transcript.ready" || event == "transcript.failed" {
			status := "ready"
			if event == "transcript.failed" {
				status = "failed"
			}
			guard = ` AND EXISTS(SELECT 1 FROM transcripts t JOIN record r ON r.uuid=t.recording_id WHERE t.id=? AND t.conference_id=p.conference_id AND t.generation=? AND t.status=? AND r.status='ready' AND r.deleted_at IS NULL)`
			guardArgs = []any{job.EntityID, job.Version, status}
			payload["transcriptId"] = job.EntityID
			payload["generation"] = job.Version
		} else if event == "summary.ready" || event == "summary.failed" {
			status := "ready"
			if event == "summary.failed" {
				status = "failed"
			}
			guard = ` AND EXISTS(SELECT 1 FROM meeting_summaries s JOIN transcripts t ON t.id=s.transcript_id AND t.status='ready' AND t.generation=s.transcript_generation JOIN record r ON r.uuid=t.recording_id WHERE s.id=? AND s.conference_id=p.conference_id AND s.generation=? AND s.status=? AND r.status='ready' AND r.deleted_at IS NULL)`
			guardArgs = []any{job.EntityID, job.Version, status}
			payload["summaryId"] = job.EntityID
			payload["generation"] = job.Version
		}
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		// Порция ограничена сотней членств; задание продолжения сохраняется в той же транзакции.
		type recipient struct {
			ID     string
			UserID string
		}
		q := tx.Table("conference_participants p").Select("p.id,p.user_id").Where("p.conference_id=? AND p.user_id IS NOT NULL AND p.status NOT IN ('kicked','rejected')", job.ConferenceID)
		if event == "conference.invited" {
			q = q.Where("NOT EXISTS(SELECT 1 FROM conference_invitations i WHERE i.conference_id=p.conference_id AND i.user_id=p.user_id)")
		}
		if cursor, ok := payload["cursorParticipantId"].(string); ok && cursor != "" {
			q = q.Where("p.id>?::uuid", cursor)
		}
		if isSchedule {
			q = q.Where("p.admission_state IN ('admitted','waiting')")
		} else {
			q = q.Where("p.admission_state='admitted' AND p.status IN ('joined','left')")
		}
		if job.UserID != nil {
			q = q.Where("p.user_id=?", *job.UserID)
		}
		if kind == "processing.failed" {
			q = q.Where("p.user_id=?", c.OwnerID)
		}
		var recipients []recipient
		if err := q.Order("p.id").Limit(100).Scan(&recipients).Error; err != nil {
			return err
		}
		for _, recipient := range recipients {
			userID := recipient.UserID
			p := d.DefaultPreferences(userID)
			err := tx.Where("user_id=?", userID).Take(&p).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if !p.Allows(kind) {
				continue
			}
			entity := job.EntityID
			if isSchedule {
				entity = job.ConferenceID
			}
			key := fmt.Sprintf("integration-notification:%s:%s:%d:%s", event, entity, job.Version, userID)
			if event == "conference.soon" {
				offset, _ := payload["offsetSec"].(float64)
				if int64(offset) == 900 {
					key = fmt.Sprintf("soon:%s:%d", c.ID, c.ScheduledAt.UnixMicro())
				} else {
					key = fmt.Sprintf("reminder:%s:%d:%d", c.ID, c.ScheduledAt.UnixMicro(), int64(offset))
				}
			}
			live := "p.admission_state='admitted' AND p.status IN ('joined','left')"
			if isSchedule {
				live = "p.admission_state IN ('admitted','waiting') AND p.status NOT IN ('kicked','rejected') AND c.integration_version=?"
			}
			preference := ""
			switch kind {
			case "conference.invited", "conference.rescheduled", "conference.cancelled":
				preference = "invitation"
			case "conference.soon":
				preference = "reminder"
			case "transcript.ready", "summary.ready":
				preference = "summary"
			}
			if preference != "" {
				live += " AND COALESCE((SELECT pref." + preference + " FROM notification_preferences pref WHERE pref.user_id=p.user_id),TRUE)"
			}
			args := []any{kind, string(encoded), key, userID, job.ConferenceID}
			if isSchedule {
				args = append(args, job.Version)
			}
			args = append(args, guardArgs...)
			if err := tx.Exec(`INSERT INTO notifications(id,user_id,type,payload,dedup_key) SELECT gen_random_uuid(),p.user_id,?,?::jsonb,? FROM conference_participants p JOIN conferences c ON c.id=p.conference_id WHERE p.user_id=? AND p.conference_id=? AND `+live+guard+` ON CONFLICT(user_id,dedup_key) DO NOTHING`, args...).Error; err != nil {
				return err
			}
		}
		if len(recipients) == 100 {
			payload["event"] = event
			payload["cursorParticipantId"] = recipients[len(recipients)-1].ID
			encoded, err := json.Marshal(payload)
			if err != nil {
				return err
			}
			key := fmt.Sprintf("integration-fanout:%s:%s:%d:%v:%s", event, job.EntityID, job.Version, payload["offsetSec"], recipients[len(recipients)-1].ID)
			if err := tx.Exec(`INSERT INTO background_jobs(kind,entity_id,conference_id,user_id,version,payload,dedup_key) VALUES('integrations.event',?,?,?,?,?::jsonb,?) ON CONFLICT(dedup_key) DO NOTHING`, job.EntityID, job.ConferenceID, job.UserID, job.Version, string(encoded), key).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// ScheduleReminders порциями сохраняет напоминания при наступлении смещения текущего расписания без таймера на каждую встречу.
// @args ctx — отмена; offsets — проверенные positive offsets.
// @return: ошибка SQL; повторные ticks не дублируют уведомления.
func (r *IntegrationRepository) ScheduleReminders(ctx context.Context, offsets []time.Duration) error {
	if err := r.db.WithContext(ctx).Exec(`DELETE FROM calendar_oauth_states WHERE id IN (SELECT id FROM calendar_oauth_states WHERE expires_at<now() OR used_at IS NOT NULL ORDER BY expires_at,id LIMIT 1000)`).Error; err != nil {
		return err
	}
	for _, offset := range offsets {
		seconds := int64(offset / time.Second)
		if err := r.db.WithContext(ctx).Exec(`INSERT INTO background_jobs(kind,entity_id,conference_id,version,payload,dedup_key,available_at)
		SELECT 'integrations.event',c.id,c.id,c.integration_version,jsonb_build_object('event','conference.soon','scheduledAt',c.scheduled_at,'offsetSec',?::bigint),
		'reminder-job:'||c.id::text||':'||c.integration_version::text||':'||?::bigint::text,
		GREATEST(now(),c.scheduled_at-(?*interval '1 second')) FROM conferences c
		WHERE c.status='scheduled' AND c.scheduled_at>now() AND c.scheduled_at-(?*interval '1 second')>=c.integration_version_at
		AND NOT EXISTS(SELECT 1 FROM background_jobs j WHERE j.dedup_key='reminder-job:'||c.id::text||':'||c.integration_version::text||':'||?::bigint::text)
		ORDER BY c.scheduled_at,c.id LIMIT 1000 ON CONFLICT(dedup_key) DO NOTHING`, seconds, seconds, seconds, seconds, seconds).Error; err != nil {
			return err
		}
	}
	return nil
}

// Delivery повторно проверяет владельца уведомления, настройки категории и актуальное членство перед отправкой.
// @args ctx — отмена; job — notification entity и авторизованный получатель.
// @return: представление доставки с Allowed=false при отзыве допуска.
func (r *IntegrationRepository) Delivery(ctx context.Context, job jobs.Job) (u.Delivery, error) {
	var result u.Delivery
	if job.UserID == nil {
		return result, jobs.Error{Code: "delivery_user"}
	}
	if err := r.db.WithContext(ctx).Where("id=? AND user_id=?", job.EntityID, *job.UserID).Take(&result.Notification).Error; err != nil {
		return result, err
	}
	if err := r.db.WithContext(ctx).Table("users").Where("id=?", *job.UserID).Pluck("email", &result.Email).Error; err != nil {
		return result, err
	}
	p, err := r.Preferences(ctx, *job.UserID)
	if err != nil {
		return result, err
	}
	result.Preferences = p
	conference, err := r.Conference(ctx, job.ConferenceID)
	if err != nil {
		return result, err
	}
	kind := result.Notification.Type
	if kind == "conference.soon" || kind == "conference.rescheduled" || kind == "conference.invited" {
		if conference.Status != "scheduled" || conference.ScheduledAt == nil {
			return result, nil
		}
		if result.Notification.Payload.ScheduledAt != nil && !result.Notification.Payload.ScheduledAt.Equal(*conference.ScheduledAt) {
			return result, nil
		}
		if kind == "conference.soon" && !conference.ScheduledAt.After(time.Now()) {
			return result, nil
		}
	}
	if kind == "conference.cancelled" && conference.Status != "cancelled" {
		return result, nil
	}
	if kind == "recording.ready" {
		var available int64
		if err := r.db.WithContext(ctx).Table("record").Where("uuid=? AND platform_conference_id=? AND deleted_at IS NULL AND status IN ('ready','partial_ready')", result.Notification.Payload.RecordingID, job.ConferenceID).Count(&available).Error; err != nil {
			return result, err
		}
		if available == 0 {
			return result, nil
		}
	}
	if kind == "transcript.ready" || kind == "summary.ready" {
		var available int64
		if kind == "transcript.ready" {
			err = r.db.WithContext(ctx).Raw(`SELECT count(*) FROM transcripts t JOIN record r ON r.uuid=t.recording_id WHERE t.id=? AND t.conference_id=? AND t.generation=? AND t.status='ready' AND r.status='ready' AND r.deleted_at IS NULL`, result.Notification.Payload.TranscriptID, job.ConferenceID, result.Notification.Payload.Generation).Scan(&available).Error
		} else {
			err = r.db.WithContext(ctx).Raw(`SELECT count(*) FROM meeting_summaries s JOIN transcripts t ON t.id=s.transcript_id AND t.status='ready' AND t.generation=s.transcript_generation JOIN record r ON r.uuid=t.recording_id WHERE s.id=? AND s.conference_id=? AND s.generation=? AND s.status='ready' AND r.status='ready' AND r.deleted_at IS NULL`, result.Notification.Payload.SummaryID, job.ConferenceID, result.Notification.Payload.Generation).Scan(&available).Error
		}
		if err != nil {
			return result, err
		}
		if available == 0 {
			return result, nil
		}
	}
	var count int64
	q := r.db.WithContext(ctx).Table("conference_participants").Where("user_id=? AND conference_id=? AND status NOT IN ('kicked','rejected')", *job.UserID, job.ConferenceID)
	if result.Notification.Type == "conference.invited" || result.Notification.Type == "conference.rescheduled" || result.Notification.Type == "conference.cancelled" || result.Notification.Type == "conference.soon" {
		q = q.Where("admission_state IN ('admitted','waiting')")
	} else if result.Notification.Type == "admission.decided" {
		q = q.Where("admission_state IN ('admitted','rejected')")
	} else {
		q = q.Where("admission_state='admitted' AND status IN ('joined','left')")
	}
	if err := q.Count(&count).Error; err != nil {
		return result, err
	}
	result.Allowed = count > 0
	return result, nil
}

// CompleteDelivery сохраняет единственный окончательный результат доставки с проверкой текущей аренды задания.
// @args ctx — отмена; job — аренда и получатель; status/code — безопасные технические метки.
// @return: ошибка БД или потеря актуальности аренды.
func (r *IntegrationRepository) CompleteDelivery(ctx context.Context, job jobs.Job, status, code string) error {
	if job.UserID == nil {
		return jobs.Error{Code: "delivery_user"}
	}
	var payload struct {
		Channel string `json:"channel"`
	}
	if json.Unmarshal(job.Payload, &payload) != nil {
		return jobs.Error{Code: "delivery_payload"}
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := integrationLease(tx, job); err != nil {
			return err
		}
		return tx.Exec(`INSERT INTO integration_deliveries(job_id,user_id,channel,status,error_code) VALUES(?,?,?,?,?) ON CONFLICT(job_id) DO NOTHING`, job.ID, *job.UserID, payload.Channel, status, code).Error
	})
}

// ScheduleCalendarSync атомарно создаёт jobs для текущих будущих встреч владельца после подключения календаря.
// @args ctx — отмена; userID — владелец подключения.
// @return: ошибка постановки в постоянную очередь.
func (r *IntegrationRepository) ScheduleCalendarSync(ctx context.Context, userID string) error {
	return r.db.WithContext(ctx).Exec(`INSERT INTO background_jobs(kind,entity_id,conference_id,version,payload,dedup_key) SELECT 'integrations.calendar',id,id,integration_version,'{}'::jsonb,'calendar-connect:'||id::text||':'||integration_version::text||':'||? FROM conferences WHERE owner_id=? AND status='scheduled' ORDER BY scheduled_at,id LIMIT 1000 ON CONFLICT(dedup_key) DO NOTHING`, userID, userID).Error
}
