PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS slack_threads (
    id TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL,
    channel_id TEXT NOT NULL,
    thread_ts TEXT NOT NULL,
    parent_message_id TEXT NOT NULL DEFAULT '',
    last_message_ts TEXT NOT NULL DEFAULT '',
    context_version INTEGER NOT NULL DEFAULT 0,
    sync_status TEXT NOT NULL,
    last_synced_at INTEGER,
    requested_analysis_version INTEGER NOT NULL DEFAULT 0,
    latest_workflow_run_id TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE (workspace_id, channel_id, thread_ts)
);

CREATE INDEX IF NOT EXISTS slack_threads_updated_at_idx ON slack_threads(updated_at DESC);

CREATE TABLE IF NOT EXISTS slack_messages (
    id TEXT PRIMARY KEY,
    thread_id TEXT NOT NULL,
    workspace_id TEXT NOT NULL,
    channel_id TEXT NOT NULL,
    slack_message_ts TEXT NOT NULL,
    thread_ts TEXT NOT NULL,
    author_id TEXT NOT NULL DEFAULT '',
    text TEXT NOT NULL DEFAULT '',
    content_hash TEXT NOT NULL,
    is_parent INTEGER NOT NULL DEFAULT 0,
    is_deleted INTEGER NOT NULL DEFAULT 0,
    edited_at INTEGER,
    deleted_at INTEGER,
    slack_created_at INTEGER NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE (workspace_id, channel_id, slack_message_ts),
    FOREIGN KEY (thread_id) REFERENCES slack_threads(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS slack_messages_thread_idx ON slack_messages(thread_id, slack_created_at, slack_message_ts);

CREATE TABLE IF NOT EXISTS slack_events (
    event_id TEXT PRIMARY KEY,
    status TEXT NOT NULL,
    received_at INTEGER NOT NULL,
    claimed_at INTEGER NOT NULL,
    processed_at INTEGER
);
