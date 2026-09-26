-- +goose Up
ALTER TABLE elicitation_requests ADD COLUMN session_id TEXT;
ALTER TABLE elicitation_requests ADD COLUMN scope_json TEXT NOT NULL DEFAULT '{}';
ALTER TABLE elicitation_requests ADD COLUMN requested_schema_json TEXT;
ALTER TABLE elicitation_requests ADD COLUMN url TEXT;
ALTER TABLE elicitation_requests ADD COLUMN elicitation_id TEXT;
ALTER TABLE projects ADD COLUMN current_session_id TEXT;
INSERT INTO check_plans(project_id,revision)
 SELECT p.project_id,1 FROM projects p LEFT JOIN check_plans c USING(project_id) WHERE c.project_id IS NULL;
CREATE TABLE preparation_sessions (
 session_id TEXT PRIMARY KEY, agent_session_id TEXT NOT NULL DEFAULT '', previous_session_id TEXT NOT NULL DEFAULT '', project_id TEXT NOT NULL REFERENCES projects(project_id), state TEXT NOT NULL,
 modes_json TEXT NOT NULL DEFAULT 'null', config_options_json TEXT NOT NULL DEFAULT '[]', capabilities_json TEXT NOT NULL DEFAULT '{}', permission_mode TEXT NOT NULL DEFAULT 'ask_every_time', permission_revision INTEGER NOT NULL DEFAULT 1,
 revision INTEGER NOT NULL DEFAULT 1, change_sequence INTEGER NOT NULL DEFAULT 0, acp_receive_sequence INTEGER NOT NULL DEFAULT 0,
 protocol_version TEXT NOT NULL DEFAULT '', agent_name TEXT NOT NULL DEFAULT '', agent_version TEXT NOT NULL DEFAULT '',
 started_at TEXT NOT NULL, disconnected_at TEXT
);
CREATE TABLE preparation_messages (
 message_id TEXT PRIMARY KEY, session_id TEXT NOT NULL REFERENCES preparation_sessions(session_id), turn_id TEXT NOT NULL,
 role TEXT NOT NULL, content_json TEXT NOT NULL, status TEXT NOT NULL, created_at TEXT NOT NULL, sequence INTEGER NOT NULL,
 UNIQUE(session_id, sequence)
);
CREATE TABLE preparation_turns (
 turn_id TEXT PRIMARY KEY, session_id TEXT NOT NULL REFERENCES preparation_sessions(session_id), state TEXT NOT NULL,
 accepted_at TEXT NOT NULL, completed_at TEXT
);
CREATE TABLE preparation_jobs (
 job_id TEXT PRIMARY KEY, session_id TEXT NOT NULL REFERENCES preparation_sessions(session_id), turn_id TEXT, target_job_id TEXT NOT NULL DEFAULT '', run_id TEXT NOT NULL DEFAULT '',
 kind TEXT NOT NULL, state TEXT NOT NULL, accepted_at TEXT NOT NULL, completed_at TEXT
);
CREATE TABLE session_configuration_claims (
 session_id TEXT PRIMARY KEY REFERENCES preparation_sessions(session_id), operation_id TEXT NOT NULL, request_hash TEXT NOT NULL, claimed_at TEXT NOT NULL
);
ALTER TABLE executions ADD COLUMN session_id TEXT REFERENCES preparation_sessions(session_id);
ALTER TABLE executions ADD COLUMN purpose TEXT NOT NULL DEFAULT '';
ALTER TABLE executions ADD COLUMN completion_criteria_json TEXT NOT NULL DEFAULT '[]';
ALTER TABLE executions ADD COLUMN intended_users TEXT NOT NULL DEFAULT '';
ALTER TABLE executions ADD COLUMN preparation_revision INTEGER NOT NULL DEFAULT 1;
ALTER TABLE executions ADD COLUMN plan_revision INTEGER NOT NULL DEFAULT 1;
ALTER TABLE executions ADD COLUMN started_at TEXT NOT NULL DEFAULT '';
CREATE TABLE execution_checks (
 execution_id TEXT NOT NULL REFERENCES executions(execution_id), check_id TEXT NOT NULL, sequence INTEGER NOT NULL,
 title TEXT NOT NULL, instruction TEXT NOT NULL, expected_result TEXT NOT NULL, suggested_command TEXT NOT NULL,
 ai_required INTEGER NOT NULL, human_required INTEGER NOT NULL, human_evidence_requirement TEXT NOT NULL,
 PRIMARY KEY(execution_id, check_id)
);
-- +goose Down
ALTER TABLE elicitation_requests DROP COLUMN url;
ALTER TABLE elicitation_requests DROP COLUMN elicitation_id;
ALTER TABLE elicitation_requests DROP COLUMN requested_schema_json;
ALTER TABLE elicitation_requests DROP COLUMN scope_json;
ALTER TABLE elicitation_requests DROP COLUMN session_id;
DROP TABLE execution_checks;
ALTER TABLE executions DROP COLUMN started_at;
ALTER TABLE executions DROP COLUMN plan_revision;
ALTER TABLE executions DROP COLUMN preparation_revision;
ALTER TABLE executions DROP COLUMN intended_users;
ALTER TABLE executions DROP COLUMN completion_criteria_json;
ALTER TABLE executions DROP COLUMN purpose;
ALTER TABLE executions DROP COLUMN session_id;
DROP TABLE session_configuration_claims;
DROP TABLE preparation_jobs;
DROP TABLE preparation_turns;
DROP TABLE preparation_messages;
DROP TABLE preparation_sessions;
ALTER TABLE projects DROP COLUMN current_session_id;
