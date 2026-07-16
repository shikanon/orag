-- +goose Up
ALTER TABLE tutorial_experiment_runs
    ADD COLUMN context_pack_top_n INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN context_pack_max_tokens INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE tutorial_experiment_runs
    DROP COLUMN context_pack_max_tokens,
    DROP COLUMN context_pack_top_n;
