-- Additive adapter: existing conference chat and object paths are untouched.
CREATE TABLE conversations (
 id UUID PRIMARY KEY, type TEXT NOT NULL DEFAULT 'direct' CHECK(type='direct'),
 user_low_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
 user_high_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 last_message_at TIMESTAMPTZ NULL, last_message_id UUID NULL,
 CHECK(user_low_id<user_high_id), UNIQUE(user_low_id,user_high_id)
);
CREATE INDEX conversations_activity ON conversations((COALESCE(last_message_at,created_at)) DESC,id DESC);
CREATE TABLE conversation_members (
 conversation_id UUID NOT NULL REFERENCES conversations(id) ON DELETE RESTRICT,
 user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
 last_read_message_id UUID NULL, last_read_sequence BIGINT NOT NULL DEFAULT 0 CHECK(last_read_sequence>=0),
 joined_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(), updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(conversation_id,user_id),
 CHECK((last_read_message_id IS NULL)=(last_read_sequence=0))
);
CREATE INDEX conversation_members_user ON conversation_members(user_id,conversation_id);
CREATE FUNCTION check_direct_pair() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE cid UUID; lo UUID; hi UUID;
BEGIN
 IF TG_TABLE_NAME='conversations' THEN cid:=NEW.id; ELSIF TG_OP='DELETE' THEN cid:=OLD.conversation_id; ELSE cid:=NEW.conversation_id; END IF;
 SELECT user_low_id,user_high_id INTO lo,hi FROM conversations WHERE id=cid;
 IF NOT FOUND THEN RETURN NULL; END IF;
 IF EXISTS(SELECT 1 FROM users WHERE id IN (lo,hi) AND guest_conference_id IS NOT NULL)
 OR (SELECT COUNT(*) FROM conversation_members WHERE conversation_id=cid)<>2
 OR EXISTS(SELECT 1 FROM conversation_members WHERE conversation_id=cid AND user_id NOT IN(lo,hi)) THEN
   RAISE EXCEPTION 'direct conversation requires exactly its two registered accounts' USING ERRCODE='23514';
 END IF;
 RETURN NULL;
END $$;
CREATE CONSTRAINT TRIGGER direct_pair_members AFTER INSERT OR UPDATE OR DELETE ON conversation_members DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION check_direct_pair();
CREATE CONSTRAINT TRIGGER direct_pair_conversation AFTER INSERT OR UPDATE ON conversations DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION check_direct_pair();
CREATE TABLE conversation_messages (
 id UUID PRIMARY KEY, sequence BIGSERIAL NOT NULL,
 conversation_id UUID NOT NULL REFERENCES conversations(id) ON DELETE RESTRICT,
 sender_user_id UUID NOT NULL, client_request_id UUID NOT NULL, request_fingerprint CHAR(64) NOT NULL,
 text TEXT NOT NULL DEFAULT '' CHECK(char_length(text)<=4000), reply_to_id UUID NULL,
 version BIGINT NOT NULL DEFAULT 1 CHECK(version>0),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(), updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(), deleted_at TIMESTAMPTZ NULL,
 UNIQUE(conversation_id,sender_user_id,client_request_id), UNIQUE(conversation_id,id), UNIQUE(conversation_id,id,sequence),
 FOREIGN KEY(conversation_id,sender_user_id) REFERENCES conversation_members(conversation_id,user_id) ON DELETE RESTRICT,
 FOREIGN KEY(conversation_id,reply_to_id) REFERENCES conversation_messages(conversation_id,id) ON DELETE RESTRICT
);
CREATE UNIQUE INDEX conversation_messages_cursor ON conversation_messages(conversation_id,sequence DESC);
CREATE INDEX conversation_messages_unread ON conversation_messages(conversation_id,sequence) INCLUDE(sender_user_id) WHERE deleted_at IS NULL;
ALTER TABLE conversation_members ADD CONSTRAINT conversation_members_read_fkey FOREIGN KEY(conversation_id,last_read_message_id,last_read_sequence) REFERENCES conversation_messages(conversation_id,id,sequence) ON DELETE RESTRICT;
ALTER TABLE conversations ADD CONSTRAINT conversation_last_message_fkey FOREIGN KEY(id,last_message_id) REFERENCES conversation_messages(conversation_id,id) ON DELETE RESTRICT;
CREATE TABLE conversation_attachments (
 id UUID PRIMARY KEY, conversation_id UUID NOT NULL REFERENCES conversations(id) ON DELETE RESTRICT,
 owner_user_id UUID NOT NULL, client_request_id UUID NOT NULL,
 filename VARCHAR(720) NOT NULL, mime_type VARCHAR(80) NOT NULL, size BIGINT NOT NULL CHECK(size BETWEEN 1 AND 10485760),
 checksum CHAR(64) NOT NULL DEFAULT '', status VARCHAR(16) NOT NULL DEFAULT 'pending' CHECK(status IN('pending','ready','attached','expired')),
 message_id UUID NULL, object_key TEXT NOT NULL DEFAULT '', upload_token UUID NULL, upload_lease_until TIMESTAMPTZ NULL,
 uploaded_at TIMESTAMPTZ NULL, cleaned_at TIMESTAMPTZ NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(), updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(), expires_at TIMESTAMPTZ NOT NULL,
 UNIQUE(conversation_id,owner_user_id,client_request_id), CHECK((status='attached')=(message_id IS NOT NULL)),
 FOREIGN KEY(conversation_id,owner_user_id) REFERENCES conversation_members(conversation_id,user_id) ON DELETE RESTRICT,
 FOREIGN KEY(conversation_id,message_id) REFERENCES conversation_messages(conversation_id,id) ON DELETE RESTRICT
);
CREATE INDEX conversation_attachments_message ON conversation_attachments(message_id) WHERE message_id IS NOT NULL;
CREATE INDEX conversation_attachments_cleanup ON conversation_attachments(expires_at,id) WHERE cleaned_at IS NULL;
CREATE INDEX conversation_attachments_owner_pending ON conversation_attachments(conversation_id,owner_user_id) WHERE status IN('pending','ready');
