-- +goose Up
CREATE TABLE IF NOT EXISTS project_evaluation_policies (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    dataset_id TEXT NOT NULL REFERENCES datasets(id) ON DELETE RESTRICT,
    name TEXT NOT NULL,
    version INTEGER NOT NULL CHECK (version > 0),
    gates JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (project_id, name, version)
);

CREATE INDEX IF NOT EXISTS project_evaluation_policies_project_created_idx
    ON project_evaluation_policies (tenant_id, project_id, created_at DESC);

CREATE TABLE IF NOT EXISTS project_evaluation_evidence (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    policy_id TEXT NOT NULL REFERENCES project_evaluation_policies(id) ON DELETE RESTRICT,
    policy_version INTEGER NOT NULL CHECK (policy_version > 0),
    evaluation_run_id TEXT NOT NULL REFERENCES evaluation_runs(id) ON DELETE RESTRICT,
    pipeline_version_id TEXT NOT NULL REFERENCES pipeline_versions(id) ON DELETE RESTRICT,
    content_hash TEXT NOT NULL,
    frozen_input JSONB NOT NULL,
    gate_results JSONB NOT NULL,
    passed BOOLEAN NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS project_evaluation_evidence_version_created_idx
    ON project_evaluation_evidence (project_id, pipeline_version_id, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS project_evaluation_evidence;
DROP TABLE IF EXISTS project_evaluation_policies;
