-- +goose Up
ALTER TABLE dataset_items
    ADD COLUMN IF NOT EXISTS split TEXT NOT NULL DEFAULT 'eval',
    ADD COLUMN IF NOT EXISTS weight DOUBLE PRECISION NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS expected_evidence JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN IF NOT EXISTS human_scores JSONB NOT NULL DEFAULT '{}'::jsonb;

-- +goose Down
ALTER TABLE dataset_items
    DROP COLUMN IF EXISTS human_scores,
    DROP COLUMN IF EXISTS expected_evidence,
    DROP COLUMN IF EXISTS weight,
    DROP COLUMN IF EXISTS split;
