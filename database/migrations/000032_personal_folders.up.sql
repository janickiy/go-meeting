CREATE TABLE folders (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 50),
    name_key text NOT NULL CHECK (name_key <> ''),
    position integer NOT NULL CHECK (position >= 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT folders_owner_name_key UNIQUE (user_id, name_key),
    CONSTRAINT folders_owner_position UNIQUE (user_id, position) DEFERRABLE INITIALLY DEFERRED
);
CREATE INDEX idx_folders_owner_order ON folders(user_id, position, id);

CREATE TABLE folder_conversations (
    folder_id uuid NOT NULL REFERENCES folders(id) ON DELETE CASCADE,
    conversation_id uuid NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (folder_id, conversation_id)
);
CREATE INDEX idx_folder_conversations_target ON folder_conversations(conversation_id, folder_id);

CREATE TABLE folder_conferences (
    folder_id uuid NOT NULL REFERENCES folders(id) ON DELETE CASCADE,
    conference_id uuid NOT NULL REFERENCES conferences(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (folder_id, conference_id)
);
CREATE INDEX idx_folder_conferences_target ON folder_conferences(conference_id, folder_id);
