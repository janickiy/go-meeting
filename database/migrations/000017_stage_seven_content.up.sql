-- Новые продуктовые задачи создаются внутри ready commit, без ожидания STT/AI
-- и без изменения media command queue. Исторические записи не обрабатываются
-- автоматически: повторная обработка требует разрешённого явного запроса.
CREATE TABLE IF NOT EXISTS transcripts (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 conference_id UUID NOT NULL REFERENCES conferences(id) ON DELETE RESTRICT,
 recording_id UUID NOT NULL UNIQUE REFERENCES record(uuid) ON DELETE RESTRICT,
 status VARCHAR(16) NOT NULL CHECK (status IN ('queued','processing','ready','failed')),
 language VARCHAR(32) NOT NULL DEFAULT 'auto', provider VARCHAR(64) NOT NULL DEFAULT '',
 generation BIGINT NOT NULL DEFAULT 1 CHECK (generation > 0),
 error_code VARCHAR(64), error_message VARCHAR(300),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(), processed_at TIMESTAMPTZ
);
CREATE TABLE IF NOT EXISTS transcript_segments (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 transcript_id UUID NOT NULL REFERENCES transcripts(id) ON DELETE RESTRICT,
 ordinal INTEGER NOT NULL CHECK (ordinal >= 0), start_ms BIGINT NOT NULL CHECK (start_ms >= 0), end_ms BIGINT NOT NULL CHECK (end_ms >= start_ms),
 speaker_id UUID, speaker_label VARCHAR(100), text TEXT NOT NULL CHECK (length(text) BETWEEN 1 AND 20000), confidence DOUBLE PRECISION CHECK (confidence BETWEEN 0 AND 1),
 search_vector TSVECTOR GENERATED ALWAYS AS (to_tsvector('simple'::regconfig,text)) STORED,
 UNIQUE(transcript_id,ordinal)
);
CREATE INDEX IF NOT EXISTS idx_transcript_segments_search ON transcript_segments USING GIN(search_vector);
CREATE TABLE IF NOT EXISTS meeting_summaries (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(), conference_id UUID NOT NULL REFERENCES conferences(id) ON DELETE RESTRICT,
 transcript_id UUID NOT NULL UNIQUE REFERENCES transcripts(id) ON DELETE RESTRICT,
 transcript_generation BIGINT NOT NULL CHECK (transcript_generation > 0),
 status VARCHAR(16) NOT NULL CHECK (status IN ('queued','processing','ready','failed')),
 summary TEXT NOT NULL DEFAULT '', structured_output JSONB NOT NULL DEFAULT '{"summary":"","keyPoints":[],"actionItems":[],"topics":[]}',
 provider VARCHAR(64) NOT NULL DEFAULT '', model VARCHAR(100) NOT NULL DEFAULT '',
 prompt_version VARCHAR(40) NOT NULL DEFAULT '', schema_version VARCHAR(40) NOT NULL DEFAULT '',
 generation BIGINT NOT NULL DEFAULT 1 CHECK (generation > 0), search_text TEXT NOT NULL DEFAULT '',
 search_vector TSVECTOR GENERATED ALWAYS AS (to_tsvector('simple'::regconfig,search_text)) STORED,
 error_code VARCHAR(64), error_message VARCHAR(300),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(), processed_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_meeting_summaries_search ON meeting_summaries USING GIN(search_vector);
ALTER TABLE conferences ADD COLUMN IF NOT EXISTS search_vector TSVECTOR GENERATED ALWAYS AS (to_tsvector('simple'::regconfig,title)) STORED;
CREATE INDEX IF NOT EXISTS idx_conferences_search ON conferences USING GIN(search_vector);

CREATE OR REPLACE FUNCTION enqueue_stage_seven_recording() RETURNS TRIGGER AS $$
BEGIN
 IF NEW.mode='composite' AND NEW.platform_conference_id IS NOT NULL AND NEW.status='ready' AND NEW.deleted_at IS NULL
 AND (TG_OP='INSERT' OR OLD.status IS DISTINCT FROM 'ready') THEN
   -- Нет direct conference FK: сохраняется established recorder lock ordering.
   INSERT INTO background_jobs(kind,entity_id,conference_id,version,payload,dedup_key,max_attempts)
   VALUES ('content.transcribe',NEW.uuid,NEW.platform_conference_id,1,'{}','content.transcribe:'||NEW.uuid::text||':1',5)
   ON CONFLICT(dedup_key) DO NOTHING;
 END IF;
 RETURN NEW;
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS trg_stage_seven_recording ON record;
CREATE TRIGGER trg_stage_seven_recording AFTER INSERT OR UPDATE OF status ON record FOR EACH ROW EXECUTE FUNCTION enqueue_stage_seven_recording();
