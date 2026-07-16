-- +goose Up
ALTER TABLE tutorial_experiment_runs
    ADD COLUMN retrieval_strategy TEXT NOT NULL DEFAULT 'hybrid',
    ADD COLUMN reused_baseline_index BOOLEAN NOT NULL DEFAULT FALSE;

-- +goose Down
ALTER TABLE tutorial_experiment_runs
    DROP COLUMN reused_baseline_index,
    DROP COLUMN retrieval_strategy;
