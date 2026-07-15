-- +goose Up
-- Draft-originated versions need a durable executable snapshot, not only a
-- digest. The columns remain nullable so legacy manually recorded versions
-- and existing rows stay compatible without a speculative backfill.
ALTER TABLE pipeline_versions
    ADD COLUMN IF NOT EXISTS pipeline_id TEXT REFERENCES pipelines(id) ON DELETE RESTRICT,
    ADD COLUMN IF NOT EXISTS definition JSONB;

CREATE INDEX IF NOT EXISTS pipeline_versions_project_pipeline_created_idx
    ON pipeline_versions(project_id, pipeline_id, created_at DESC);

-- +goose Down
DROP INDEX IF EXISTS pipeline_versions_project_pipeline_created_idx;

ALTER TABLE pipeline_versions
    DROP COLUMN IF EXISTS definition,
    DROP COLUMN IF EXISTS pipeline_id;
