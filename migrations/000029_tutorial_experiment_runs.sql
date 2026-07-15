-- +goose Up
CREATE TABLE IF NOT EXISTS tutorial_experiment_runs (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    experiment_id TEXT NOT NULL REFERENCES tutorial_experiments(id) ON DELETE CASCADE,
    variant TEXT NOT NULL CHECK (variant IN ('baseline')),
    idempotency_key TEXT NOT NULL,
    stage TEXT NOT NULL CHECK (stage IN ('index_private_pack', 'run_evaluation', 'completed')),
    status TEXT NOT NULL CHECK (status IN ('queued', 'running', 'cancel_requested', 'cancelled', 'failed', 'completed')),
    evaluation_run_id TEXT NOT NULL DEFAULT '',
    failure_code TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    UNIQUE (tenant_id, project_id, variant, idempotency_key)
);

CREATE TABLE IF NOT EXISTS tutorial_experiment_run_events (
    id BIGSERIAL PRIMARY KEY,
    run_id TEXT NOT NULL REFERENCES tutorial_experiment_runs(id) ON DELETE CASCADE,
    stage TEXT NOT NULL,
    outcome TEXT NOT NULL,
    detail_code TEXT NOT NULL DEFAULT '',
    occurred_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS tutorial_experiment_runs_tenant_updated_idx
    ON tutorial_experiment_runs (tenant_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS tutorial_experiment_run_events_run_occurred_idx
    ON tutorial_experiment_run_events (run_id, occurred_at ASC, id ASC);

-- +goose Down
DROP TABLE IF EXISTS tutorial_experiment_run_events;
DROP TABLE IF EXISTS tutorial_experiment_runs;
