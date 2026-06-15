ALTER TABLE record
	ADD COLUMN IF NOT EXISTS worker_id VARCHAR(128) NULL;

CREATE INDEX IF NOT EXISTS idx_record_worker_id ON record(worker_id);
