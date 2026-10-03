-- Откат восстанавливает структуру, но не удалённые исторические счётчики.
ALTER TABLE participant_analytics
    ADD COLUMN hand_raises BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN last_hand_at TIMESTAMPTZ;
