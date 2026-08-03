PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS slack_attachments (
    id TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL,
    message_id TEXT NOT NULL,
    thread_id TEXT NOT NULL,
    slack_file_id TEXT NOT NULL,
    filename TEXT NOT NULL,
    mime_type TEXT NOT NULL DEFAULT '',
    size_bytes INTEGER NOT NULL DEFAULT 0,
    checksum TEXT NOT NULL DEFAULT '',
    local_path TEXT NOT NULL DEFAULT '',
    download_status TEXT NOT NULL DEFAULT 'pending',
    extraction_status TEXT NOT NULL DEFAULT 'pending',
    extractor_name TEXT NOT NULL DEFAULT '',
    extractor_version TEXT NOT NULL DEFAULT '',
    is_removed INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE (workspace_id, slack_file_id),
    FOREIGN KEY (message_id) REFERENCES slack_messages(id) ON DELETE CASCADE,
    FOREIGN KEY (thread_id) REFERENCES slack_threads(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS slack_attachments_thread_idx ON slack_attachments(thread_id, created_at);
CREATE INDEX IF NOT EXISTS slack_attachments_message_idx ON slack_attachments(message_id);

CREATE TABLE IF NOT EXISTS extracted_documents (
    id TEXT PRIMARY KEY,
    attachment_id TEXT NOT NULL,
    content TEXT NOT NULL,
    metadata_json TEXT NOT NULL DEFAULT '{}',
    extractor_name TEXT NOT NULL,
    extractor_version TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    FOREIGN KEY (attachment_id) REFERENCES slack_attachments(id) ON DELETE CASCADE
);
