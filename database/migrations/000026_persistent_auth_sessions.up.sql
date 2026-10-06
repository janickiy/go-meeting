-- Account sessions are revoked by explicit logout, not elapsed or idle time.
CREATE TABLE IF NOT EXISTS auth_sessions (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at TIMESTAMPTZ NULL
);
CREATE INDEX IF NOT EXISTS idx_auth_sessions_user ON auth_sessions(user_id);

-- Concurrent legacy-token upgrades share one SID while receiving distinct cookies.
CREATE TABLE IF NOT EXISTS auth_session_tokens (
    token_hash CHAR(64) PRIMARY KEY CHECK (token_hash ~ '^[a-f0-9]{64}$'),
    session_id UUID NOT NULL REFERENCES auth_sessions(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_auth_session_tokens_session ON auth_session_tokens(session_id);

-- Preserve logout markers so an in-flight legacy upgrade cannot resurrect sign-in.
CREATE TABLE IF NOT EXISTS auth_legacy_tokens (
    token_hash CHAR(64) PRIMARY KEY CHECK (token_hash ~ '^[a-f0-9]{64}$'),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    session_id UUID NULL REFERENCES auth_sessions(id) ON DELETE CASCADE,
    revoked_at TIMESTAMPTZ NULL
);
