-- Explicit rollback only; does not touch the conference chat.
DROP TABLE IF EXISTS conversation_attachments;
ALTER TABLE conversations DROP CONSTRAINT IF EXISTS conversation_last_message_fkey;
ALTER TABLE conversation_members DROP CONSTRAINT IF EXISTS conversation_members_read_fkey;
DROP TABLE IF EXISTS conversation_messages CASCADE;
DROP TABLE IF EXISTS conversation_members;
DROP TABLE IF EXISTS conversations;
DROP FUNCTION IF EXISTS check_direct_pair();
