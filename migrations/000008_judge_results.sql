-- +goose Up
CREATE TABLE IF NOT EXISTS judge_runs (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    evaluation_run_id TEXT NOT NULL REFERENCES evaluation_runs(id),
    judge_provider TEXT NOT NULL,
    judge_model TEXT NOT NULL,
    prompt_version TEXT NOT NULL,
    rubric_hash TEXT NOT NULL,
    prompt_hash TEXT NOT NULL,
    judge_params_hash TEXT NOT NULL,
    mode TEXT NOT NULL,
    comparison_mode TEXT NOT NULL DEFAULT 'absolute',
    rubric JSONB NOT NULL DEFAULT '{}'::jsonb,
    judge_params JSONB NOT NULL DEFAULT '{}'::jsonb,
    ensemble JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS judge_results (
    id TEXT PRIMARY KEY,
    judge_run_id TEXT NOT NULL REFERENCES judge_runs(id),
    dataset_item_id TEXT NOT NULL REFERENCES dataset_items(id),
    candidate_id TEXT NOT NULL DEFAULT '',
    scores JSONB NOT NULL DEFAULT '{}'::jsonb,
    pass BOOLEAN NOT NULL DEFAULT false,
    rationale TEXT NOT NULL DEFAULT '',
    findings JSONB NOT NULL DEFAULT '[]'::jsonb,
    raw_response TEXT NOT NULL DEFAULT '',
    parsed_response JSONB NOT NULL DEFAULT '{}'::jsonb,
    confidence JSONB NOT NULL DEFAULT '{}'::jsonb,
    token_usage JSONB NOT NULL DEFAULT '{}'::jsonb,
    cost_usd DOUBLE PRECISION NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS pairwise_judge_results (
    id TEXT PRIMARY KEY,
    judge_run_id TEXT NOT NULL REFERENCES judge_runs(id),
    dataset_item_id TEXT NOT NULL REFERENCES dataset_items(id),
    candidate_a_id TEXT NOT NULL,
    candidate_b_id TEXT NOT NULL,
    winner TEXT NOT NULL,
    preference TEXT NOT NULL,
    reasons JSONB NOT NULL DEFAULT '[]'::jsonb,
    raw_response TEXT NOT NULL DEFAULT '',
    parsed_response JSONB NOT NULL DEFAULT '{}'::jsonb,
    token_usage JSONB NOT NULL DEFAULT '{}'::jsonb,
    cost_usd DOUBLE PRECISION NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS judge_calibration_runs (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    dataset_id TEXT NOT NULL REFERENCES datasets(id),
    judge_config_hash TEXT NOT NULL,
    human_score_version TEXT NOT NULL,
    spearman DOUBLE PRECISION NOT NULL DEFAULT 0,
    cohen_kappa DOUBLE PRECISION NOT NULL DEFAULT 0,
    sample_count INT NOT NULL DEFAULT 0,
    metrics JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS judge_runs_eval_idx ON judge_runs (tenant_id, evaluation_run_id);
CREATE INDEX IF NOT EXISTS judge_runs_hash_idx ON judge_runs (tenant_id, rubric_hash, prompt_hash, judge_params_hash);
CREATE INDEX IF NOT EXISTS judge_results_run_idx ON judge_results (judge_run_id);
CREATE INDEX IF NOT EXISTS pairwise_judge_results_run_idx ON pairwise_judge_results (judge_run_id);
CREATE INDEX IF NOT EXISTS judge_calibration_runs_hash_idx ON judge_calibration_runs (tenant_id, judge_config_hash, created_at DESC);

-- +goose Down
DROP INDEX IF EXISTS judge_calibration_runs_hash_idx;
DROP INDEX IF EXISTS pairwise_judge_results_run_idx;
DROP INDEX IF EXISTS judge_results_run_idx;
DROP INDEX IF EXISTS judge_runs_hash_idx;
DROP INDEX IF EXISTS judge_runs_eval_idx;

DROP TABLE IF EXISTS judge_calibration_runs;
DROP TABLE IF EXISTS pairwise_judge_results;
DROP TABLE IF EXISTS judge_results;
DROP TABLE IF EXISTS judge_runs;
