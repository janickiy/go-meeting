package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	domain "github.com/janickiy/meet-space/internal/domain/captions"
	"github.com/janickiy/meet-space/internal/domain/conferences"
	live "github.com/janickiy/meet-space/internal/usecase/captions"
	"gorm.io/gorm"
)

// CaptionsRepository хранит только финальные реплики, настройки и ограниченные агрегаты активности.
type CaptionsRepository struct{ db *gorm.DB }

// NewCaptionsRepository подключает хранилище без запуска распознавания.
// @args db — SQL pool.
// @return репозиторий live-обработки.
func NewCaptionsRepository(db *gorm.DB) *CaptionsRepository { return &CaptionsRepository{db} }

// Read возвращает состояние после актуальной проверки допуска к истории встречи.
// @args ctx — срок выполнения; user,cid — актор и конференция.
// @return состояние, включая право управления, либо ошибка доступа.
func (r *CaptionsRepository) Read(ctx context.Context, user, cid string) (domain.State, error) {
	db := r.db.WithContext(ctx)
	p, err := findMembership(db, cid, user)
	if err != nil {
		return domain.State{}, membershipError(err)
	}
	if !p.CanReadHistory() {
		return domain.State{}, apperrors.ErrForbidden
	}
	state := domain.State{ConferenceID: cid, Status: "off", Language: "auto", CanManage: p.CanParticipate() && (p.Role == conferences.Owner || p.Role == conferences.CoHost)}
	row := db.Raw(`SELECT s.id AS session_id,s.conference_id,s.enabled,s.language,s.status,s.generation,s.origin,
 CASE WHEN EXISTS(SELECT 1 FROM transcripts t JOIN record r ON r.uuid=t.recording_id WHERE t.recording_id=s.canonical_recording_id AND t.status='ready' AND r.deleted_at IS NULL) THEN s.canonical_recording_id ELSE NULL END AS canonical_recording_id
 FROM live_transcription_sessions s WHERE s.conference_id=? AND EXISTS(SELECT 1 FROM conference_participants p WHERE p.conference_id=s.conference_id AND p.user_id=? AND p.admission_state='admitted' AND p.status IN ('joined','left'))`, cid, user).Scan(&state)
	return state, row.Error
}

// Set меняет согласие только для участвующего организатора или соорганизатора и прекращает прежнюю аренду.
// @args ctx — срок выполнения; user,cid — актор и встреча; enabled — отправка провайдеру; language — auto/ru/en.
// @return новое состояние либо ошибка роли/состояния.
func (r *CaptionsRepository) Set(ctx context.Context, user, cid string, enabled bool, language string) (domain.State, error) {
	if language != "auto" && language != "ru" && language != "en" {
		return domain.State{}, apperrors.ErrInvalidInput
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		c, e := findConference(tx, cid, true)
		if e != nil {
			return e
		}
		p, e := findMembership(tx, cid, user)
		if e != nil {
			return membershipError(e)
		}
		if !p.CanParticipate() || (p.Role != conferences.Owner && p.Role != conferences.CoHost) {
			return apperrors.ErrForbidden
		}
		if c.Status != conferences.Active {
			return apperrors.ErrConflict
		}
		return tx.Exec(`INSERT INTO live_transcription_sessions(conference_id,enabled,language,status,origin,enabled_at)
   VALUES(?,?,?,CASE WHEN ? THEN 'queued' ELSE 'off' END,?,clock_timestamp())
   ON CONFLICT(conference_id) DO UPDATE SET enabled=EXCLUDED.enabled,language=EXCLUDED.language,
    status=EXCLUDED.status,generation=live_transcription_sessions.generation+1,enabled_at=clock_timestamp(),
    attempts=0,lease_token=NULL,lease_until=NULL,error_code=NULL,updated_at=clock_timestamp()
   WHERE live_transcription_sessions.enabled IS DISTINCT FROM EXCLUDED.enabled OR live_transcription_sessions.language<>EXCLUDED.language OR live_transcription_sessions.status IN ('failed','completed')`, cid, enabled, language, enabled, c.StartedAt).Error
	})
	if err != nil {
		return domain.State{}, err
	}
	return r.Read(ctx, user, cid)
}

// Finals выдаёт курсорную страницу, применяя права внутри запроса до выборки текста.
// @args ctx — срок выполнения; user,cid — область доступа; after — постоянный курсор; limit — 1..200.
// @return финальные реплики всех live-поколений, восстановимые после потери WS.
func (r *CaptionsRepository) Finals(ctx context.Context, user, cid string, after int64, limit int) ([]domain.Caption, error) {
	if after < 0 || limit < 1 || limit > 200 {
		return nil, apperrors.ErrInvalidInput
	}
	if _, err := r.Read(ctx, user, cid); err != nil {
		return nil, err
	}
	items := []domain.Caption{}
	err := r.db.WithContext(ctx).Raw(`SELECT f.*,p.display_name AS speaker,true AS final FROM live_caption_segments f JOIN conference_participants p ON p.id=f.participant_id
 WHERE f.conference_id=? AND f.cursor>? AND EXISTS(SELECT 1 FROM conference_participants a WHERE a.conference_id=f.conference_id AND a.user_id=? AND a.admission_state='admitted' AND a.status IN ('joined','left'))
 ORDER BY f.cursor LIMIT ?`, cid, after, user, limit).Scan(&items).Error
	return items, err
}

// Claim атомарно получает одну конференцию с ограничением активных аренд во всём кластере.
// @args ctx — срок выполнения; analytics — автоматическое локальное наблюдение; maximum — число конференций; attempts — предел перезапусков.
// @return аренда либо gorm.ErrRecordNotFound при отсутствии доступной работы.
func (r *CaptionsRepository) Claim(ctx context.Context, analytics bool, maximum, attempts int) (live.Lease, error) {
	lease := live.Lease{}
	if err := r.db.WithContext(ctx).Exec(`UPDATE live_transcription_sessions SET status='failed',lease_until=NULL,lease_token=NULL,error_code='restart_limit',updated_at=clock_timestamp() WHERE attempts>=? AND (lease_until IS NULL OR lease_until<clock_timestamp()) AND status IN ('queued','active','degraded')`, attempts).Error; err != nil {
		return lease, err
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := tx.Exec(`SELECT pg_advisory_xact_lock(81923018)`).Error; e != nil {
			return e
		}
		if analytics {
			if e := tx.Exec(`INSERT INTO live_transcription_sessions(conference_id,origin,status)
   SELECT id,started_at,'queued' FROM conferences WHERE status='active' AND started_at IS NOT NULL ON CONFLICT(conference_id) DO NOTHING`).Error; e != nil {
				return e
			}
		}
		var active int64
		if e := tx.Table("live_transcription_sessions").Where("lease_until>clock_timestamp()").Count(&active).Error; e != nil {
			return e
		}
		if active >= int64(maximum) {
			return gorm.ErrRecordNotFound
		}
		result := tx.Raw(`UPDATE live_transcription_sessions s SET lease_token=?,analytics_enabled=?,lease_until=clock_timestamp()+interval '15 seconds',attempts=attempts+1,updated_at=clock_timestamp()
   WHERE s.id=(SELECT l.id FROM live_transcription_sessions l JOIN conferences c ON c.id=l.conference_id
    WHERE c.status='active' AND l.status IN ('queued','active','degraded','off') AND (l.enabled OR ?) AND l.attempts<?
     AND (l.lease_until IS NULL OR l.lease_until<clock_timestamp()) ORDER BY l.updated_at LIMIT 1 FOR UPDATE OF l SKIP LOCKED)
   RETURNING s.id AS session_id,s.conference_id,s.enabled,s.language,s.status,s.generation,s.origin,COALESCE(s.enabled_at,s.origin) AS enabled_at,s.attempts,s.lease_token AS token`, uuid.NewString(), analytics, analytics, attempts).Scan(&lease)
		if result.Error != nil {
			return result.Error
		}
		if lease.Token == "" {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
	return lease, err
}

// Renew продлевает только текущую аренду активной конференции и поколения.
// @args ctx — срок выполнения; lease — захваченная версия.
// @return true пока обработка разрешена.
func (r *CaptionsRepository) Renew(ctx context.Context, lease live.Lease) (bool, error) {
	result := r.db.WithContext(ctx).Exec(`UPDATE live_transcription_sessions SET lease_until=clock_timestamp()+interval '15 seconds'
 WHERE id=? AND generation=? AND lease_token=? AND lease_until>clock_timestamp() AND EXISTS(SELECT 1 FROM conferences c WHERE c.id=conference_id AND c.status='active')`, lease.SessionID, lease.Generation, lease.Token)
	return result.RowsAffected == 1, result.Error
}

// Finish завершает текущую аренду; ошибки субтитров не изменяют конференцию или запись.
// @args ctx — срок выполнения; lease — актуальная аренда; status — completed/failed.
// @return ошибка SQL.
func (r *CaptionsRepository) Finish(ctx context.Context, lease live.Lease, status string) error {
	if status != "completed" && status != "failed" && status != "queued" {
		return apperrors.ErrInvalidInput
	}
	return r.db.WithContext(ctx).Exec(`UPDATE live_transcription_sessions SET status=?,lease_until=NULL,lease_token=NULL,updated_at=clock_timestamp() WHERE id=? AND generation=? AND lease_token=?`, status, lease.SessionID, lease.Generation, lease.Token).Error
}

// Status сохраняет видимую деградацию только при действующей аренде.
// @args ctx — срок выполнения; lease — актуальная аренда; status — active/degraded.
// @return ошибка SQL.
func (r *CaptionsRepository) Status(ctx context.Context, lease live.Lease, status string) error {
	if status != "active" && status != "degraded" {
		return apperrors.ErrInvalidInput
	}
	return r.db.WithContext(ctx).Exec(`UPDATE live_transcription_sessions SET status=?,updated_at=clock_timestamp() WHERE id=? AND generation=? AND lease_token=? AND lease_until>clock_timestamp()`, status, lease.SessionID, lease.Generation, lease.Token).Error
}

// SaveFinal фиксирует принятую финальную ревизию до публикации события.
// @args ctx — срок выполнения; lease — текущее поколение/аренда; caption — проверенная реплика; maximum — предел строк на конференцию.
// @return сохранённая реплика с курсором либо безопасный отказ.
func (r *CaptionsRepository) SaveFinal(ctx context.Context, lease live.Lease, caption domain.Caption, maximum int) (domain.Caption, error) {
	caption.SessionID = lease.SessionID
	caption.ConferenceID = lease.ConferenceID
	caption.Generation = lease.Generation
	caption.Final = true
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var id string
		if err := tx.Raw(`SELECT id FROM live_transcription_sessions WHERE id=? AND generation=? AND lease_token=? AND lease_until>clock_timestamp() AND enabled FOR UPDATE`, lease.SessionID, lease.Generation, lease.Token).Scan(&id).Error; err != nil {
			return err
		}
		if id == "" {
			return domain.ErrUnavailable
		}
		var count int64
		if err := tx.Table("live_caption_segments").Where("session_id=? AND id<>?", lease.SessionID, caption.ID).Count(&count).Error; err != nil {
			return err
		}
		if count >= int64(maximum) {
			return domain.ErrUnavailable
		}
		var receipt struct {
			Cursor    int64
			CreatedAt time.Time
		}
		result := tx.Raw(`INSERT INTO live_caption_segments(id,session_id,conference_id,participant_id,generation,track_instance_id,utterance_id,sequence,revision,start_ms,end_ms,text,language)
   SELECT ?,?,?,?,?,?,?,?,?,?,?,?,? WHERE EXISTS(SELECT 1 FROM conference_participants WHERE id=? AND conference_id=? AND admission_state='admitted' AND status IN ('joined','left'))
   ON CONFLICT(id) DO UPDATE SET revision=EXCLUDED.revision,sequence=EXCLUDED.sequence,end_ms=EXCLUDED.end_ms,text=EXCLUDED.text,language=EXCLUDED.language,cursor=nextval('live_caption_segments_cursor_seq')
   WHERE live_caption_segments.revision<EXCLUDED.revision
   RETURNING cursor,created_at`, caption.ID, lease.SessionID, lease.ConferenceID, caption.ParticipantID, lease.Generation, caption.TrackInstanceID, caption.UtteranceID, caption.Sequence, caption.Revision, caption.StartMS, caption.EndMS, caption.Text, caption.Language, caption.ParticipantID, lease.ConferenceID).Scan(&receipt)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return domain.ErrUnavailable
		}
		caption.Cursor = receipt.Cursor
		caption.CreatedAt = receipt.CreatedAt
		return nil
	})
	return caption, err
}

// Speaker проверяет серверную принадлежность дорожки допущенному участнику.
// @args ctx — срок выполнения; cid,pid — конференция и участник.
// @return отображаемое имя либо ошибка доступа.
func (r *CaptionsRepository) Speaker(ctx context.Context, cid, pid string) (string, error) {
	var p conferences.Participant
	err := r.db.WithContext(ctx).Where("id=? AND conference_id=? AND admission_state='admitted' AND status='joined'", pid, cid).Take(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = apperrors.ErrForbidden
	}
	return p.DisplayName, err
}

// Observe добавляет одну порцию агрегированной активности, проверяя аренду и принадлежность участника.
// @args ctx — срок выполнения; lease — актуальная аренда; pid — участник; speech,observed — приращения в миллисекундах за небольшой интервал.
// @return ошибка фиксации без влияния на звук.
func (r *CaptionsRepository) Observe(ctx context.Context, lease live.Lease, pid string, speech, observed int64) error {
	if speech < 0 || speech > 30000 || observed < 0 || observed > 30000 {
		return apperrors.ErrInvalidInput
	}
	return r.db.WithContext(ctx).Exec(`INSERT INTO participant_analytics(participant_id,conference_id,speaking_ms,observed_audio_ms)
 SELECT ?,?,?,? WHERE EXISTS(SELECT 1 FROM live_transcription_sessions WHERE id=? AND generation=? AND lease_token=? AND lease_until>clock_timestamp() AND analytics_enabled)
 AND EXISTS(SELECT 1 FROM conference_participants WHERE id=? AND conference_id=?)
 ON CONFLICT(participant_id) DO UPDATE SET speaking_ms=participant_analytics.speaking_ms+EXCLUDED.speaking_ms,observed_audio_ms=participant_analytics.observed_audio_ms+EXCLUDED.observed_audio_ms,updated_at=clock_timestamp()`, pid, lease.ConferenceID, speech, observed, lease.SessionID, lease.Generation, lease.Token, pid, lease.ConferenceID).Error
}
