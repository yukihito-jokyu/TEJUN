-- +goose Up
ALTER TABLE preparation_turns ADD COLUMN plan_revision INTEGER NOT NULL DEFAULT 1;
-- +goose Down
ALTER TABLE preparation_turns DROP COLUMN plan_revision;
