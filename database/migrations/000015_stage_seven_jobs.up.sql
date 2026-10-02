-- Постоянная продуктовая очередь отделена от real-time и команд записи.
-- conference_id намеренно не FK: INSERT из ready-trigger не должен блокировать
-- строку конференции и инвертировать порядок блокировок recorder/owner Stop.
CREATE TABLE IF NOT EXISTS background_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kind VARCHAR(64) NOT NULL,
    entity_id UUID NOT NULL,
    conference_id UUID NOT NULL,
    user_id UUID NULL,
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    payload JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (octet_length(payload::text) <= 32768),
    dedup_key VARCHAR(255) NOT NULL UNIQUE,
    state VARCHAR(16) NOT NULL DEFAULT 'queued' CHECK (state IN ('queued','processing','done','failed','skipped')),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    max_attempts INTEGER NOT NULL DEFAULT 5 CHECK (max_attempts BETWEEN 1 AND 10),
    available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    lease_token UUID NULL,
    lease_until TIMESTAMPTZ NULL,
    error_code VARCHAR(64) NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at TIMESTAMPTZ NULL,
    CHECK ((state='processing') = (lease_token IS NOT NULL AND lease_until IS NOT NULL))
);
CREATE INDEX IF NOT EXISTS idx_background_jobs_available ON background_jobs(kind,available_at,id) WHERE state IN ('queued','processing');
CREATE INDEX IF NOT EXISTS idx_background_jobs_entity ON background_jobs(entity_id,kind,version);
CREATE INDEX IF NOT EXISTS idx_background_jobs_retention ON background_jobs(finished_at,id) WHERE finished_at IS NOT NULL;
