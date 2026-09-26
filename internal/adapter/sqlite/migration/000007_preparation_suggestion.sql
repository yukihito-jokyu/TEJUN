-- +goose Up
ALTER TABLE preparation_turns ADD COLUMN brief_revision INTEGER NOT NULL DEFAULT 1;
-- +goose Down
ALTER TABLE preparation_turns DROP COLUMN brief_revision;
