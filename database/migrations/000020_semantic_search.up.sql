-- Без установленного расширения базовый FTS продолжает запускаться.
CREATE TABLE IF NOT EXISTS content_search_indexes (
 transcript_id UUID NOT NULL REFERENCES transcripts(id) ON DELETE RESTRICT,
 model_key CHAR(64) NOT NULL, generation BIGINT NOT NULL, state VARCHAR(16) NOT NULL DEFAULT 'queued',
 model VARCHAR(100) NOT NULL, model_version VARCHAR(100) NOT NULL, dimensions INTEGER NOT NULL,
 error_code VARCHAR(64), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(), PRIMARY KEY(transcript_id,model_key)
);
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM pg_available_extensions WHERE name='vector') AND NOT EXISTS(SELECT 1 FROM pg_extension WHERE extname='vector') THEN
  BEGIN
   CREATE EXTENSION IF NOT EXISTS vector;
  EXCEPTION WHEN insufficient_privilege THEN
   RAISE NOTICE 'Optional pgvector requires DBA setup; keyword search remains available';
  END;
 END IF;
 IF EXISTS(SELECT 1 FROM pg_extension WHERE extname='vector') THEN
  CREATE TABLE IF NOT EXISTS embedding_cache (
   model_key CHAR(64) NOT NULL, content_hash CHAR(64) NOT NULL, embedding vector NOT NULL,
   created_at TIMESTAMPTZ NOT NULL DEFAULT now(), PRIMARY KEY(model_key,content_hash)
  );
  CREATE TABLE IF NOT EXISTS content_embeddings (
   id UUID PRIMARY KEY, transcript_id UUID NOT NULL REFERENCES transcripts(id) ON DELETE RESTRICT,
   conference_id UUID NOT NULL REFERENCES conferences(id) ON DELETE RESTRICT,
   recording_id UUID NOT NULL REFERENCES record(uuid) ON DELETE RESTRICT,
   generation BIGINT NOT NULL, ordinal INTEGER NOT NULL, segment_id UUID NOT NULL,
   start_ms BIGINT NOT NULL,end_ms BIGINT NOT NULL,speaker_id UUID,speaker VARCHAR(100) NOT NULL DEFAULT '',
   source VARCHAR(16) NOT NULL DEFAULT 'transcript',text TEXT NOT NULL CHECK(length(text) BETWEEN 1 AND 4000),
   model_key CHAR(64) NOT NULL,content_hash CHAR(64) NOT NULL,embedding vector NOT NULL,
   search_vector TSVECTOR GENERATED ALWAYS AS(to_tsvector('simple'::regconfig,text)) STORED,
   UNIQUE(transcript_id,generation,model_key,ordinal)
  );
  CREATE INDEX IF NOT EXISTS idx_embeddings_scope ON content_embeddings(conference_id,model_key,transcript_id,generation);
  CREATE INDEX IF NOT EXISTS idx_embeddings_keywords ON content_embeddings USING GIN(search_vector);
 END IF;
END $$;
CREATE OR REPLACE FUNCTION enqueue_transcript_index() RETURNS TRIGGER AS $$
BEGIN
 IF NEW.status='ready' AND (OLD.status IS DISTINCT FROM NEW.status OR OLD.generation<>NEW.generation) THEN
  INSERT INTO background_jobs(kind,entity_id,conference_id,version,payload,dedup_key,max_attempts)
   VALUES('content.embed',NEW.id,NEW.conference_id,NEW.generation,'{}','content.embed:'||NEW.id::text||':'||NEW.generation::text,5) ON CONFLICT(dedup_key) DO NOTHING;
 END IF;
 RETURN NEW;
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS trg_transcript_index ON transcripts;
CREATE TRIGGER trg_transcript_index AFTER UPDATE OF status,generation ON transcripts FOR EACH ROW EXECUTE FUNCTION enqueue_transcript_index();
