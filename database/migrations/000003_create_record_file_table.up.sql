CREATE TABLE IF NOT EXISTS record_file (
	id BIGSERIAL PRIMARY KEY,
	record_id BIGINT NOT NULL REFERENCES record(id) ON DELETE CASCADE,
	uuid UUID NOT NULL UNIQUE DEFAULT gen_random_uuid(),
	file_type VARCHAR(32) NOT NULL,
	bucket VARCHAR(128) NOT NULL,
	object_key TEXT NOT NULL,
	file_name VARCHAR(255) NULL,
	mime_type VARCHAR(100) NOT NULL,
	size_bytes BIGINT NULL,
	duration_sec INTEGER NULL,
	checksum_sha256 VARCHAR(64) NULL,
	is_primary BOOLEAN NOT NULL DEFAULT FALSE,
	is_public BOOLEAN NOT NULL DEFAULT FALSE,
	metadata_json JSONB NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	CONSTRAINT record_file_bucket_object_unique UNIQUE(bucket, object_key),
	CONSTRAINT record_file_type_check CHECK (file_type IN (
		'final_mp4', 'preview_jpg', 'playlist_m3u8', 'audio_m4a', 'transcript_json', 'debug_log'
	))
);

CREATE INDEX IF NOT EXISTS idx_record_file_record_id ON record_file(record_id);
CREATE INDEX IF NOT EXISTS idx_record_file_record_type ON record_file(record_id, file_type);
CREATE INDEX IF NOT EXISTS idx_record_file_is_primary ON record_file(is_primary);
CREATE UNIQUE INDEX IF NOT EXISTS idx_record_file_primary_unique ON record_file(record_id) WHERE is_primary = true;

DROP TRIGGER IF EXISTS trg_record_file_updated_at ON record_file;
CREATE TRIGGER trg_record_file_updated_at
BEFORE UPDATE ON record_file
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();
