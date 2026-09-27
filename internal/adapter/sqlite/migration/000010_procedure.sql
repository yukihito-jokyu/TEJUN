-- +goose Up
ALTER TABLE procedures ADD COLUMN updated_at INTEGER NOT NULL DEFAULT 0;
ALTER TABLE procedure_sources ADD COLUMN execution_revision INTEGER NOT NULL DEFAULT 1;
ALTER TABLE procedure_sources ADD COLUMN check_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE procedure_sources ADD COLUMN evidence_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE procedure_sources ADD COLUMN captured_at INTEGER NOT NULL DEFAULT 0;
CREATE TABLE procedure_source_evidence (
    procedure_id TEXT NOT NULL REFERENCES procedure_sources(procedure_id),
    evidence_id TEXT NOT NULL REFERENCES execution_evidence(evidence_id),
    PRIMARY KEY (procedure_id, evidence_id)
);
CREATE TABLE procedure_turns (
    turn_id TEXT PRIMARY KEY,
    procedure_id TEXT NOT NULL REFERENCES procedures(procedure_id),
    role TEXT NOT NULL CHECK (role IN ('user', 'agent', 'thought', 'system')),
    content_json TEXT NOT NULL,
    status TEXT NOT NULL,
    created_at INTEGER NOT NULL
);
CREATE INDEX procedure_turns_page_idx ON procedure_turns(procedure_id, created_at DESC, turn_id DESC);

-- +goose Down
DROP TABLE procedure_turns;
DROP TABLE procedure_source_evidence;
ALTER TABLE procedure_sources DROP COLUMN captured_at;
ALTER TABLE procedure_sources DROP COLUMN evidence_count;
ALTER TABLE procedure_sources DROP COLUMN check_count;
ALTER TABLE procedure_sources DROP COLUMN execution_revision;
ALTER TABLE procedures DROP COLUMN updated_at;
