-- +goose Up
UPDATE executions SET started_at = (
    SELECT strftime('%Y-%m-%dT%H:%M:%fZ', p.created_at / 1000000, 'unixepoch')
    FROM projects p WHERE p.project_id = executions.project_id
) WHERE started_at = '';
ALTER TABLE executions ADD COLUMN status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'completed', 'cancelled'));
ALTER TABLE executions ADD COLUMN active_run_id TEXT;
ALTER TABLE executions ADD COLUMN completed_at TEXT;
ALTER TABLE execution_checks ADD COLUMN ai_status TEXT NOT NULL DEFAULT 'pending' CHECK (ai_status IN ('pending', 'queued', 'running', 'completed', 'failed'));
ALTER TABLE execution_checks ADD COLUMN human_status TEXT NOT NULL DEFAULT 'pending' CHECK (human_status IN ('pending', 'completed'));
ALTER TABLE execution_checks ADD COLUMN ai_checked_at TEXT;
ALTER TABLE execution_checks ADD COLUMN human_checked_at TEXT;
ALTER TABLE execution_checks ADD COLUMN ai_failure_summary TEXT NOT NULL DEFAULT '';

CREATE TABLE execution_runs (
    run_id TEXT PRIMARY KEY, execution_id TEXT NOT NULL REFERENCES executions(execution_id),
    session_id TEXT NOT NULL, job_id TEXT NOT NULL UNIQUE,
    state TEXT NOT NULL CHECK (state IN ('queued', 'running', 'cancellation_requested', 'completed', 'failed', 'cancelled', 'interrupted')),
    accepted_at TEXT NOT NULL, completed_at TEXT
);
CREATE TABLE execution_agent_jobs (
    job_id TEXT PRIMARY KEY, execution_id TEXT NOT NULL REFERENCES executions(execution_id),
    run_id TEXT, turn_id TEXT, session_id TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('checks', 'message')),
    state TEXT NOT NULL CHECK (state IN ('queued', 'running', 'cancellation_requested', 'completed', 'failed', 'cancelled', 'interrupted')),
    prompt_text TEXT NOT NULL, accepted_at TEXT NOT NULL, completed_at TEXT
);
CREATE TABLE execution_run_checks (
    run_id TEXT NOT NULL REFERENCES execution_runs(run_id), check_id TEXT NOT NULL,
    PRIMARY KEY (run_id, check_id)
);
CREATE TABLE execution_turns (
    turn_id TEXT PRIMARY KEY, execution_id TEXT NOT NULL REFERENCES executions(execution_id),
    role TEXT NOT NULL, text TEXT NOT NULL, status TEXT NOT NULL, created_at TEXT NOT NULL
);
CREATE TABLE execution_permissions (
    permission_request_id TEXT PRIMARY KEY, execution_id TEXT NOT NULL REFERENCES executions(execution_id),
    session_id TEXT NOT NULL, run_id TEXT, tool_call_id TEXT NOT NULL, title TEXT NOT NULL,
    options_json TEXT NOT NULL, rpc_request_id_json TEXT NOT NULL, agent_session_id TEXT NOT NULL,
    process_generation INTEGER NOT NULL, command_digest TEXT NOT NULL,
    state TEXT NOT NULL CHECK (state IN ('pending', 'responding', 'sent', 'expired', 'failed')),
    selected_option_id TEXT, requested_at TEXT NOT NULL, expires_at TEXT, responded_at TEXT
);
CREATE TABLE execution_evidence (
    evidence_id TEXT PRIMARY KEY, execution_id TEXT NOT NULL REFERENCES executions(execution_id),
    check_id TEXT NOT NULL, actor TEXT NOT NULL CHECK (actor IN ('ai', 'human')),
    kind TEXT NOT NULL CHECK (kind IN ('text', 'image')),
    text TEXT NOT NULL DEFAULT '', display_name TEXT NOT NULL DEFAULT '',
    blob_hash TEXT REFERENCES evidence_blobs(hash), staging_name TEXT, created_at TEXT NOT NULL,
    FOREIGN KEY (execution_id, check_id) REFERENCES execution_checks(execution_id, check_id)
);
CREATE TABLE procedure_drafts (
    procedure_id TEXT PRIMARY KEY, execution_id TEXT NOT NULL UNIQUE REFERENCES executions(execution_id),
    source_revision INTEGER NOT NULL, created_at TEXT NOT NULL
);

-- +goose Down
ALTER TABLE execution_checks DROP COLUMN ai_failure_summary;
ALTER TABLE execution_checks DROP COLUMN human_checked_at;
ALTER TABLE execution_checks DROP COLUMN ai_checked_at;
DROP TABLE procedure_drafts;
DROP TABLE execution_evidence;
DROP TABLE execution_permissions;
DROP TABLE execution_turns;
DROP TABLE execution_run_checks;
DROP TABLE execution_agent_jobs;
DROP TABLE execution_runs;
ALTER TABLE execution_checks DROP COLUMN human_status;
ALTER TABLE execution_checks DROP COLUMN ai_status;
ALTER TABLE executions DROP COLUMN completed_at;
ALTER TABLE executions DROP COLUMN active_run_id;
ALTER TABLE executions DROP COLUMN status;
