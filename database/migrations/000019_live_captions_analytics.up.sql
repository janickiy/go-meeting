-- Потоковое распознавание вспомогательно: его аренды и сбои не меняют жизненный цикл SFU и записи.
CREATE TABLE IF NOT EXISTS live_transcription_sessions (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(), conference_id UUID NOT NULL UNIQUE REFERENCES conferences(id) ON DELETE RESTRICT,
 enabled BOOLEAN NOT NULL DEFAULT FALSE, language VARCHAR(8) NOT NULL DEFAULT 'auto' CHECK(language IN ('auto','ru','en')),
 generation BIGINT NOT NULL DEFAULT 1, status VARCHAR(16) NOT NULL DEFAULT 'off' CHECK(status IN ('off','queued','active','degraded','failed','completed')),
 origin TIMESTAMPTZ NOT NULL, enabled_at TIMESTAMPTZ, lease_token UUID, lease_until TIMESTAMPTZ,
 attempts INTEGER NOT NULL DEFAULT 0, error_code VARCHAR(64), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 canonical_recording_id UUID REFERENCES record(uuid) ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS idx_live_sessions_pending ON live_transcription_sessions(updated_at) WHERE status IN ('queued','active','degraded');
ALTER TABLE live_transcription_sessions ADD COLUMN IF NOT EXISTS analytics_enabled BOOLEAN NOT NULL DEFAULT FALSE;
CREATE TABLE IF NOT EXISTS live_caption_segments (

 cursor BIGSERIAL UNIQUE,
 id UUID PRIMARY KEY, session_id UUID NOT NULL REFERENCES live_transcription_sessions(id) ON DELETE RESTRICT,
 conference_id UUID NOT NULL REFERENCES conferences(id) ON DELETE RESTRICT,
 participant_id UUID NOT NULL REFERENCES conference_participants(id) ON DELETE RESTRICT,
 generation BIGINT NOT NULL, track_instance_id UUID NOT NULL, utterance_id VARCHAR(128) NOT NULL,
 sequence BIGINT NOT NULL CHECK(sequence>=0), revision BIGINT NOT NULL CHECK(revision>=0),
 start_ms BIGINT NOT NULL CHECK(start_ms>=0), end_ms BIGINT NOT NULL CHECK(end_ms>=start_ms),
 text TEXT NOT NULL CHECK(length(text) BETWEEN 1 AND 4000), language VARCHAR(8) NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_live_captions_page ON live_caption_segments(conference_id,start_ms,id);
CREATE INDEX IF NOT EXISTS idx_live_captions_cursor ON live_caption_segments(conference_id,cursor);
CREATE TABLE IF NOT EXISTS participant_analytics (
 participant_id UUID PRIMARY KEY REFERENCES conference_participants(id) ON DELETE RESTRICT,
 conference_id UUID NOT NULL REFERENCES conferences(id) ON DELETE RESTRICT,
 participation_ms BIGINT NOT NULL DEFAULT 0, speaking_ms BIGINT NOT NULL DEFAULT 0,
 observed_audio_ms BIGINT NOT NULL DEFAULT 0, screen_ms BIGINT NOT NULL DEFAULT 0,
 message_count BIGINT NOT NULL DEFAULT 0, hand_raises BIGINT NOT NULL DEFAULT 0,
 screen_started_at TIMESTAMPTZ, last_hand_at TIMESTAMPTZ, updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_participant_analytics_conference ON participant_analytics(conference_id);
CREATE TABLE IF NOT EXISTS conference_analytics (
 conference_id UUID PRIMARY KEY REFERENCES conferences(id) ON DELETE RESTRICT,
 duration_ms BIGINT NOT NULL DEFAULT 0, participant_count INTEGER NOT NULL DEFAULT 0,
 participant_timeline JSONB NOT NULL DEFAULT '[]',
 recording_available BOOLEAN NOT NULL DEFAULT FALSE, transcript_available BOOLEAN NOT NULL DEFAULT FALSE,
 audio_observation_enabled BOOLEAN NOT NULL DEFAULT FALSE, updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- Хранятся только переходы состояния демонстрации, без отдельных строк на каждый RTP-пакет или кадр.
CREATE OR REPLACE FUNCTION track_screen_duration() RETURNS TRIGGER AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM live_transcription_sessions WHERE conference_id=NEW.conference_id AND analytics_enabled AND lease_until>clock_timestamp()) THEN
  RETURN NEW;
 END IF;
 IF NEW.screen_sharing IS DISTINCT FROM OLD.screen_sharing OR (NEW.status IS DISTINCT FROM OLD.status AND OLD.screen_sharing) THEN
  INSERT INTO participant_analytics(participant_id,conference_id) VALUES(NEW.id,NEW.conference_id) ON CONFLICT DO NOTHING;
  UPDATE participant_analytics SET
   screen_ms=screen_ms+CASE WHEN screen_started_at IS NOT NULL THEN GREATEST(0,(EXTRACT(EPOCH FROM(clock_timestamp()-screen_started_at))*1000)::bigint) ELSE 0 END,
   screen_started_at=CASE WHEN NEW.screen_sharing AND NEW.status='joined' THEN clock_timestamp() ELSE NULL END,
   updated_at=clock_timestamp() WHERE participant_id=NEW.id;
 END IF;
 RETURN NEW;
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS trg_screen_analytics ON conference_participants;
CREATE TRIGGER trg_screen_analytics AFTER UPDATE OF screen_sharing,status ON conference_participants FOR EACH ROW EXECUTE FUNCTION track_screen_duration();
