ALTER TABLE participant_sessions
    ADD COLUMN IF NOT EXISTS media_sequence BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS microphone_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS camera_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS screen_sharing BOOLEAN NOT NULL DEFAULT FALSE;
DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'participant_sessions_media_sequence_check') THEN
        ALTER TABLE participant_sessions ADD CONSTRAINT participant_sessions_media_sequence_check
            CHECK(media_sequence >= 0 AND media_sequence <= 9007199254740991);
    END IF;
END $$;
