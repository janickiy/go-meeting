CREATE TABLE IF NOT EXISTS notifications (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    type VARCHAR(40) NOT NULL CHECK (type IN ('conference.soon','admission.decided','recording.ready')),
    version INTEGER NOT NULL DEFAULT 1 CHECK (version = 1),
    payload JSONB NOT NULL,
    dedup_key TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    read_at TIMESTAMPTZ,
    published_at TIMESTAMPTZ,
    UNIQUE (user_id, dedup_key)
);
CREATE INDEX IF NOT EXISTS idx_notifications_user_cursor ON notifications(user_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_notifications_unread ON notifications(user_id) WHERE read_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_notifications_pending ON notifications(created_at, id) WHERE published_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_record_notifications ON record(platform_conference_id, uuid) WHERE mode = 'composite' AND status IN ('ready','partial_ready');

-- Durable product-event snapshots, separate from the media command queue.
-- Triggers capture rapid successive decisions without polling mutable facts.
CREATE TABLE IF NOT EXISTS notification_jobs (
    id BIGSERIAL PRIMARY KEY,
    kind VARCHAR(40) NOT NULL CHECK (kind IN ('admission.decided','recording.ready')),
    entity_id UUID NOT NULL,
    entity_version BIGINT NOT NULL CHECK (entity_version > 0),
    conference_id UUID NOT NULL,
    user_id UUID NULL REFERENCES users(id) ON DELETE RESTRICT,
    participant_id UUID NULL REFERENCES conference_participants(id) ON DELETE RESTRICT,
    recording_id UUID NULL REFERENCES record(uuid) ON DELETE RESTRICT,
    admission_state VARCHAR(16) NULL CHECK (admission_state IN ('admitted','rejected','kicked')),
    cursor_participant_id UUID NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    processed_at TIMESTAMPTZ NULL,
    UNIQUE(kind, entity_id, entity_version),
    CHECK (
        (kind='admission.decided' AND user_id IS NOT NULL AND participant_id IS NOT NULL AND admission_state IS NOT NULL AND recording_id IS NULL) OR
        (kind='recording.ready' AND recording_id IS NOT NULL AND user_id IS NULL AND participant_id IS NULL AND admission_state IS NULL)
    )
);
-- The non-null participant/record entity FK already restricts deletion of its
-- conference. A second direct FK would invert the recorder's record-row lock
-- against Stop's conference-row lock while the ready trigger inserts a job.
ALTER TABLE notification_jobs DROP CONSTRAINT IF EXISTS notification_jobs_conference_id_fkey;
CREATE INDEX IF NOT EXISTS idx_notification_jobs_pending ON notification_jobs(available_at,id) WHERE processed_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_participants_notification_fanout ON conference_participants(conference_id,id)
    WHERE user_id IS NOT NULL AND admission_state='admitted' AND status IN ('joined','left');

CREATE OR REPLACE FUNCTION enqueue_admission_notification() RETURNS TRIGGER AS $$
BEGIN
    IF NEW.user_id IS NOT NULL AND NEW.admission_decided_at IS NOT NULL AND NEW.admission_state IN ('admitted','rejected','kicked') THEN
        INSERT INTO notification_jobs(kind,entity_id,entity_version,conference_id,user_id,participant_id,admission_state,created_at)
        VALUES ('admission.decided',NEW.id,NEW.admission_version,NEW.conference_id,NEW.user_id,NEW.id,NEW.admission_state,NEW.admission_decided_at)
        ON CONFLICT(kind,entity_id,entity_version) DO NOTHING;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS trg_participants_notification ON conference_participants;
CREATE TRIGGER trg_participants_notification AFTER INSERT OR UPDATE OF admission_state,admission_version,admission_decided_at ON conference_participants
FOR EACH ROW EXECUTE FUNCTION enqueue_admission_notification();

CREATE OR REPLACE FUNCTION enqueue_recording_notification() RETURNS TRIGGER AS $$
BEGIN
    IF NEW.mode='composite' AND NEW.platform_conference_id IS NOT NULL AND NEW.status IN ('ready','partial_ready') AND NEW.deleted_at IS NULL THEN
        INSERT INTO notification_jobs(kind,entity_id,entity_version,conference_id,recording_id,created_at)
        VALUES ('recording.ready',NEW.uuid,1,NEW.platform_conference_id,NEW.uuid,NEW.updated_at)
        ON CONFLICT(kind,entity_id,entity_version) DO NOTHING;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS trg_record_notification ON record;
CREATE TRIGGER trg_record_notification AFTER INSERT OR UPDATE OF status ON record
FOR EACH ROW EXECUTE FUNCTION enqueue_recording_notification();

-- Idempotent one-time/startup discovery of pre-stage-five facts. Normal ticks
-- only inspect the partial pending-jobs index, never all historical recordings.
INSERT INTO notification_jobs(kind,entity_id,entity_version,conference_id,user_id,participant_id,admission_state,created_at)
SELECT 'admission.decided',id,admission_version,conference_id,user_id,id,admission_state,admission_decided_at
FROM conference_participants WHERE user_id IS NOT NULL AND admission_decided_at IS NOT NULL AND admission_state IN ('admitted','rejected','kicked')
ON CONFLICT(kind,entity_id,entity_version) DO NOTHING;
INSERT INTO notification_jobs(kind,entity_id,entity_version,conference_id,recording_id,created_at)
SELECT 'recording.ready',uuid,1,platform_conference_id,uuid,updated_at FROM record
WHERE mode='composite' AND platform_conference_id IS NOT NULL AND status IN ('ready','partial_ready') AND deleted_at IS NULL
ON CONFLICT(kind,entity_id,entity_version) DO NOTHING;
