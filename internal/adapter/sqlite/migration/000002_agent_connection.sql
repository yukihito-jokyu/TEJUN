-- +goose Up
CREATE TABLE app_settings (
    singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
    default_connection_id TEXT,
    change_sequence INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE agent_connections (
    connection_id TEXT PRIMARY KEY,
    display_name TEXT NOT NULL,
    command TEXT NOT NULL,
    args_json TEXT NOT NULL,
    transport TEXT NOT NULL CHECK (transport = 'stdio'),
    resolved_executable_path TEXT NOT NULL,
    last_verified_at TEXT NOT NULL,
    protocol_version TEXT NOT NULL,
    auth_state TEXT NOT NULL,
    schema_artifact_version TEXT NOT NULL,
    revision INTEGER NOT NULL CHECK (revision > 0)
);

CREATE TABLE agent_jobs (
    job_id TEXT PRIMARY KEY,
    kind TEXT NOT NULL CHECK (kind IN ('authenticate', 'logout')),
    target_id TEXT NOT NULL,
    auth_method_id TEXT NOT NULL,
    method_type TEXT NOT NULL,
    state TEXT NOT NULL CHECK (state IN ('pending', 'running', 'succeeded', 'failed')),
    accepted_at TEXT NOT NULL,
    completed_at TEXT
);

CREATE TABLE operation_receipts (
    scope TEXT NOT NULL,
    operation_id TEXT NOT NULL,
    request_hash TEXT NOT NULL,
    result_json TEXT NOT NULL,
    committed_at TEXT NOT NULL,
    PRIMARY KEY (scope, operation_id)
);

CREATE TABLE elicitation_requests (
    elicitation_request_id TEXT PRIMARY KEY,
    connection_attempt_id TEXT NOT NULL,
    process_generation INTEGER NOT NULL,
    mode TEXT NOT NULL,
    message TEXT NOT NULL,
    state TEXT NOT NULL CHECK (state IN ('pending', 'responding', 'responded', 'failed')),
    action TEXT,
    requested_at TEXT NOT NULL,
    expires_at TEXT,
    responded_at TEXT
);

CREATE TABLE event_outbox (
    event_id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    emitted_at TEXT NOT NULL,
    aggregate_type TEXT NOT NULL,
    aggregate_id TEXT NOT NULL,
    change_sequence INTEGER NOT NULL,
    stream_key TEXT NOT NULL,
    stream_revision INTEGER NOT NULL,
    correlation TEXT NOT NULL,
    payload_json TEXT NOT NULL,
    dispatched_at TEXT
);

INSERT INTO app_settings(singleton) VALUES (1);

-- +goose Down
DROP TABLE event_outbox;
DROP TABLE elicitation_requests;
DROP TABLE operation_receipts;
DROP TABLE agent_jobs;
DROP TABLE agent_connections;
DROP TABLE app_settings;
