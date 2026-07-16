-- +goose Up
CREATE TABLE IF NOT EXISTS tutorial_clone_jobs (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    subject_id TEXT NOT NULL,
    project_id TEXT NOT NULL UNIQUE,
    project_name TEXT NOT NULL,
    project_description TEXT NOT NULL DEFAULT '',
    template_id TEXT NOT NULL,
    template_version TEXT NOT NULL,
    pack_tier TEXT NOT NULL CHECK (pack_tier IN ('quick', 'benchmark')),
    idempotency_key TEXT NOT NULL,
    stage TEXT NOT NULL,
    status TEXT NOT NULL,
    attempt INTEGER NOT NULL DEFAULT 1 CHECK (attempt > 0),
    last_error_code TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    UNIQUE (tenant_id, subject_id, template_id, template_version, idempotency_key)
);

CREATE TABLE IF NOT EXISTS tutorial_experiments (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    project_id TEXT NOT NULL UNIQUE REFERENCES projects(id) ON DELETE CASCADE,
    template_id TEXT NOT NULL,
    template_version TEXT NOT NULL,
    pack_tier TEXT NOT NULL CHECK (pack_tier IN ('quick', 'benchmark')),
    pack_status TEXT NOT NULL CHECK (pack_status IN ('pending', 'installing', 'pack_installed', 'failed')),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS tutorial_clone_stage_events (
    id BIGSERIAL PRIMARY KEY,
    job_id TEXT NOT NULL REFERENCES tutorial_clone_jobs(id) ON DELETE CASCADE,
    stage TEXT NOT NULL,
    outcome TEXT NOT NULL,
    detail_code TEXT NOT NULL DEFAULT '',
    occurred_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS tutorial_clone_jobs_tenant_updated_idx
    ON tutorial_clone_jobs (tenant_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS tutorial_clone_stage_events_job_occurred_idx
    ON tutorial_clone_stage_events (job_id, occurred_at ASC, id ASC);

-- +goose Down
DROP TABLE IF EXISTS tutorial_clone_stage_events;
DROP TABLE IF EXISTS tutorial_experiments;
DROP TABLE IF EXISTS tutorial_clone_jobs;
