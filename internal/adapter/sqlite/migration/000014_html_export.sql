-- +goose Up
CREATE TABLE exports_new (
    export_id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(project_id),
    procedure_id TEXT NOT NULL REFERENCES procedures(procedure_id),
    procedure_revision INTEGER NOT NULL,
    format TEXT NOT NULL CHECK (format IN ('markdown', 'pdf', 'html')),
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
INSERT INTO exports_new SELECT * FROM exports;
DROP TABLE exports;
ALTER TABLE exports_new RENAME TO exports;

-- +goose Down
CREATE TABLE exports_old (
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
INSERT INTO exports_old SELECT * FROM exports;
DROP TABLE exports;
ALTER TABLE exports_old RENAME TO exports;
