-- +goose Up
CREATE TABLE investigation_thread_messages (
    id UUID PRIMARY KEY,
    thread_id UUID NOT NULL,
    type TEXT NOT NULL DEFAULT 'comment',
    source TEXT NOT NULL DEFAULT 'user',
    message TEXT NOT NULL,
    internal BOOLEAN NOT NULL DEFAULT false,
    edited BOOLEAN NOT NULL DEFAULT false,
    user_id TEXT DEFAULT '',
    username TEXT DEFAULT '',
    agent_type TEXT DEFAULT '',
    mm_post_id TEXT DEFAULT '',
    slack_message_ts TEXT DEFAULT '',
    reply_to_message_id TEXT DEFAULT '',
    mentions JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT fk_investigation_thread_messages_thread FOREIGN KEY (thread_id) REFERENCES investigation_threads(id) ON DELETE CASCADE
);

CREATE INDEX idx_investigation_thread_messages_thread_created ON investigation_thread_messages (thread_id, created_at);

-- +goose Down
DROP TABLE IF EXISTS investigation_thread_messages;
