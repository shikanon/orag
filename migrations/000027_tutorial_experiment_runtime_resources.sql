-- +goose Up
ALTER TABLE tutorial_experiments
    ADD COLUMN IF NOT EXISTS clone_job_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS runtime_status TEXT NOT NULL DEFAULT 'pending',
    ADD COLUMN IF NOT EXISTS knowledge_base_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS dataset_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS baseline_profile TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS baseline_top_k INTEGER NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS tutorial_experiments_tenant_runtime_idx
    ON tutorial_experiments (tenant_id, runtime_status, updated_at DESC);

-- +goose Down
DROP INDEX IF EXISTS tutorial_experiments_tenant_runtime_idx;
ALTER TABLE tutorial_experiments
    DROP COLUMN IF EXISTS baseline_top_k,
    DROP COLUMN IF EXISTS baseline_profile,
    DROP COLUMN IF EXISTS dataset_id,
    DROP COLUMN IF EXISTS knowledge_base_id,
    DROP COLUMN IF EXISTS runtime_status,
    DROP COLUMN IF EXISTS clone_job_id;
