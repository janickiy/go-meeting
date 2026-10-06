-- Chat preferences are personal to a conference member. Leaving the chat does not
-- remove the conference, its owner, participant history, or recorded artifacts.
ALTER TABLE conferences ADD COLUMN IF NOT EXISTS chat_description VARCHAR(1000) NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS conference_chat_preferences (
    conference_id UUID NOT NULL REFERENCES conferences(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    notifications_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    left_at TIMESTAMPTZ NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (conference_id, user_id)
);
CREATE INDEX IF NOT EXISTS conference_chat_preferences_user_left
    ON conference_chat_preferences(user_id, conference_id) WHERE left_at IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS chat_messages_conference_id_id
    ON chat_messages(conference_id, id);
CREATE TABLE IF NOT EXISTS conference_chat_pins (
    conference_id UUID NOT NULL,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    message_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (conference_id, user_id, message_id),
    FOREIGN KEY (conference_id, message_id) REFERENCES chat_messages(conference_id, id) ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS conference_chat_pins_recent
    ON conference_chat_pins(conference_id, user_id, created_at DESC, message_id DESC);

-- Literal case-insensitive search is scoped to one conference before text lookup.
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE INDEX IF NOT EXISTS chat_messages_text_search
    ON chat_messages USING gin (text gin_trgm_ops) WHERE deleted_at IS NULL;

ALTER TABLE notifications DROP CONSTRAINT IF EXISTS notifications_type_check;
ALTER TABLE notifications ADD CONSTRAINT notifications_type_check CHECK(type IN (
    'conference.soon','admission.decided','recording.ready','conference.invited','conference.rescheduled',
    'conference.cancelled','transcript.ready','summary.ready','processing.failed','chat.message'
));

-- Chat notices are in-app only: the existing notification stream publishes them,
-- while email/push jobs remain reserved for meeting-level events.
CREATE OR REPLACE FUNCTION enqueue_external_notification() RETURNS TRIGGER AS $$
BEGIN
 IF NEW.type='chat.message' THEN RETURN NEW; END IF;
 INSERT INTO background_jobs(kind,entity_id,conference_id,user_id,payload,dedup_key)
 SELECT 'integrations.delivery',NEW.id,(NEW.payload->>'conferenceId')::uuid,NEW.user_id,
 jsonb_build_object('notificationId',NEW.id,'channel',channel),
 'delivery:'||NEW.id::text||':'||channel FROM (VALUES ('email'),('push')) channels(channel)
 WHERE channel <> 'email' OR NOT (NEW.type='conference.invited' AND NEW.payload ? 'invitationId')
 ON CONFLICT(dedup_key) DO NOTHING;
 RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TABLE IF NOT EXISTS chat_notification_jobs (
    message_id UUID PRIMARY KEY REFERENCES chat_messages(id) ON DELETE RESTRICT,
    conference_id UUID NOT NULL REFERENCES conferences(id) ON DELETE RESTRICT,
    cursor_participant_id UUID NULL,
    completed_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX IF NOT EXISTS chat_notification_jobs_pending ON chat_notification_jobs(created_at,message_id)
    WHERE completed_at IS NULL;
