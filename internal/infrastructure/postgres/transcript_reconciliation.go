package postgres

import (
	domain "github.com/janickiy/go-recorder/internal/domain/content"
	"gorm.io/gorm"
)

// reconcileLiveTranscript сохраняет живой черновик и делает основной готовую расшифровку после записи.
// Speaker переносится только при буквальном совпадении нормализованного текста, времени и единственном участнике.
// @args tx — транзакция ready transcript; t — проверенные связанные UUID.
// @return ошибка SQL; неизвестная личность остаётся NULL и не угадывается по голосу.
func reconcileLiveTranscript(tx *gorm.DB, t domain.Transcript) error {
	if err := tx.Exec(`WITH links AS (
 SELECT s.id,min(l.participant_id::text)::uuid AS participant_id FROM transcript_segments s
 JOIN record r ON r.uuid=? AND r.media_started_at IS NOT NULL
 JOIN live_transcription_sessions ls ON ls.conference_id=r.platform_conference_id
 JOIN live_caption_segments l ON l.session_id=ls.id
 WHERE s.transcript_id=? AND lower(regexp_replace(s.text,'[[:space:]]+',' ','g'))=lower(regexp_replace(l.text,'[[:space:]]+',' ','g'))
 AND abs(EXTRACT(EPOCH FROM((r.media_started_at+s.start_ms*interval '1 millisecond')-(ls.origin+l.start_ms*interval '1 millisecond'))))<=2
 GROUP BY s.id HAVING count(DISTINCT l.participant_id)=1
 ) UPDATE transcript_segments s SET speaker_id=links.participant_id,speaker_label=p.display_name
 FROM links JOIN conference_participants p ON p.id=links.participant_id WHERE s.id=links.id`, t.RecordingID, t.ID).Error; err != nil {
		return err
	}
	return tx.Exec(`UPDATE live_transcription_sessions SET canonical_recording_id=?,updated_at=clock_timestamp()
 WHERE conference_id=? AND (canonical_recording_id IS NULL OR canonical_recording_id=? OR
 (SELECT created_at FROM record WHERE uuid=canonical_recording_id)<=(SELECT created_at FROM record WHERE uuid=?))`, t.RecordingID, t.ConferenceID, t.RecordingID, t.RecordingID).Error
}
