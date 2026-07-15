-- +goose Up
ALTER TABLE tutorial_experiments
    ADD COLUMN IF NOT EXISTS pack_manifest JSONB NOT NULL DEFAULT '{}'::jsonb;

-- +goose Down
ALTER TABLE tutorial_experiments DROP COLUMN IF EXISTS pack_manifest;
