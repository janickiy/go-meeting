package postgres

import (
	"context"
	domain "github.com/janickiy/meet-space/internal/domain/content"
)

// contentSearchSQL использует простой словесный индекс для русского и английского текста без семантического поиска.
// Permissions являются частью SQL до rank/page; snippet содержит обычный текст,
// не HTML. Старый summary исключается, если сменилось поколение transcript.
const contentSearchSQL = `WITH searchquery AS (SELECT websearch_to_tsquery('simple'::regconfig,?) AS query),
 permitted AS (SELECT c.* FROM conferences c WHERE
 EXISTS(SELECT 1 FROM conference_participants p WHERE p.conference_id=c.id AND p.user_id=? AND p.admission_state='admitted' AND p.status IN ('joined','left'))
 AND (?='' OR c.id::text=?)
 AND (? IN ('','all') OR (?='owned' AND c.owner_id::text=?) OR (?='participating' AND c.owner_id::text<>?))
 AND (?='' OR EXISTS(SELECT 1 FROM conference_participants fp WHERE fp.conference_id=c.id AND fp.id::text=?))
 AND (?::timestamptz IS NULL OR COALESCE(c.finished_at,c.scheduled_at,c.created_at)>=?::timestamptz)
 AND (?::timestamptz IS NULL OR COALESCE(c.finished_at,c.scheduled_at,c.created_at)<=?::timestamptz)),
 matches AS (
 SELECT 'conference'::text AS type,c.id AS conference_id,c.title AS conference_title,NULL::uuid AS recording_id,NULL::uuid AS transcript_id,NULL::uuid AS segment_id,NULL::bigint AS start_ms,
 left(c.title,240) AS snippet,NULL::uuid AS speaker_id,''::text AS speaker,ts_rank(c.search_vector,q.query) AS rank
 FROM permitted c CROSS JOIN searchquery q WHERE c.search_vector@@q.query AND ? IN ('all','conference')
 UNION ALL
 SELECT 'transcript',c.id,c.title,r.uuid,t.id,s.id,s.start_ms,
 ts_headline('simple'::regconfig,s.text,q.query,'StartSel=[,StopSel=],MaxWords=35,MinWords=12,MaxFragments=1'),s.speaker_id,COALESCE(s.speaker_label,''),ts_rank(s.search_vector,q.query)
 FROM permitted c JOIN transcripts t ON t.conference_id=c.id AND t.status='ready'
 JOIN record r ON r.uuid=t.recording_id AND r.platform_conference_id=c.id AND r.mode IN ('composite','audio_only','individual_tracks','screen_focus') AND r.status='ready' AND r.deleted_at IS NULL
 JOIN transcript_segments s ON s.transcript_id=t.id CROSS JOIN searchquery q WHERE s.search_vector@@q.query AND ? IN ('all','transcript') AND (?='' OR s.speaker_id::text=?)
 UNION ALL
 SELECT 'summary',c.id,c.title,r.uuid,t.id,NULL::uuid,NULL::bigint,
 ts_headline('simple'::regconfig,m.search_text,q.query,'StartSel=[,StopSel=],MaxWords=35,MinWords=12,MaxFragments=1'),NULL::uuid,'',ts_rank(m.search_vector,q.query)
 FROM permitted c JOIN transcripts t ON t.conference_id=c.id AND t.status='ready'
 JOIN record r ON r.uuid=t.recording_id AND r.platform_conference_id=c.id AND r.mode IN ('composite','audio_only','individual_tracks','screen_focus') AND r.status='ready' AND r.deleted_at IS NULL
 JOIN meeting_summaries m ON m.transcript_id=t.id AND m.transcript_generation=t.generation AND m.status='ready'
 CROSS JOIN searchquery q WHERE m.search_vector@@q.query AND ? IN ('all','summary')) `

// Search выполняет разрешённый FTS без выдачи count/snippet из других конференций.
// @args ctx — срок выполнения запроса; userID — действующий пользователь; query — проверенные фильтры и пагинация.
// @return страница plain-text результатов с timestamp и ошибка БД.
func (r *ContentRepository) Search(ctx context.Context, userID string, query domain.SearchQuery) (domain.SearchPage, error) {
	page := domain.SearchPage{Items: []domain.SearchResult{}, Limit: query.Limit, Offset: query.Offset}
	args := contentSearchArgs(userID, query)
	db := r.db.WithContext(ctx)
	if err := db.Raw(contentSearchSQL+"SELECT count(*) FROM matches", args...).Scan(&page.Total).Error; err != nil {
		return page, err
	}
	args = append(args, query.Limit, query.Offset)
	err := db.Raw(contentSearchSQL+"SELECT * FROM matches ORDER BY rank DESC,conference_id,type,segment_id NULLS FIRST LIMIT ? OFFSET ?", args...).Scan(&page.Items).Error
	return page, err
}

// contentSearchArgs формирует параметры запроса с предварительной проверкой прав в неизменном порядке подстановок.
// @args userID — текущий актор; query — проверенные фильтры.
// @return параметры SQL без пользовательской интерполяции.
func contentSearchArgs(userID string, query domain.SearchQuery) []any {
	return []any{query.Query, userID, query.ConferenceID, query.ConferenceID, query.Membership, query.Membership, userID, query.Membership, userID, query.ParticipantID, query.ParticipantID, query.From, query.From, query.To, query.To, query.Source, query.Source, query.ParticipantID, query.ParticipantID, query.Source}
}
