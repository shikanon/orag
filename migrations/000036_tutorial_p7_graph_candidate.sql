-- +goose Up
ALTER TABLE tutorial_experiment_runs
    ADD COLUMN graph_retrieval_enabled BOOLEAN NOT NULL DEFAULT FALSE;

-- +goose Down
ALTER TABLE tutorial_experiment_runs
    DROP COLUMN graph_retrieval_enabled;
