-- +goose Up
CREATE TABLE evidence_blob_deletions (
    hash TEXT PRIMARY KEY,
    relative_path TEXT NOT NULL
);

-- +goose Down
DROP TABLE evidence_blob_deletions;
