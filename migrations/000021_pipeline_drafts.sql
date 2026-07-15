-- +goose Up
CREATE TABLE IF NOT EXISTS pipelines (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS pipelines_project_updated_idx ON pipelines(project_id, updated_at DESC);

CREATE TABLE IF NOT EXISTS pipeline_drafts (
    pipeline_id TEXT PRIMARY KEY REFERENCES pipelines(id) ON DELETE CASCADE,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    revision BIGINT NOT NULL DEFAULT 0,
    schema_version INTEGER NOT NULL DEFAULT 1,
    definition JSONB NOT NULL DEFAULT '{"nodes":[],"edges":[]}'::jsonb,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE IF EXISTS pipeline_drafts;
DROP TABLE IF EXISTS pipelines;
