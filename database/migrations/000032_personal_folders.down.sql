-- A rollback must not silently discard account-owned organization metadata.
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM folders) THEN
  RAISE EXCEPTION 'personal folders must be preserved before rollback';
 END IF;
END $$;
DROP TABLE folder_conferences;
DROP TABLE folder_conversations;
DROP TABLE folders;
