-- +goose Up
ALTER TABLE project_environments
    ADD COLUMN IF NOT EXISTS active_version_id TEXT,
    ADD COLUMN IF NOT EXISTS revision BIGINT NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS pipeline_versions (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    content_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS project_environment_bindings (
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    environment_kind TEXT NOT NULL,
    binding_ref TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (project_id, environment_kind)
);

CREATE TABLE IF NOT EXISTS project_release_validations (
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    version_id TEXT NOT NULL REFERENCES pipeline_versions(id) ON DELETE CASCADE,
    environment_kind TEXT NOT NULL,
    passed BOOLEAN NOT NULL,
    content_hash TEXT NOT NULL,
    validated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (project_id, version_id, environment_kind)
);

CREATE TABLE IF NOT EXISTS project_releases (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    source_version_id TEXT NOT NULL,
    target_version_id TEXT NOT NULL,
    source_environment TEXT NOT NULL,
    target_environment TEXT NOT NULL,
    action TEXT NOT NULL,
    actor TEXT NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS project_releases_project_created_idx
    ON project_releases(project_id, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS project_releases;
DROP TABLE IF EXISTS project_release_validations;
DROP TABLE IF EXISTS project_environment_bindings;
DROP TABLE IF EXISTS pipeline_versions;
ALTER TABLE project_environments
    DROP COLUMN IF EXISTS active_version_id,
    DROP COLUMN IF EXISTS revision;
