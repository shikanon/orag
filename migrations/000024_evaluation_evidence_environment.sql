-- +goose Up
ALTER TABLE project_evaluation_evidence
    ADD COLUMN IF NOT EXISTS environment_kind TEXT;

CREATE INDEX IF NOT EXISTS project_evaluation_evidence_version_environment_created_idx
    ON project_evaluation_evidence(project_id, pipeline_version_id, environment_kind, created_at DESC);

-- +goose Down
DROP INDEX IF EXISTS project_evaluation_evidence_version_environment_created_idx;

ALTER TABLE project_evaluation_evidence
    DROP COLUMN IF EXISTS environment_kind;
