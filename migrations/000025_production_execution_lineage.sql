-- +goose Up
-- The active release pointer makes the version selected by an environment
-- auditable even when a version is later promoted or rolled back elsewhere.
ALTER TABLE project_environments
    ADD COLUMN IF NOT EXISTS active_release_id TEXT;

-- Existing active pointers predate the column. Recover their most recent
-- matching transition so they remain auditable after the migration.
UPDATE project_environments e
SET active_release_id = (
    SELECT r.id
    FROM project_releases r
    WHERE r.project_id = e.project_id
      AND r.target_environment = e.kind
      AND r.target_version_id = e.active_version_id
    ORDER BY r.created_at DESC, r.id DESC
    LIMIT 1
)
WHERE COALESCE(e.active_release_id, '') = ''
  AND COALESCE(e.active_version_id, '') <> '';

ALTER TABLE project_release_validations
    ADD COLUMN IF NOT EXISTS dataset_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS evaluation_run_id TEXT NOT NULL DEFAULT '';

ALTER TABLE rag_traces
    ADD COLUMN IF NOT EXISTS project_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS pipeline_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS pipeline_version_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS release_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS environment_kind TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS dataset_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS evaluation_run_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS retrieval_params JSONB NOT NULL DEFAULT '{}'::jsonb;

CREATE INDEX IF NOT EXISTS rag_traces_project_version_created_idx
    ON rag_traces(project_id, pipeline_version_id, created_at DESC);

CREATE INDEX IF NOT EXISTS rag_traces_release_created_idx
    ON rag_traces(release_id, created_at DESC);

-- +goose Down
DROP INDEX IF EXISTS rag_traces_release_created_idx;
DROP INDEX IF EXISTS rag_traces_project_version_created_idx;

ALTER TABLE rag_traces
    DROP COLUMN IF EXISTS retrieval_params,
    DROP COLUMN IF EXISTS evaluation_run_id,
    DROP COLUMN IF EXISTS dataset_id,
    DROP COLUMN IF EXISTS environment_kind,
    DROP COLUMN IF EXISTS release_id,
    DROP COLUMN IF EXISTS pipeline_version_id,
    DROP COLUMN IF EXISTS pipeline_id,
    DROP COLUMN IF EXISTS project_id;

ALTER TABLE project_release_validations
    DROP COLUMN IF EXISTS evaluation_run_id,
    DROP COLUMN IF EXISTS dataset_id;

ALTER TABLE project_environments
    DROP COLUMN IF EXISTS active_release_id;
