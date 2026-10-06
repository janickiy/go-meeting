-- A downgrade is only safe before any group data exists. Never discard group history.
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM conversations WHERE type='group') THEN
  RAISE EXCEPTION 'group conversations exist; retain schema and use a compatible forward fix';
 END IF;
END $$;
DROP INDEX conversation_members_one_owner;
DROP INDEX conversation_members_active_user;
DROP INDEX conversation_members_active_group;
ALTER TABLE conversation_members DROP COLUMN role;
ALTER TABLE conversation_members DROP COLUMN left_at;
ALTER TABLE conversations DROP CONSTRAINT conversations_group_request_unique;
ALTER TABLE conversations DROP CONSTRAINT conversations_group_shape_check;
ALTER TABLE conversations DROP CONSTRAINT conversations_kind_check;
ALTER TABLE conversations DROP COLUMN name;
ALTER TABLE conversations DROP COLUMN description;
ALTER TABLE conversations DROP COLUMN created_by;
ALTER TABLE conversations DROP COLUMN deleted_at;
ALTER TABLE conversations DROP COLUMN metadata_version;
ALTER TABLE conversations DROP COLUMN avatar_key;
ALTER TABLE conversations DROP COLUMN avatar_version;
ALTER TABLE conversations DROP COLUMN group_create_request_id;
ALTER TABLE conversations DROP COLUMN group_request_fingerprint;
ALTER TABLE conversations ALTER COLUMN user_low_id SET NOT NULL;
ALTER TABLE conversations ALTER COLUMN user_high_id SET NOT NULL;
ALTER TABLE conversations ADD CONSTRAINT conversations_type_check CHECK(type='direct');
CREATE OR REPLACE FUNCTION check_direct_pair() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE cid UUID; lo UUID; hi UUID;
BEGIN
 IF TG_TABLE_NAME='conversations' THEN cid:=NEW.id; ELSIF TG_OP='DELETE' THEN cid:=OLD.conversation_id; ELSE cid:=NEW.conversation_id; END IF;
 SELECT user_low_id,user_high_id INTO lo,hi FROM conversations WHERE id=cid;
 IF NOT FOUND THEN RETURN NULL; END IF;
 IF EXISTS(SELECT 1 FROM users WHERE id IN(lo,hi) AND guest_conference_id IS NOT NULL)
  OR (SELECT count(*) FROM conversation_members WHERE conversation_id=cid)<>2
  OR EXISTS(SELECT 1 FROM conversation_members WHERE conversation_id=cid AND user_id NOT IN(lo,hi)) THEN
  RAISE EXCEPTION 'direct conversation requires exactly its two registered accounts' USING ERRCODE='23514';
 END IF;
 RETURN NULL;
END $$;
