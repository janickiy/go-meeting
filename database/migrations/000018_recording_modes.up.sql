-- Стратегии используют общую аренду, outbox и атомарную публикацию артефактов.
ALTER TABLE record ADD COLUMN IF NOT EXISTS media_started_at TIMESTAMPTZ;
ALTER TABLE record_file DROP CONSTRAINT IF EXISTS record_file_type_check;
ALTER TABLE record_file ADD CONSTRAINT record_file_type_check CHECK (file_type IN ('final_mp4','preview_jpg','debug_log','final_audio','tracks_archive'));
ALTER TABLE record DROP CONSTRAINT IF EXISTS record_composite_conference_check;
ALTER TABLE record ADD CONSTRAINT record_composite_conference_check CHECK (
 (mode='legacy' AND platform_conference_id IS NULL) OR
 (mode IN ('composite','audio_only','individual_tracks','screen_focus') AND platform_conference_id IS NOT NULL AND platform_conference_id=conference_id)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_record_one_active_conference ON record(platform_conference_id)
 WHERE mode IN ('composite','audio_only','individual_tracks','screen_focus')
 AND status IN ('starting','recording','degraded','stopping','finalizing','uploading');
CREATE OR REPLACE FUNCTION enqueue_stage_seven_recording() RETURNS TRIGGER AS $$
BEGIN
 IF NEW.mode IN ('composite','audio_only','individual_tracks','screen_focus') AND NEW.platform_conference_id IS NOT NULL AND NEW.status='ready' AND NEW.deleted_at IS NULL
 AND (TG_OP='INSERT' OR OLD.status IS DISTINCT FROM 'ready') THEN
  INSERT INTO background_jobs(kind,entity_id,conference_id,version,payload,dedup_key,max_attempts)
  VALUES ('content.transcribe',NEW.uuid,NEW.platform_conference_id,1,'{}','content.transcribe:'||NEW.uuid::text||':1',5)
  ON CONFLICT(dedup_key) DO NOTHING;
 END IF;
 RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION enqueue_recording_notification() RETURNS TRIGGER AS $$
BEGIN
 IF NEW.mode IN ('composite','audio_only','individual_tracks','screen_focus') AND NEW.platform_conference_id IS NOT NULL AND NEW.status IN ('ready','partial_ready') AND NEW.deleted_at IS NULL THEN
  INSERT INTO notification_jobs(kind,entity_id,entity_version,conference_id,recording_id,created_at)
  VALUES ('recording.ready',NEW.uuid,1,NEW.platform_conference_id,NEW.uuid,NEW.updated_at)
  ON CONFLICT(kind,entity_id,entity_version) DO NOTHING;
 END IF;
 RETURN NEW;
END;
$$ LANGUAGE plpgsql;
