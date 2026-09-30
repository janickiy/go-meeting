CREATE TABLE IF NOT EXISTS conference_participants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    conference_id UUID NOT NULL REFERENCES conferences(id) ON DELETE CASCADE,
    user_id UUID NULL REFERENCES users(id) ON DELETE RESTRICT,
    display_name VARCHAR(100) NOT NULL,
    role VARCHAR(16) NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'left',
    joined_at TIMESTAMPTZ NULL,
    left_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT conference_participants_membership_unique UNIQUE(conference_id, user_id),
    CONSTRAINT conference_participants_role_check CHECK (role IN ('owner', 'co_host', 'participant', 'guest')),
    CONSTRAINT conference_participants_status_check CHECK (status IN ('joined', 'left', 'waiting', 'rejected', 'kicked')),
    CONSTRAINT conference_participants_identity_check CHECK (
        (user_id IS NULL AND role = 'guest') OR
        (user_id IS NOT NULL AND role IN ('owner', 'co_host', 'participant'))
    ),
    CONSTRAINT conference_participants_joined_check CHECK (
        status <> 'joined' OR (joined_at IS NOT NULL AND left_at IS NULL)
    ),
    CONSTRAINT conference_participants_left_check CHECK (
        left_at IS NULL OR (joined_at IS NOT NULL AND left_at >= joined_at)
    )
);
CREATE INDEX IF NOT EXISTS idx_conference_participants_conference ON conference_participants(conference_id);
CREATE INDEX IF NOT EXISTS idx_conference_participants_user ON conference_participants(user_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_conference_participants_single_owner
ON conference_participants(conference_id) WHERE role = 'owner';
DROP TRIGGER IF EXISTS trg_conference_participants_updated_at ON conference_participants;
CREATE TRIGGER trg_conference_participants_updated_at BEFORE UPDATE ON conference_participants
FOR EACH ROW EXECUTE FUNCTION set_updated_at();
