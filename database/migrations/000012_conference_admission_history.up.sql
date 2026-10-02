ALTER TABLE conferences ADD COLUMN IF NOT EXISTS waiting_room_enabled BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE conferences ADD COLUMN IF NOT EXISTS scheduled_at TIMESTAMPTZ NULL;
ALTER TABLE conferences ADD COLUMN IF NOT EXISTS planned_duration_min INTEGER NULL;
ALTER TABLE conferences DROP CONSTRAINT IF EXISTS conferences_status_check;
ALTER TABLE conferences ADD CONSTRAINT conferences_status_check CHECK (status IN ('created','scheduled','active','finished','cancelled'));
ALTER TABLE conferences DROP CONSTRAINT IF EXISTS conferences_lifecycle_check;
ALTER TABLE conferences ADD CONSTRAINT conferences_lifecycle_check CHECK (
    (status = 'created' AND started_at IS NULL AND finished_at IS NULL) OR
    (status = 'scheduled' AND scheduled_at IS NOT NULL AND started_at IS NULL AND finished_at IS NULL) OR
    (status = 'active' AND started_at IS NOT NULL AND finished_at IS NULL) OR
    (status = 'finished' AND started_at IS NOT NULL AND finished_at IS NOT NULL) OR
    (status = 'cancelled' AND started_at IS NULL AND finished_at IS NOT NULL)
);
ALTER TABLE conferences DROP CONSTRAINT IF EXISTS conferences_planned_duration_check;
ALTER TABLE conferences ADD CONSTRAINT conferences_planned_duration_check CHECK (
    planned_duration_min IS NULL OR (scheduled_at IS NOT NULL AND planned_duration_min BETWEEN 1 AND 1440)
);
ALTER TABLE conference_participants ADD COLUMN IF NOT EXISTS admission_state VARCHAR(16);
UPDATE conference_participants SET admission_state = CASE WHEN status IN ('waiting','rejected','kicked') THEN status ELSE 'admitted' END WHERE admission_state IS NULL;
ALTER TABLE conference_participants ALTER COLUMN admission_state SET DEFAULT 'admitted';
ALTER TABLE conference_participants ALTER COLUMN admission_state SET NOT NULL;
ALTER TABLE conference_participants ADD COLUMN IF NOT EXISTS admission_decided_at TIMESTAMPTZ NULL;
ALTER TABLE conference_participants ADD COLUMN IF NOT EXISTS admission_version BIGINT NOT NULL DEFAULT 1;
ALTER TABLE conference_participants DROP CONSTRAINT IF EXISTS conference_participants_admission_check;
ALTER TABLE conference_participants ADD CONSTRAINT conference_participants_admission_check CHECK (
    admission_state IN ('waiting','admitted','rejected','kicked') AND admission_version > 0 AND
    (status <> 'joined' OR admission_state = 'admitted') AND
    (status <> 'waiting' OR admission_state = 'waiting') AND
    (status <> 'rejected' OR admission_state = 'rejected') AND
    (status <> 'kicked' OR admission_state = 'kicked')
);
-- Historical membership must not silently vanish if a conference is deleted.
ALTER TABLE conference_participants DROP CONSTRAINT IF EXISTS conference_participants_conference_id_fkey;
ALTER TABLE conference_participants ADD CONSTRAINT conference_participants_conference_id_fkey FOREIGN KEY(conference_id) REFERENCES conferences(id) ON DELETE RESTRICT;
-- Chronological keyset browsing and soon-meeting discovery use these orders.
CREATE INDEX IF NOT EXISTS idx_conferences_timeline ON conferences ((COALESCE(finished_at, scheduled_at, created_at)) DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_conferences_scheduled ON conferences (scheduled_at, id) WHERE status = 'scheduled';
CREATE INDEX IF NOT EXISTS idx_participants_user_admission ON conference_participants (user_id, admission_state, conference_id);
CREATE INDEX IF NOT EXISTS idx_participants_history ON conference_participants (conference_id, created_at, id);
CREATE INDEX IF NOT EXISTS idx_participants_waiting ON conference_participants (conference_id, created_at, id) WHERE admission_state = 'waiting' AND status = 'waiting';
CREATE INDEX IF NOT EXISTS idx_participants_admission_decisions ON conference_participants (admission_decided_at, id) WHERE admission_decided_at IS NOT NULL;
