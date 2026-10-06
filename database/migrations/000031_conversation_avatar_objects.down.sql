-- Never discard the only durable cleanup ledger while object-store bytes remain.
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM conversation_avatar_objects WHERE state<>'cleaned')
 OR EXISTS(SELECT 1 FROM conversations WHERE avatar_version IS NOT NULL) THEN
  RAISE EXCEPTION 'conversation avatar objects require reconciliation before rollback';
 END IF;
END $$;
DROP TABLE conversation_avatar_objects;
DROP INDEX conversations_avatar_reference;
