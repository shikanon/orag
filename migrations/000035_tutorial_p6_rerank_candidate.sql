-- +goose Up
ALTER TABLE tutorial_experiment_runs
    ADD COLUMN rerank_enabled BOOLEAN NOT NULL DEFAULT FALSE;

-- +goose Down
ALTER TABLE tutorial_experiment_runs
    DROP COLUMN rerank_enabled;
