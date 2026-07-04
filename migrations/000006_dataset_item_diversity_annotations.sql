-- +goose Up
ALTER TABLE dataset_items
    ADD COLUMN IF NOT EXISTS diversity_annotations JSONB NOT NULL DEFAULT '[]'::jsonb;

-- +goose Down
ALTER TABLE dataset_items
    DROP COLUMN IF EXISTS diversity_annotations;
