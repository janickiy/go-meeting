-- Персональные настройки не меняют общие сообщения и доступ собеседника.
ALTER TABLE conversation_members
 ADD COLUMN notifications_enabled BOOLEAN NOT NULL DEFAULT true,
 ADD COLUMN history_cleared_through BIGINT NOT NULL DEFAULT 0 CHECK(history_cleared_through>=0),
 ADD COLUMN hidden_at TIMESTAMPTZ NULL;
CREATE INDEX conversation_members_visible_user ON conversation_members(user_id,conversation_id)
 WHERE left_at IS NULL AND hidden_at IS NULL;
