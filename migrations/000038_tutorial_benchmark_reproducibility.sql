-- +goose Up
ALTER TABLE tutorial_experiment_runs
    ADD COLUMN pack_manifest_sha256 TEXT NOT NULL DEFAULT '',
    ADD COLUMN runtime_environment_sha256 TEXT NOT NULL DEFAULT '',
    ADD COLUMN build_revision TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE tutorial_experiment_runs
    DROP COLUMN build_revision,
    DROP COLUMN runtime_environment_sha256,
    DROP COLUMN pack_manifest_sha256;
