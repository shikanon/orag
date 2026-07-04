-- +goose Up
CREATE TABLE IF NOT EXISTS optimization_runs (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    dataset_id TEXT NOT NULL REFERENCES datasets(id),
    knowledge_base_id TEXT NOT NULL REFERENCES knowledge_bases(id),
    objective JSONB NOT NULL DEFAULT '{}'::jsonb,
    search_space JSONB NOT NULL DEFAULT '{}'::jsonb,
    runner JSONB NOT NULL DEFAULT '{}'::jsonb,
    status TEXT NOT NULL,
    status_reason TEXT NOT NULL DEFAULT '',
    best_candidate_id TEXT NOT NULL DEFAULT '',
    holdout_candidate_id TEXT NOT NULL DEFAULT '',
    sampling_strategy TEXT NOT NULL DEFAULT 'random',
    search_space_size BIGINT NOT NULL DEFAULT 0,
    sampled_candidate_count INT NOT NULL DEFAULT 0,
    completed_candidate_count INT NOT NULL DEFAULT 0,
    checkpoint JSONB NOT NULL DEFAULT '{}'::jsonb,
    token_usage JSONB NOT NULL DEFAULT '{}'::jsonb,
    cost_usd DOUBLE PRECISION NOT NULL DEFAULT 0,
    cost_budget_usd DOUBLE PRECISION,
    cancel_requested_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS optimization_candidates (
    id TEXT PRIMARY KEY,
    optimization_run_id TEXT NOT NULL REFERENCES optimization_runs(id),
    config JSONB NOT NULL DEFAULT '{}'::jsonb,
    status TEXT NOT NULL,
    evaluation_run_id TEXT NOT NULL DEFAULT '',
    judge_run_id TEXT NOT NULL DEFAULT '',
    objective_score DOUBLE PRECISION NOT NULL DEFAULT 0,
    holdout_score DOUBLE PRECISION,
    confidence JSONB NOT NULL DEFAULT '{}'::jsonb,
    metrics JSONB NOT NULL DEFAULT '{}'::jsonb,
    token_usage JSONB NOT NULL DEFAULT '{}'::jsonb,
    cost_usd DOUBLE PRECISION NOT NULL DEFAULT 0,
    artifacts JSONB NOT NULL DEFAULT '{}'::jsonb,
    temp_namespaces JSONB NOT NULL DEFAULT '[]'::jsonb,
    cleanup_status TEXT NOT NULL DEFAULT 'not_required',
    expires_at TIMESTAMPTZ,
    error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS optimization_runs_tenant_status_idx ON optimization_runs (tenant_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS optimization_candidates_run_score_idx ON optimization_candidates (optimization_run_id, objective_score DESC);
CREATE INDEX IF NOT EXISTS optimization_candidates_run_status_idx ON optimization_candidates (optimization_run_id, status, created_at);
CREATE INDEX IF NOT EXISTS optimization_candidates_cleanup_idx ON optimization_candidates (cleanup_status, expires_at);

-- +goose Down
DROP INDEX IF EXISTS optimization_candidates_cleanup_idx;
DROP INDEX IF EXISTS optimization_candidates_run_status_idx;
DROP INDEX IF EXISTS optimization_candidates_run_score_idx;
DROP INDEX IF EXISTS optimization_runs_tenant_status_idx;

DROP TABLE IF EXISTS optimization_candidates;
DROP TABLE IF EXISTS optimization_runs;
