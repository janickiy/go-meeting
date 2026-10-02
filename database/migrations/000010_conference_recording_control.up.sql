ALTER TABLE conference_participants
    ADD COLUMN IF NOT EXISTS microphone_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS camera_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS screen_sharing BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS microphone_blocked BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS camera_blocked BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS screen_blocked BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS media_policy_version BIGINT NOT NULL DEFAULT 1;

CREATE TABLE IF NOT EXISTS conference_moderation_audit (
    id BIGSERIAL PRIMARY KEY,
    conference_id UUID NOT NULL REFERENCES conferences(id) ON DELETE CASCADE,
    actor_id UUID NOT NULL REFERENCES conference_participants(id),
    participant_id UUID NOT NULL REFERENCES conference_participants(id),
    action VARCHAR(16) NOT NULL,
    blocked BOOLEAN,
    role VARCHAR(16),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE record
    ADD COLUMN IF NOT EXISTS mode VARCHAR(24) NOT NULL DEFAULT 'legacy',
    ADD COLUMN IF NOT EXISTS platform_conference_id UUID REFERENCES conferences(id) ON DELETE RESTRICT,
    ADD COLUMN IF NOT EXISTS recorder_token UUID,
    ADD COLUMN IF NOT EXISTS recorder_lease_until TIMESTAMPTZ;
DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'record_composite_conference_check') THEN
        ALTER TABLE record ADD CONSTRAINT record_composite_conference_check CHECK (
            (mode = 'legacy' AND platform_conference_id IS NULL) OR
            (mode = 'composite' AND platform_conference_id IS NOT NULL AND platform_conference_id = conference_id)
        );
    END IF;
END $$;
CREATE UNIQUE INDEX IF NOT EXISTS idx_record_one_active_composite
    ON record(platform_conference_id) WHERE mode = 'composite'
    AND status IN ('starting','recording','degraded','stopping','finalizing','uploading');

CREATE TABLE IF NOT EXISTS recording_outbox (
    id BIGSERIAL PRIMARY KEY,
    record_id UUID NOT NULL REFERENCES record(uuid) ON DELETE CASCADE,
    command_type VARCHAR(16) NOT NULL CHECK(command_type IN ('record.start','record.stop')),
    reason VARCHAR(64) NOT NULL DEFAULT '',
    claimed_until TIMESTAMPTZ,
    claim_token UUID,
    published_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(record_id, command_type)
);
CREATE INDEX IF NOT EXISTS idx_recording_outbox_pending ON recording_outbox(id) WHERE published_at IS NULL;
