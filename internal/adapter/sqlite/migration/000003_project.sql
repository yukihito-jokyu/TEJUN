-- +goose Up
ALTER TABLE operation_receipts ADD COLUMN request_hash_version INTEGER NOT NULL DEFAULT 0 CHECK (request_hash_version >= 0);
CREATE TABLE projects (
    project_id TEXT PRIMARY KEY,
    lineage_id TEXT NOT NULL,
    source_project_id TEXT REFERENCES projects(project_id),
    source_procedure_id TEXT,
    connection_id TEXT NOT NULL REFERENCES agent_connections(connection_id),
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    workspace_path TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('preparing', 'ai_running', 'human_waiting', 'procedure_editing', 'completed', 'error', 'archived')),
    current_stage TEXT NOT NULL CHECK (current_stage IN ('preparation', 'execution', 'procedure', 'completed')),
    revision INTEGER NOT NULL CHECK (revision BETWEEN 1 AND 9007199254740991),
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    completed_at INTEGER,
    archived_at INTEGER,
    error_code TEXT
);
CREATE INDEX projects_list_idx ON projects(status, archived_at, updated_at, project_id);

CREATE TABLE preparations (
    project_id TEXT PRIMARY KEY REFERENCES projects(project_id),
    purpose TEXT NOT NULL DEFAULT '',
    completion_criteria_json TEXT NOT NULL DEFAULT '[]',
    intended_users TEXT NOT NULL DEFAULT '',
    revision INTEGER NOT NULL CHECK (revision BETWEEN 1 AND 9007199254740991)
);
CREATE TABLE check_plans (
    project_id TEXT PRIMARY KEY REFERENCES projects(project_id),
    revision INTEGER NOT NULL CHECK (revision BETWEEN 1 AND 9007199254740991)
);
CREATE TABLE check_items (
    check_id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES check_plans(project_id),
    sequence INTEGER NOT NULL,
    title TEXT NOT NULL,
    instruction TEXT NOT NULL,
    expected_result TEXT NOT NULL,
    suggested_command TEXT NOT NULL DEFAULT '',
    ai_required INTEGER NOT NULL,
    human_required INTEGER NOT NULL,
    human_evidence_requirement TEXT NOT NULL,
    UNIQUE(project_id, sequence)
);
CREATE TABLE procedures (
    procedure_id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(project_id),
    revision INTEGER NOT NULL CHECK (revision BETWEEN 1 AND 9007199254740991),
    status TEXT NOT NULL CHECK (status IN ('generating', 'draft', 'checking', 'completed', 'failed')),
    document_json TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    completed_at INTEGER
);
CREATE INDEX procedures_project_idx ON procedures(project_id, status, completed_at);

CREATE TABLE acp_sessions (
    session_id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(project_id),
    agent_session_id TEXT,
    requested_strategy TEXT NOT NULL CHECK (requested_strategy IN ('auto', 'new_session', 'resume_existing')),
    resume_supported INTEGER NOT NULL DEFAULT 0,
    load_supported INTEGER NOT NULL DEFAULT 0,
    recovery_mode TEXT NOT NULL CHECK (recovery_mode IN ('new', 'load', 'resume')),
    state TEXT NOT NULL CHECK (state IN ('connecting', 'connected', 'failed', 'interrupted')),
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    completed_at INTEGER,
    error_code TEXT
);
CREATE INDEX acp_sessions_project_idx ON acp_sessions(project_id, created_at);
CREATE TABLE background_jobs (
    job_id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(project_id),
    kind TEXT NOT NULL CHECK (kind IN ('reconnect', 'export')),
    state TEXT NOT NULL CHECK (state IN ('pending', 'running', 'succeeded', 'failed', 'interrupted')),
    target_id TEXT NOT NULL,
    accepted_at INTEGER NOT NULL,
    completed_at INTEGER,
    error_code TEXT
);
CREATE INDEX background_jobs_active_idx ON background_jobs(project_id, state);
CREATE TABLE exports (
    export_id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(project_id),
    procedure_id TEXT NOT NULL REFERENCES procedures(procedure_id),
    procedure_revision INTEGER NOT NULL,
    format TEXT NOT NULL CHECK (format IN ('markdown', 'pdf')),
    destination_display_name TEXT NOT NULL,
    destination_path TEXT NOT NULL,
    overwrite_confirmed INTEGER NOT NULL DEFAULT 0,
    overwrite_identity_json TEXT,
    destination_metadata_json TEXT NOT NULL,
    state TEXT NOT NULL CHECK (state IN ('pending', 'running', 'succeeded', 'failed')),
    accepted_at INTEGER NOT NULL,
    completed_at INTEGER,
    error_code TEXT
);

-- +goose Down
DROP TABLE exports;
DROP TABLE background_jobs;
DROP TABLE acp_sessions;
DROP TABLE procedures;
DROP TABLE check_items;
DROP TABLE check_plans;
DROP TABLE preparations;
DROP INDEX projects_list_idx;
DROP TABLE projects;
ALTER TABLE operation_receipts DROP COLUMN request_hash_version;
