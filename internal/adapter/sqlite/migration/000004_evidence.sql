-- +goose Up
CREATE TABLE executions (
    execution_id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(project_id),
    revision INTEGER NOT NULL CHECK (revision > 0)
);
CREATE TABLE evidence_blobs (
    hash TEXT PRIMARY KEY CHECK (length(hash) = 64),
    size INTEGER NOT NULL CHECK (size > 0 AND size <= 26214400),
    mime TEXT NOT NULL CHECK (mime IN ('image/png', 'image/jpeg')),
    relative_path TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('pending', 'available', 'corrupt')),
    staging_name TEXT,
    ref_count INTEGER NOT NULL DEFAULT 0 CHECK (ref_count >= 0)
);
CREATE TABLE evidence_records (
    evidence_id TEXT PRIMARY KEY,
    execution_id TEXT NOT NULL REFERENCES executions(execution_id),
    blob_hash TEXT NOT NULL REFERENCES evidence_blobs(hash),
    actor TEXT NOT NULL CHECK (actor IN ('human', 'ai')),
    description TEXT NOT NULL DEFAULT '',
    created_at_us INTEGER NOT NULL
);
CREATE TABLE check_evidence (
    check_id TEXT NOT NULL REFERENCES check_items(check_id),
    evidence_id TEXT NOT NULL REFERENCES evidence_records(evidence_id),
    PRIMARY KEY (check_id, evidence_id)
);
CREATE TABLE procedure_sources (
    procedure_id TEXT PRIMARY KEY REFERENCES procedures(procedure_id),
    execution_id TEXT NOT NULL REFERENCES executions(execution_id)
);
CREATE TABLE procedure_evidence (
    procedure_id TEXT NOT NULL REFERENCES procedure_sources(procedure_id),
    evidence_id TEXT NOT NULL REFERENCES evidence_records(evidence_id),
    PRIMARY KEY (procedure_id, evidence_id)
);

-- +goose Down
DROP TABLE procedure_evidence;
DROP TABLE procedure_sources;
DROP TABLE check_evidence;
DROP TABLE evidence_records;
DROP TABLE evidence_blobs;
DROP TABLE executions;
