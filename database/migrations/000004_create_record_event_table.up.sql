CREATE TABLE IF NOT EXISTS record_event (
	id BIGSERIAL PRIMARY KEY,
	record_id BIGINT NOT NULL REFERENCES record(id) ON DELETE CASCADE,
	segment_id BIGINT NULL REFERENCES record_segment(id) ON DELETE SET NULL,
	event_type VARCHAR(64) NOT NULL,
	event_source VARCHAR(32) NOT NULL,
	severity VARCHAR(16) NOT NULL DEFAULT 'info',
	message TEXT NULL,
	payload_json JSONB NULL,
	correlation_id VARCHAR(128) NULL,
	worker_id VARCHAR(128) NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	CONSTRAINT record_event_severity_check CHECK (severity IN ('info', 'warning', 'error'))
);

CREATE INDEX IF NOT EXISTS idx_record_event_record_created_at ON record_event(record_id, created_at);
CREATE INDEX IF NOT EXISTS idx_record_event_event_type ON record_event(event_type);
CREATE INDEX IF NOT EXISTS idx_record_event_severity ON record_event(severity);
CREATE INDEX IF NOT EXISTS idx_record_event_correlation_id ON record_event(correlation_id);
CREATE INDEX IF NOT EXISTS idx_record_event_payload_json ON record_event USING GIN(payload_json);
