-- +goose Up
ALTER TABLE tutorial_experiment_runs
    ADD COLUMN query_expansion_mode TEXT NOT NULL DEFAULT 'none',
    ADD COLUMN multi_query_count INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE tutorial_experiment_runs
    DROP COLUMN multi_query_count,
    DROP COLUMN query_expansion_mode;
