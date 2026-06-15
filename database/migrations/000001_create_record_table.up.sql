CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE OR REPLACE FUNCTION nullable_uuid_text(value UUID)
RETURNS TEXT
LANGUAGE sql
IMMUTABLE
AS $$
	SELECT CASE WHEN value IS NULL THEN NULL ELSE value::text END
$$;

CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
	NEW.updated_at = now();
	RETURN NEW;
END;
$$;

CREATE TABLE IF NOT EXISTS record (
	id BIGSERIAL PRIMARY KEY,
	uuid UUID NOT NULL UNIQUE DEFAULT gen_random_uuid(),
	conference_id UUID NOT NULL,
	requested_by UUID NULL,
	source_type VARCHAR(32) NOT NULL,
	source_session_id VARCHAR(128) NULL,
	transport_type VARCHAR(32) NOT NULL,
	status VARCHAR(32) NOT NULL,
	requested_quality VARCHAR(16) NULL,
	effective_quality VARCHAR(16) NULL,
	quality_mode VARCHAR(16) NOT NULL,
	min_quality VARCHAR(16) NULL,
	layout VARCHAR(32) NULL,
	audio_mode VARCHAR(32) NULL,
	width SMALLINT NULL,
	height SMALLINT NULL,
	fps SMALLINT NULL,
	segment_duration_sec SMALLINT NULL,
	reconnect_grace_sec SMALLINT NOT NULL DEFAULT 10,
	need_preview BOOLEAN NOT NULL DEFAULT TRUE,
	storage_bucket VARCHAR(128) NULL,
	storage_object_key TEXT NULL,
	preview_object_key TEXT NULL,
	duration_sec INTEGER NULL,
	size_bytes BIGINT NULL,
	started_at TIMESTAMPTZ NULL,
	stopped_at TIMESTAMPTZ NULL,
	ended_at TIMESTAMPTZ NULL,
	ended_reason VARCHAR(64) NULL,
	error_message TEXT NULL,
	metadata_json JSONB NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	deleted_at TIMESTAMPTZ NULL,
	CONSTRAINT record_status_check CHECK (status IN (
		'starting', 'recording', 'degraded', 'stopping', 'finalizing',
		'uploading', 'ready', 'partial_ready', 'failed', 'cancelled'
	)),
	CONSTRAINT record_quality_mode_check CHECK (quality_mode IN ('manual', 'auto')),
	CONSTRAINT record_requested_quality_check CHECK (requested_quality IS NULL OR requested_quality IN ('1080p', '720p', '480p', '360p')),
	CONSTRAINT record_effective_quality_check CHECK (effective_quality IS NULL OR effective_quality IN ('1080p', '720p', '480p', '360p')),
	CONSTRAINT record_min_quality_check CHECK (min_quality IS NULL OR min_quality IN ('1080p', '720p', '480p', '360p')),
	CONSTRAINT record_layout_check CHECK (layout IS NULL OR layout IN ('speaker', 'grid', 'screen_share')),
	CONSTRAINT record_audio_mode_check CHECK (audio_mode IS NULL OR audio_mode IN ('mixed', 'separate'))
);

CREATE INDEX IF NOT EXISTS idx_record_conference_id ON record(conference_id);
CREATE INDEX IF NOT EXISTS idx_record_status ON record(status);
CREATE INDEX IF NOT EXISTS idx_record_created_at ON record(created_at);
CREATE INDEX IF NOT EXISTS idx_record_conference_status ON record(conference_id, status);
CREATE INDEX IF NOT EXISTS idx_record_ended_at ON record(ended_at);

DROP TRIGGER IF EXISTS trg_record_updated_at ON record;
CREATE TRIGGER trg_record_updated_at
BEFORE UPDATE ON record
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();
