CREATE TABLE IF NOT EXISTS record_segment (
	id BIGSERIAL PRIMARY KEY,
	record_id BIGINT NOT NULL REFERENCES record(id) ON DELETE CASCADE,
	uuid UUID NOT NULL UNIQUE DEFAULT gen_random_uuid(),
	seq_no INTEGER NOT NULL,
	status VARCHAR(32) NOT NULL,
	local_path TEXT NULL,
	object_key TEXT NULL,
	file_name VARCHAR(255) NULL,
	mime_type VARCHAR(100) NULL,
	started_at TIMESTAMPTZ NULL,
	ended_at TIMESTAMPTZ NULL,
	duration_ms INTEGER NULL,
	size_bytes BIGINT NULL,
	checksum_sha256 VARCHAR(64) NULL,
	effective_quality VARCHAR(16) NULL,
	width SMALLINT NULL,
	height SMALLINT NULL,
	fps SMALLINT NULL,
	error_message TEXT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	CONSTRAINT record_segment_record_seq_unique UNIQUE(record_id, seq_no),
	CONSTRAINT record_segment_status_check CHECK (status IN ('writing', 'closed', 'uploaded', 'failed', 'skipped')),
	CONSTRAINT record_segment_effective_quality_check CHECK (effective_quality IS NULL OR effective_quality IN ('1080p', '720p', '480p', '360p'))
);

CREATE INDEX IF NOT EXISTS idx_record_segment_record_id ON record_segment(record_id);
CREATE INDEX IF NOT EXISTS idx_record_segment_status ON record_segment(status);
CREATE INDEX IF NOT EXISTS idx_record_segment_record_status ON record_segment(record_id, status);
CREATE INDEX IF NOT EXISTS idx_record_segment_created_at ON record_segment(created_at);
CREATE INDEX IF NOT EXISTS idx_record_segment_updated_at ON record_segment(updated_at);

DROP TRIGGER IF EXISTS trg_record_segment_updated_at ON record_segment;
CREATE TRIGGER trg_record_segment_updated_at
BEFORE UPDATE ON record_segment
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();
