CREATE TABLE IF NOT EXISTS chat_messages (
    id UUID PRIMARY KEY,
    sequence BIGSERIAL NOT NULL,
    conference_id UUID NOT NULL REFERENCES conferences(id) ON DELETE RESTRICT,
    sender_user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    client_request_id UUID NOT NULL,
    request_fingerprint CHAR(64) NOT NULL,
    text TEXT NOT NULL DEFAULT '',
    reply_to_id UUID NULL REFERENCES chat_messages(id) ON DELETE RESTRICT,
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    deleted_at TIMESTAMPTZ NULL,
    UNIQUE (conference_id, sender_user_id, client_request_id),
    CHECK (char_length(text) <= 4000)
);
-- Sequence allocation is global, but uniqueness/index access is conference-
-- scoped. A standalone sequence index tempted the planner to scan unrelated
-- newer conferences for LIMIT queries instead of the scoped cursor index.
ALTER TABLE chat_messages DROP CONSTRAINT IF EXISTS chat_messages_sequence_key;
DROP INDEX IF EXISTS chat_messages_cursor;
CREATE UNIQUE INDEX IF NOT EXISTS chat_messages_conference_sequence ON chat_messages(conference_id, sequence DESC);
CREATE INDEX IF NOT EXISTS chat_messages_unread ON chat_messages(conference_id, sequence) INCLUDE (sender_user_id) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS chat_read_states (
    conference_id UUID NOT NULL REFERENCES conferences(id) ON DELETE RESTRICT,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    last_read_message_id UUID NOT NULL REFERENCES chat_messages(id) ON DELETE RESTRICT,
    last_read_sequence BIGINT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY(conference_id, user_id)
);

CREATE TABLE IF NOT EXISTS chat_attachments (
    id UUID PRIMARY KEY,
    conference_id UUID NOT NULL REFERENCES conferences(id) ON DELETE RESTRICT,
    owner_user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    client_request_id UUID NOT NULL,
    filename VARCHAR(720) NOT NULL,
    mime_type VARCHAR(80) NOT NULL,
    size BIGINT NOT NULL CHECK(size BETWEEN 1 AND 10485760),
    checksum CHAR(64) NOT NULL DEFAULT '',
    status VARCHAR(16) NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','ready','attached','expired')),
    message_id UUID NULL REFERENCES chat_messages(id) ON DELETE RESTRICT,
    object_key TEXT NOT NULL DEFAULT '',
    upload_token UUID NULL,
    upload_lease_until TIMESTAMPTZ NULL,
    uploaded_at TIMESTAMPTZ NULL,
    cleaned_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    expires_at TIMESTAMPTZ NOT NULL,
    UNIQUE(conference_id, owner_user_id, client_request_id),
    CHECK ((status='attached') = (message_id IS NOT NULL))
);
CREATE INDEX IF NOT EXISTS chat_attachments_message ON chat_attachments(message_id) WHERE message_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS chat_attachments_cleanup ON chat_attachments(expires_at, id) WHERE cleaned_at IS NULL;
CREATE INDEX IF NOT EXISTS chat_attachments_owner_pending ON chat_attachments(conference_id, owner_user_id) WHERE status IN ('pending','ready');
