-- +goose Up
ALTER TABLE tutorial_experiment_runs
    ADD COLUMN contextual_retrieval_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN contextualized_chunk_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN average_context_tokens DOUBLE PRECISION NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE tutorial_experiment_runs
    DROP COLUMN average_context_tokens,
    DROP COLUMN contextualized_chunk_count,
    DROP COLUMN contextual_retrieval_enabled;
