-- Object metadata survives an interrupted upload; bytes stay in the private bucket.
CREATE TABLE conversation_avatar_objects (
 id UUID PRIMARY KEY,
 conversation_id UUID NOT NULL REFERENCES conversations(id) ON DELETE RESTRICT,
 object_key TEXT NOT NULL UNIQUE,
 content_type TEXT NOT NULL CHECK(content_type IN('image/jpeg','image/png')),
 size BIGINT NOT NULL CHECK(size BETWEEN 1 AND 2097152),
 checksum CHAR(64) NOT NULL CHECK(checksum ~ '^[a-f0-9]{64}$'),
 state TEXT NOT NULL DEFAULT 'pending' CHECK(state IN('pending','active','retired','cleaning','cleaned')),
 cleanup_after TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()+INTERVAL '10 minutes',
 cleanup_token UUID NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 CHECK(object_key='avatars/conversations/'||conversation_id::text||'/'||id::text),
 CHECK((state='cleaning')=(cleanup_token IS NOT NULL))
);
CREATE INDEX conversation_avatar_cleanup ON conversation_avatar_objects(cleanup_after,id) WHERE state IN('pending','retired','cleaning');
CREATE INDEX conversation_avatar_current ON conversation_avatar_objects(conversation_id) WHERE state='active';
CREATE INDEX conversation_avatar_cleaned_retention ON conversation_avatar_objects(updated_at,id) WHERE state='cleaned';
CREATE INDEX conversations_avatar_reference ON conversations(avatar_version) WHERE avatar_version IS NOT NULL;
