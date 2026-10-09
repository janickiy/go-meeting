-- Нельзя терять персональные ограничения истории и настройки при откате.
DO $$ BEGIN
 LOCK TABLE conversation_members IN ACCESS EXCLUSIVE MODE;
 IF EXISTS(SELECT 1 FROM conversation_members WHERE notifications_enabled=false
  OR history_cleared_through>0 OR hidden_at IS NOT NULL) THEN
  RAISE EXCEPTION 'personal conversation preferences exist; retain schema and use a compatible forward fix' USING ERRCODE='23514';
 END IF;
END $$;
DROP INDEX conversation_members_visible_user;
ALTER TABLE conversation_members DROP COLUMN hidden_at,
 DROP COLUMN history_cleared_through, DROP COLUMN notifications_enabled;
