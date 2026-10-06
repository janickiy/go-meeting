-- Extend the existing conversation/message core without moving conference data.
ALTER TABLE conversations DROP CONSTRAINT conversations_type_check;
ALTER TABLE conversations ALTER COLUMN user_low_id DROP NOT NULL;
ALTER TABLE conversations ALTER COLUMN user_high_id DROP NOT NULL;
ALTER TABLE conversations ADD COLUMN name TEXT NULL;
ALTER TABLE conversations ADD COLUMN description TEXT NOT NULL DEFAULT '';
ALTER TABLE conversations ADD COLUMN created_by UUID NULL REFERENCES users(id) ON DELETE RESTRICT;
ALTER TABLE conversations ADD COLUMN deleted_at TIMESTAMPTZ NULL;
ALTER TABLE conversations ADD COLUMN metadata_version BIGINT NOT NULL DEFAULT 1 CHECK(metadata_version>0);
ALTER TABLE conversations ADD COLUMN avatar_key TEXT NULL;
ALTER TABLE conversations ADD COLUMN avatar_version UUID NULL;
ALTER TABLE conversations ADD COLUMN group_create_request_id UUID NULL;
ALTER TABLE conversations ADD COLUMN group_request_fingerprint CHAR(64) NULL;
ALTER TABLE conversations ADD CONSTRAINT conversations_kind_check CHECK(type IN('direct','group'));
ALTER TABLE conversations ADD CONSTRAINT conversations_group_shape_check CHECK(
 (type='direct' AND user_low_id IS NOT NULL AND user_high_id IS NOT NULL AND user_low_id<user_high_id
  AND name IS NULL AND created_by IS NULL AND deleted_at IS NULL AND avatar_key IS NULL AND avatar_version IS NULL
  AND group_create_request_id IS NULL AND group_request_fingerprint IS NULL)
 OR
 (type='group' AND user_low_id IS NULL AND user_high_id IS NULL AND created_by IS NOT NULL
  AND name IS NOT NULL AND char_length(name) BETWEEN 1 AND 50 AND name=btrim(name)
  AND char_length(description)<=200 AND group_create_request_id IS NOT NULL
  AND group_request_fingerprint IS NOT NULL AND group_request_fingerprint ~ '^[a-f0-9]{64}$'
  AND ((avatar_key IS NULL)=(avatar_version IS NULL)))
);
ALTER TABLE conversations ADD CONSTRAINT conversations_group_request_unique UNIQUE(created_by,group_create_request_id);
ALTER TABLE conversation_members ADD COLUMN role TEXT NOT NULL DEFAULT 'member' CHECK(role IN('owner','admin','member'));
ALTER TABLE conversation_members ADD COLUMN left_at TIMESTAMPTZ NULL;
CREATE UNIQUE INDEX conversation_members_one_owner ON conversation_members(conversation_id) WHERE role='owner' AND left_at IS NULL;
CREATE INDEX conversation_members_active_user ON conversation_members(user_id,conversation_id) WHERE left_at IS NULL;
CREATE INDEX conversation_members_active_group ON conversation_members(conversation_id,user_id) WHERE left_at IS NULL;

-- The unique direct pair remains unchanged for existing ON CONFLICT pair creation.
-- Deferred validation lets group creation/ownership transfer be atomic transactions.
CREATE OR REPLACE FUNCTION check_direct_pair() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE ids UUID[]; cid UUID; kind TEXT; lo UUID; hi UUID; gone TIMESTAMPTZ; total BIGINT; owners BIGINT;
BEGIN
 IF TG_TABLE_NAME='conversations' THEN
  IF TG_OP='DELETE' THEN ids:=ARRAY[OLD.id]; ELSE ids:=ARRAY[NEW.id]; END IF;
 ELSIF TG_OP='INSERT' THEN ids:=ARRAY[NEW.conversation_id];
 ELSIF TG_OP='DELETE' THEN ids:=ARRAY[OLD.conversation_id];
 ELSE ids:=ARRAY[OLD.conversation_id,NEW.conversation_id]; END IF;
 FOREACH cid IN ARRAY ids LOOP
  SELECT type,user_low_id,user_high_id,deleted_at INTO kind,lo,hi,gone FROM conversations WHERE id=cid;
  IF NOT FOUND THEN CONTINUE; END IF;
  IF EXISTS(SELECT 1 FROM conversation_members m JOIN users u ON u.id=m.user_id WHERE m.conversation_id=cid AND u.guest_conference_id IS NOT NULL) THEN
   RAISE EXCEPTION 'conversation requires registered accounts' USING ERRCODE='23514';
  END IF;
  IF kind='direct' THEN
   IF (SELECT COUNT(*) FROM conversation_members WHERE conversation_id=cid)<>2
    OR EXISTS(SELECT 1 FROM conversation_members WHERE conversation_id=cid AND (user_id NOT IN(lo,hi) OR left_at IS NOT NULL OR role<>'member')) THEN
    RAISE EXCEPTION 'direct conversation requires exactly its two active registered accounts' USING ERRCODE='23514';
   END IF;
  ELSE
   SELECT count(*),count(*) FILTER(WHERE role='owner') INTO total,owners FROM conversation_members WHERE conversation_id=cid AND left_at IS NULL;
   IF total>100 OR (gone IS NULL AND (total<1 OR owners<>1)) OR (gone IS NOT NULL AND total<>0) THEN
    RAISE EXCEPTION 'active group requires one owner and at most 100 members' USING ERRCODE='23514';
   END IF;
  END IF;
 END LOOP;
 RETURN NULL;
END $$;
