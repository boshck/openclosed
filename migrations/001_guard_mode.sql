CREATE TABLE IF NOT EXISTS chats (
    chat_id BIGINT PRIMARY KEY,
    chat_type TEXT NOT NULL,
    title TEXT NOT NULL DEFAULT '',
    username TEXT NOT NULL DEFAULT '',
    guard_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    first_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS allowlist (
    chat_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    note TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (chat_id, user_id)
);

CREATE TABLE IF NOT EXISTS known_members (
    chat_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    username TEXT NOT NULL DEFAULT '',
    first_name TEXT NOT NULL DEFAULT '',
    last_name TEXT NOT NULL DEFAULT '',
    is_bot BOOLEAN NOT NULL DEFAULT FALSE,
    last_status TEXT NOT NULL DEFAULT '',
    first_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (chat_id, user_id)
);

CREATE TABLE IF NOT EXISTS removal_queue (
    chat_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    status TEXT NOT NULL,
    reason TEXT NOT NULL,
    source_update_id BIGINT,
    attempts INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (chat_id, user_id),
    CONSTRAINT removal_queue_status_check CHECK (status IN ('pending', 'error', 'done'))
);

CREATE INDEX IF NOT EXISTS removal_queue_due_idx
    ON removal_queue (status, next_attempt_at);

CREATE TABLE IF NOT EXISTS moderation_events (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    chat_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL DEFAULT 0,
    event_type TEXT NOT NULL,
    update_id BIGINT,
    message_id BIGINT,
    status TEXT NOT NULL,
    error TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS moderation_events_chat_idx
    ON moderation_events (chat_id, created_at);

CREATE TABLE IF NOT EXISTS bot_state (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
