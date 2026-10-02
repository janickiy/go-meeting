CREATE TABLE IF NOT EXISTS notification_preferences (
    user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE RESTRICT,
    invitation BOOLEAN NOT NULL DEFAULT TRUE, reminder BOOLEAN NOT NULL DEFAULT TRUE,
    recording BOOLEAN NOT NULL DEFAULT TRUE, summary BOOLEAN NOT NULL DEFAULT TRUE,
    email BOOLEAN NOT NULL DEFAULT FALSE, push BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS push_devices (
    id UUID PRIMARY KEY, user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    platform TEXT NOT NULL CHECK(platform IN ('web','ios','android')),
    provider TEXT NOT NULL, token_ciphertext TEXT NOT NULL, token_fingerprint TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at TIMESTAMPTZ, disabled_at TIMESTAMPTZ,
    UNIQUE(user_id,provider,token_fingerprint)
);
CREATE INDEX IF NOT EXISTS idx_push_devices_active ON push_devices(user_id,id) WHERE revoked_at IS NULL AND disabled_at IS NULL;
CREATE TABLE IF NOT EXISTS calendar_connections (
    id UUID PRIMARY KEY, user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    provider TEXT NOT NULL, calendar_id TEXT NOT NULL DEFAULT 'primary',
    status TEXT NOT NULL CHECK(status IN ('connected','revoked')),
    access_ciphertext TEXT NOT NULL DEFAULT '', refresh_ciphertext TEXT NOT NULL DEFAULT '',
    scopes TEXT NOT NULL DEFAULT '', expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(user_id,provider,calendar_id)
);
CREATE TABLE IF NOT EXISTS calendar_oauth_states (
    id UUID PRIMARY KEY, user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    provider TEXT NOT NULL, state_hash TEXT NOT NULL UNIQUE, verifier_ciphertext TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL, used_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_calendar_oauth_expiry ON calendar_oauth_states(expires_at);
CREATE TABLE IF NOT EXISTS calendar_event_mappings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(), conference_id UUID NOT NULL REFERENCES conferences(id) ON DELETE RESTRICT,
    connection_id UUID NOT NULL REFERENCES calendar_connections(id) ON DELETE RESTRICT,
    provider TEXT NOT NULL, external_calendar_id TEXT NOT NULL, external_event_id TEXT NOT NULL DEFAULT '',
    sync_status TEXT NOT NULL CHECK(sync_status IN ('pending','synced','cancelled','failed')),
    source_version BIGINT NOT NULL DEFAULT 0, last_synced_at TIMESTAMPTZ,
    UNIQUE(conference_id,connection_id)
);
CREATE TABLE IF NOT EXISTS integration_deliveries (
    job_id UUID PRIMARY KEY REFERENCES background_jobs(id) ON DELETE RESTRICT,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    channel TEXT NOT NULL CHECK(channel IN ('email','push')),
    status TEXT NOT NULL CHECK(status IN ('delivered','skipped','failed')),
    error_code TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE notifications DROP CONSTRAINT IF EXISTS notifications_type_check;
ALTER TABLE notifications ADD CONSTRAINT notifications_type_check CHECK(type IN (
 'conference.soon','admission.decided','recording.ready','conference.invited','conference.rescheduled',
 'conference.cancelled','transcript.ready','summary.ready','processing.failed'));

-- Durable external delivery is separate from the recorder/media Rabbit queue.
CREATE OR REPLACE FUNCTION enqueue_external_notification() RETURNS TRIGGER AS $$
BEGIN
 INSERT INTO background_jobs(kind,entity_id,conference_id,user_id,payload,dedup_key)
 SELECT 'integrations.delivery',NEW.id,(NEW.payload->>'conferenceId')::uuid,NEW.user_id,
 jsonb_build_object('notificationId',NEW.id,'channel',channel),
 'delivery:'||NEW.id::text||':'||channel FROM (VALUES ('email'),('push')) channels(channel)
 ON CONFLICT(dedup_key) DO NOTHING;
 RETURN NEW;
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS trg_notification_external ON notifications;
CREATE TRIGGER trg_notification_external AFTER INSERT ON notifications FOR EACH ROW EXECUTE FUNCTION enqueue_external_notification();

ALTER TABLE conferences ADD COLUMN IF NOT EXISTS integration_version BIGINT NOT NULL DEFAULT 1;
ALTER TABLE conferences ADD COLUMN IF NOT EXISTS integration_version_at TIMESTAMPTZ NOT NULL DEFAULT now();
CREATE OR REPLACE FUNCTION version_conference_integration() RETURNS TRIGGER AS $$
BEGIN
 IF NEW.scheduled_at IS DISTINCT FROM OLD.scheduled_at OR NEW.planned_duration_min IS DISTINCT FROM OLD.planned_duration_min
 OR NEW.title IS DISTINCT FROM OLD.title OR (NEW.status='cancelled' AND OLD.status <> 'cancelled') THEN
  NEW.integration_version=OLD.integration_version+1;
  NEW.integration_version_at=clock_timestamp();
 END IF;
 RETURN NEW;
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS trg_conference_integration_version ON conferences;
CREATE TRIGGER trg_conference_integration_version BEFORE UPDATE ON conferences FOR EACH ROW EXECUTE FUNCTION version_conference_integration();
CREATE OR REPLACE FUNCTION enqueue_conference_integration() RETURNS TRIGGER AS $$
DECLARE event_name TEXT;
BEGIN
 IF TG_OP='INSERT' AND NEW.status='scheduled' THEN event_name='conference.invited';
 ELSIF TG_OP='UPDATE' AND NEW.status='cancelled' AND OLD.status <> 'cancelled' THEN event_name='conference.cancelled';
 ELSIF TG_OP='UPDATE' AND NEW.status='scheduled' AND NEW.integration_version <> OLD.integration_version THEN event_name='conference.rescheduled';
 ELSE RETURN NEW; END IF;
 INSERT INTO background_jobs(kind,entity_id,conference_id,version,payload,dedup_key)
 VALUES('integrations.conference',NEW.id,NEW.id,NEW.integration_version,jsonb_build_object('event',event_name),
 'conference:'||NEW.id::text||':'||NEW.integration_version::text) ON CONFLICT(dedup_key) DO NOTHING;
 RETURN NEW;
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS trg_conference_integrations ON conferences;
CREATE TRIGGER trg_conference_integrations AFTER INSERT OR UPDATE ON conferences FOR EACH ROW EXECUTE FUNCTION enqueue_conference_integration();
CREATE OR REPLACE FUNCTION enqueue_enrollment_integration() RETURNS TRIGGER AS $$
BEGIN
 IF NEW.user_id IS NOT NULL AND NEW.role <> 'owner' AND NEW.status NOT IN ('kicked','rejected') THEN
  INSERT INTO background_jobs(kind,entity_id,conference_id,user_id,version,payload,dedup_key)
  SELECT 'integrations.event',NEW.id,NEW.conference_id,NEW.user_id,c.integration_version,jsonb_build_object('event','conference.invited'),
   'enrollment:'||NEW.id::text||':'||c.integration_version::text FROM conferences c
  WHERE c.id=NEW.conference_id AND c.status='scheduled' ON CONFLICT(dedup_key) DO NOTHING;
 END IF;
 RETURN NEW;
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS trg_enrollment_integrations ON conference_participants;
CREATE TRIGGER trg_enrollment_integrations AFTER INSERT ON conference_participants FOR EACH ROW EXECUTE FUNCTION enqueue_enrollment_integration();

-- Capture already scheduled meetings without replaying historical recording-ready emails.
INSERT INTO background_jobs(kind,entity_id,conference_id,version,payload,dedup_key)
SELECT 'integrations.conference',id,id,integration_version,jsonb_build_object('event','conference.invited'),
'conference:'||id::text||':'||integration_version::text FROM conferences WHERE status='scheduled'
ON CONFLICT(dedup_key) DO NOTHING;
