package postgres

import (
	"context"
	"encoding/json"
	domain "github.com/janickiy/meet-space/internal/domain/analytics"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/jobs"
	"gorm.io/gorm"
	"time"
)

// AnalyticsRepository хранит агрегаты без отдельных строк SQL для частых кадров RTP и детекции речи.
type AnalyticsRepository struct{ db *gorm.DB }

// NewAnalyticsRepository связывает техническую аналитику с постоянным хранилищем.
// @args db — SQL pool.
// @return repository агрегатов.
func NewAnalyticsRepository(db *gorm.DB) *AnalyticsRepository { return &AnalyticsRepository{db} }

// Source читает ограниченный набор интервалов подключений, отсекая время до допуска из зала ожидания.
// @args ctx — срок выполнения; job — конференция и аренда.
// @return начало/конец встречи, интервалы и ошибка.
func (r *AnalyticsRepository) Source(ctx context.Context, job jobs.Job) (time.Time, time.Time, []domain.Interval, error) {
	var bounds struct{ Start, End time.Time }
	rows := []domain.Interval{}
	err := r.db.WithContext(ctx).Raw(`SELECT COALESCE(started_at,created_at) AS start,COALESCE(finished_at,clock_timestamp()) AS end FROM conferences WHERE id=?`, job.ConferenceID).Scan(&bounds).Error
	if err != nil || bounds.Start.IsZero() {
		return bounds.Start, bounds.End, rows, jobs.ErrSkip
	}
	err = r.db.WithContext(ctx).Raw(`SELECT s.participant_id,GREATEST(s.connected_at,COALESCE((SELECT min(n.created_at) FROM notification_jobs n WHERE n.participant_id=p.id AND n.kind='admission.decided' AND n.admission_state='admitted'),s.connected_at)) AS start,
 LEAST(COALESCE(s.disconnected_at,clock_timestamp()),?,CASE WHEN p.admission_state='kicked' THEN p.admission_decided_at ELSE NULL END) AS end FROM participant_sessions s JOIN conference_participants p ON p.id=s.participant_id
 WHERE s.conference_id=? AND p.admission_state IN ('admitted','kicked') ORDER BY s.connected_at LIMIT 20001`, bounds.End, job.ConferenceID).Scan(&rows).Error
	if len(rows) > 20000 {
		return bounds.Start, bounds.End, nil, jobs.Error{Code: "analytics_session_limit"}
	}
	return bounds.Start, bounds.End, rows, err
}

// Save сохраняет snapshot присутствия, не перезаписывая одновременно накопленную речь и демонстрацию экрана.
// @args ctx — срок выполнения; job — актуальная аренда; value — рассчитанные агрегаты.
// @return ошибка commit.
func (r *AnalyticsRepository) Save(ctx context.Context, job jobs.Job, value domain.Conference) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := contentLease(tx, job); err != nil {
			return err
		}
		timeline, _ := json.Marshal(value.Timeline)
		if err := tx.Exec(`INSERT INTO conference_analytics(conference_id,duration_ms,participant_count,participant_timeline,audio_observation_enabled)
   VALUES(?,?,?,?::jsonb,true) ON CONFLICT(conference_id) DO UPDATE SET duration_ms=EXCLUDED.duration_ms,participant_count=EXCLUDED.participant_count,participant_timeline=EXCLUDED.participant_timeline,audio_observation_enabled=true,updated_at=clock_timestamp()`, job.ConferenceID, value.DurationMS, value.ParticipantCount, string(timeline)).Error; err != nil {
			return err
		}
		for _, p := range value.Participants {
			if err := tx.Exec(`INSERT INTO participant_analytics(participant_id,conference_id,participation_ms) VALUES(?,?,?)
   ON CONFLICT(participant_id) DO UPDATE SET participation_ms=EXCLUDED.participation_ms,updated_at=clock_timestamp()`, p.ParticipantID, job.ConferenceID, p.ParticipationMS).Error; err != nil {
				return err
			}
		}
		return tx.Exec(`UPDATE participant_analytics a SET message_count=(SELECT count(*) FROM chat_messages m JOIN conference_participants p ON p.id=a.participant_id WHERE m.conference_id=a.conference_id AND m.sender_user_id=p.user_id AND m.deleted_at IS NULL) WHERE a.conference_id=?`, job.ConferenceID).Error
	})
}

// Read повторно проверяет доступ к истории в обоих запросах и возвращает только технические значения.
// @args ctx — срок выполнения; user,cid — актор/встреча.
// @return snapshot с понятной приближённостью speaking-time.
func (r *AnalyticsRepository) Read(ctx context.Context, user, cid string) (domain.Conference, error) {
	p, err := findMembership(r.db.WithContext(ctx), cid, user)
	if err != nil {
		return domain.Conference{}, membershipError(err)
	}
	if !p.CanReadHistory() {
		return domain.Conference{}, apperrors.ErrForbidden
	}
	result := domain.Conference{ConferenceID: cid, ApproximateSpeaking: true, Participants: []domain.Participant{}, Timeline: []domain.Point{}}
	var row struct {
		domain.Conference
		ParticipantTimeline json.RawMessage
	}
	scope := `EXISTS(SELECT 1 FROM conference_participants a WHERE a.conference_id=? AND a.user_id=? AND a.admission_state='admitted' AND a.status IN ('joined','left'))`
	err = r.db.WithContext(ctx).Raw(`SELECT a.*,a.audio_observation_enabled AS enabled,
 EXISTS(SELECT 1 FROM record r WHERE r.platform_conference_id=a.conference_id AND r.status='ready' AND r.deleted_at IS NULL) AS recording_available,
 EXISTS(SELECT 1 FROM transcripts t JOIN record r ON r.uuid=t.recording_id WHERE t.conference_id=a.conference_id AND t.status='ready' AND r.deleted_at IS NULL) AS transcript_available
 FROM conference_analytics a WHERE a.conference_id=? AND `+scope, cid, cid, user).Scan(&row).Error
	if err != nil {
		return result, err
	}
	if row.ConferenceID != "" {
		result = row.Conference
		result.ApproximateSpeaking = true
		_ = json.Unmarshal(row.ParticipantTimeline, &result.Timeline)
	}
	result.Participants = []domain.Participant{}
	if result.Timeline == nil {
		result.Timeline = []domain.Point{}
	}
	err = r.db.WithContext(ctx).Raw(`SELECT a.participant_id,p.display_name,a.participation_ms,LEAST(a.speaking_ms,a.participation_ms) AS speaking_ms,a.observed_audio_ms,
 a.screen_ms+CASE WHEN a.screen_started_at IS NULL THEN 0 ELSE GREATEST(0,(EXTRACT(EPOCH FROM(LEAST(COALESCE(c.finished_at,clock_timestamp()),clock_timestamp(),COALESCE(l.lease_until,l.updated_at,a.updated_at))-a.screen_started_at))*1000)::bigint) END AS screen_ms,a.message_count
 FROM participant_analytics a JOIN conference_participants p ON p.id=a.participant_id JOIN conferences c ON c.id=a.conference_id LEFT JOIN live_transcription_sessions l ON l.conference_id=a.conference_id WHERE a.conference_id=? AND `+scope+` ORDER BY p.display_name,a.participant_id LIMIT 500`, cid, cid, user).Scan(&result.Participants).Error
	return result, err
}

// Tick ставит coarse пересчёт каждые 30 секунд для активных и недавно завершённых встреч.
// @args ctx — короткий срок выполнения планировщика.
// @return ошибка очереди; payload не содержит персональные данные.
func (r *AnalyticsRepository) Tick(ctx context.Context) error {
	return r.db.WithContext(ctx).Exec(`INSERT INTO background_jobs(kind,entity_id,conference_id,version,dedup_key,max_attempts)
 SELECT 'analytics.aggregate',id,id,1,'analytics:'||id::text||':'||floor(extract(epoch FROM clock_timestamp())/30)::text,3
 FROM conferences WHERE status='active' OR finished_at>clock_timestamp()-interval '2 minutes' ORDER BY created_at DESC LIMIT 100
 ON CONFLICT(dedup_key) DO NOTHING`).Error
}
