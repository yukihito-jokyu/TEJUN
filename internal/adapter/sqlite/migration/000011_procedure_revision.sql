-- +goose Up
CREATE TABLE procedure_revision_jobs (
 job_id TEXT PRIMARY KEY,
 procedure_id TEXT NOT NULL REFERENCES procedures(procedure_id),
 turn_id TEXT NOT NULL UNIQUE REFERENCES procedure_turns(turn_id),
 session_id TEXT NOT NULL,
 state TEXT NOT NULL CHECK (state IN ('queued','running','cancellation_requested','completed','failed','cancelled','interrupted')),
 prompt_text TEXT NOT NULL,
 expected_revision INTEGER NOT NULL,
 accepted_at INTEGER NOT NULL,
 completed_at INTEGER,
 error_code TEXT
);
CREATE INDEX procedure_revision_jobs_queue_idx ON procedure_revision_jobs(state,accepted_at);
-- +goose Down
DROP TABLE procedure_revision_jobs;
