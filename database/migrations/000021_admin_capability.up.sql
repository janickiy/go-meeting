-- Global operations access is explicit and deny-by-default. Conference owner/co-host
-- roles never grant access to application-wide operational data.
ALTER TABLE users ADD COLUMN IF NOT EXISTS is_admin BOOLEAN NOT NULL DEFAULT FALSE;

-- The admin summary reads recent terminal failures; avoid a full queue scan.
CREATE INDEX IF NOT EXISTS idx_background_jobs_failed_recent
    ON background_jobs(updated_at DESC) WHERE state = 'failed';
CREATE INDEX IF NOT EXISTS idx_record_failed_recent
    ON record(updated_at DESC) WHERE status = 'failed' AND platform_conference_id IS NOT NULL AND deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_transcripts_failed_recent
    ON transcripts(updated_at DESC) WHERE status = 'failed';
