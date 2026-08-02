PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS workflow_definitions (
    id TEXT NOT NULL,
    version INTEGER NOT NULL,
    name TEXT NOT NULL,
    enabled INTEGER NOT NULL,
    definition_json TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    PRIMARY KEY (id, version)
);

CREATE TABLE IF NOT EXISTS workflow_runs (
    id TEXT PRIMARY KEY,
    workflow_id TEXT NOT NULL,
    workflow_version INTEGER NOT NULL,
    request_id TEXT NOT NULL UNIQUE,
    request_json TEXT NOT NULL,
    status TEXT NOT NULL,
    current_step_id TEXT NOT NULL DEFAULT '',
    final_artifact_id TEXT NOT NULL DEFAULT '',
    error_code TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    started_at INTEGER,
    completed_at INTEGER,
    created_at INTEGER NOT NULL,
    FOREIGN KEY (workflow_id, workflow_version) REFERENCES workflow_definitions(id, version)
);

CREATE INDEX IF NOT EXISTS workflow_runs_created_at_idx ON workflow_runs(created_at DESC);

CREATE TABLE IF NOT EXISTS step_runs (
    id TEXT PRIMARY KEY,
    workflow_run_id TEXT NOT NULL,
    step_id TEXT NOT NULL,
    capability TEXT NOT NULL,
    model_profile TEXT NOT NULL DEFAULT '',
    model TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL,
    attempt INTEGER NOT NULL DEFAULT 0,
    input_json TEXT,
    output_json TEXT,
    error_code TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    started_at INTEGER,
    completed_at INTEGER,
    created_at INTEGER NOT NULL,
    UNIQUE (workflow_run_id, step_id),
    FOREIGN KEY (workflow_run_id) REFERENCES workflow_runs(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS artifacts (
    id TEXT PRIMARY KEY,
    workflow_run_id TEXT NOT NULL,
    step_run_id TEXT NOT NULL,
    type TEXT NOT NULL,
    content_json TEXT NOT NULL,
    model TEXT NOT NULL DEFAULT '',
    prompt_version TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    FOREIGN KEY (workflow_run_id) REFERENCES workflow_runs(id) ON DELETE CASCADE,
    FOREIGN KEY (step_run_id) REFERENCES step_runs(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS execution_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    workflow_run_id TEXT NOT NULL,
    step_run_id TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL,
    details_json TEXT NOT NULL DEFAULT '{}',
    created_at INTEGER NOT NULL,
    FOREIGN KEY (workflow_run_id) REFERENCES workflow_runs(id) ON DELETE CASCADE
);
