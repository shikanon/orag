-- +goose Up
ALTER TABLE tutorial_experiment_runs
    ADD COLUMN chunk_size_tokens INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN chunk_overlap_tokens INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN indexed_chunk_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN average_chunk_tokens DOUBLE PRECISION NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE tutorial_experiment_runs
    DROP COLUMN average_chunk_tokens,
    DROP COLUMN indexed_chunk_count,
    DROP COLUMN chunk_overlap_tokens,
    DROP COLUMN chunk_size_tokens;
