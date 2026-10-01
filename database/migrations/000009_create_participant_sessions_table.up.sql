CREATE UNIQUE INDEX IF NOT EXISTS idx_participant_session_identity
ON conference_participants(id, conference_id, user_id);
CREATE TABLE IF NOT EXISTS participant_sessions (
    id UUID PRIMARY KEY,
    conference_id UUID NOT NULL,
    participant_id UUID NOT NULL,
    user_id UUID NOT NULL,
    connection_id UUID NOT NULL UNIQUE,
    status VARCHAR(16) NOT NULL,
    connected_at TIMESTAMPTZ NOT NULL,
    disconnected_at TIMESTAMPTZ NULL,
    last_seen_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT participant_sessions_identity_fk FOREIGN KEY(participant_id, conference_id, user_id)
        REFERENCES conference_participants(id, conference_id, user_id) ON DELETE CASCADE,
    CONSTRAINT participant_sessions_status CHECK (status IN ('connected', 'disconnected')),
    CONSTRAINT participant_sessions_dates CHECK (
        last_seen_at >= connected_at AND
        ((status = 'connected' AND disconnected_at IS NULL) OR
         (status = 'disconnected' AND disconnected_at IS NOT NULL AND disconnected_at >= last_seen_at))
    )
);
CREATE INDEX IF NOT EXISTS idx_participant_sessions_member ON participant_sessions(participant_id, connected_at DESC);
CREATE INDEX IF NOT EXISTS idx_participant_sessions_connected ON participant_sessions(connected_at) WHERE status = 'connected';
