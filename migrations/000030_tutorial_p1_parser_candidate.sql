-- +goose Up
ALTER TABLE tutorial_experiment_runs DROP CONSTRAINT tutorial_experiment_runs_variant_check;
ALTER TABLE tutorial_experiment_runs
    ADD CONSTRAINT tutorial_experiment_runs_variant_check CHECK (variant ~ '^[a-z][a-z0-9_]{0,63}$'),
    ADD COLUMN baseline_run_id TEXT REFERENCES tutorial_experiment_runs(id),
    ADD COLUMN comparison_fingerprint TEXT NOT NULL DEFAULT '',
    ADD COLUMN definition_fingerprint TEXT NOT NULL DEFAULT '',
    ADD COLUMN knowledge_base_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN dataset_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN profile TEXT NOT NULL DEFAULT '',
    ADD COLUMN top_k INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN parser_method TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS tutorial_experiment_runs_compatible_baseline_idx
    ON tutorial_experiment_runs (tenant_id, project_id, experiment_id, variant, comparison_fingerprint, updated_at DESC)
    WHERE status = 'completed';

-- +goose Down
DROP INDEX IF EXISTS tutorial_experiment_runs_compatible_baseline_idx;
ALTER TABLE tutorial_experiment_runs DROP CONSTRAINT tutorial_experiment_runs_variant_check;
DELETE FROM tutorial_experiment_runs WHERE variant <> 'baseline';
ALTER TABLE tutorial_experiment_runs
    ADD CONSTRAINT tutorial_experiment_runs_variant_check CHECK (variant IN ('baseline')),
    DROP COLUMN parser_method,
    DROP COLUMN top_k,
    DROP COLUMN profile,
    DROP COLUMN dataset_id,
    DROP COLUMN knowledge_base_id,
    DROP COLUMN definition_fingerprint,
    DROP COLUMN comparison_fingerprint,
    DROP COLUMN baseline_run_id;
