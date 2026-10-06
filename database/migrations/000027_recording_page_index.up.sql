-- Conference recording pages and history summaries filter by the platform FK.
-- Legacy conference_id and status-specific partial indexes do not serve this
-- predicate. Keep the index narrow; retention remains a query-time check.
CREATE INDEX IF NOT EXISTS idx_record_platform_page
    ON record(platform_conference_id, created_at DESC, id DESC)
    WHERE deleted_at IS NULL
      AND mode IN ('composite','audio_only','individual_tracks','screen_focus');
