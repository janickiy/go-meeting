CREATE TABLE IF NOT EXISTS conference_invitations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    conference_id UUID NOT NULL REFERENCES conferences(id) ON DELETE RESTRICT,
    requested_by UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    email VARCHAR(254) NOT NULL CHECK (email=lower(btrim(email))),
    user_id UUID NULL REFERENCES users(id) ON DELETE RESTRICT,
    status VARCHAR(16) NOT NULL DEFAULT 'queued' CHECK(status IN ('queued','sent','failed','skipped')),
    error_code VARCHAR(64) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(), sent_at TIMESTAMPTZ NULL,
    UNIQUE(conference_id,email)
);
CREATE INDEX IF NOT EXISTS idx_conference_invitations_user ON conference_invitations(conference_id,user_id);

-- Explicit invitations already save the personal notification and durable email job
-- in their transaction. Enrollment must not create a second announcement.
CREATE OR REPLACE FUNCTION enqueue_enrollment_integration() RETURNS TRIGGER AS $$
BEGIN
 IF current_setting('meetrix.explicit_invitation',TRUE)='true' THEN RETURN NEW; END IF;
 IF NEW.user_id IS NOT NULL AND NEW.role <> 'owner' AND NEW.status NOT IN ('kicked','rejected') THEN
  INSERT INTO background_jobs(kind,entity_id,conference_id,user_id,version,payload,dedup_key)
  SELECT 'integrations.event',NEW.id,NEW.conference_id,NEW.user_id,c.integration_version,jsonb_build_object('event','conference.invited'),
   'enrollment:'||NEW.id::text||':'||c.integration_version::text FROM conferences c
  WHERE c.id=NEW.conference_id AND c.status='scheduled' ON CONFLICT(dedup_key) DO NOTHING;
 END IF;
 RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION enqueue_external_notification() RETURNS TRIGGER AS $$
BEGIN
 INSERT INTO background_jobs(kind,entity_id,conference_id,user_id,payload,dedup_key)
 SELECT 'integrations.delivery',NEW.id,(NEW.payload->>'conferenceId')::uuid,NEW.user_id,
 jsonb_build_object('notificationId',NEW.id,'channel',channel),
 'delivery:'||NEW.id::text||':'||channel FROM (VALUES ('email'),('push')) channels(channel)
 WHERE channel <> 'email' OR NOT (NEW.type='conference.invited' AND NEW.payload ? 'invitationId')
 ON CONFLICT(dedup_key) DO NOTHING;
 RETURN NEW;
END;
$$ LANGUAGE plpgsql;
