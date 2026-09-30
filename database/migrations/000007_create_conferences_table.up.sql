CREATE TABLE IF NOT EXISTS conferences (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    title VARCHAR(200) NOT NULL,
    invite_code VARCHAR(32) NOT NULL UNIQUE,
    status VARCHAR(16) NOT NULL DEFAULT 'created',
    started_at TIMESTAMPTZ NULL,
    finished_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT conferences_title_not_empty CHECK (length(btrim(title)) > 0),
    CONSTRAINT conferences_invite_code_format CHECK (invite_code ~ '^[A-Za-z0-9_-]{32}$'),
    CONSTRAINT conferences_status_check CHECK (status IN ('created', 'active', 'finished', 'cancelled')),
    CONSTRAINT conferences_lifecycle_check CHECK (
        (status = 'created' AND started_at IS NULL AND finished_at IS NULL) OR
        (status = 'active' AND started_at IS NOT NULL AND finished_at IS NULL) OR
        (status = 'finished' AND started_at IS NOT NULL AND finished_at IS NOT NULL) OR
        (status = 'cancelled' AND started_at IS NULL AND finished_at IS NOT NULL)
    )
);
CREATE INDEX IF NOT EXISTS idx_conferences_owner_created ON conferences(owner_id, created_at DESC);
DROP TRIGGER IF EXISTS trg_conferences_updated_at ON conferences;
CREATE TRIGGER trg_conferences_updated_at BEFORE UPDATE ON conferences
FOR EACH ROW EXECUTE FUNCTION set_updated_at();
