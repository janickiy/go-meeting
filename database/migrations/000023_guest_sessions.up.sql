-- Guest identities retain their meeting history but cannot become account sessions.
ALTER TABLE users ADD COLUMN guest_conference_id UUID NULL;
ALTER TABLE users ADD CONSTRAINT users_guest_not_admin
    CHECK (guest_conference_id IS NULL OR is_admin = FALSE);
