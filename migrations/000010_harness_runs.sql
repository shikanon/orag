-- +goose Up
CREATE TABLE IF NOT EXISTS harness_runs (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    candidate_id TEXT NOT NULL,
    harness_type TEXT NOT NULL,
    argv JSONB NOT NULL DEFAULT '[]'::jsonb,
    working_dir TEXT NOT NULL DEFAULT '',
    env_redacted JSONB NOT NULL DEFAULT '{}'::jsonb,
    stdout_redacted TEXT NOT NULL DEFAULT '',
    stderr_redacted TEXT NOT NULL DEFAULT '',
    parsed_metrics JSONB NOT NULL DEFAULT '{}'::jsonb,
    exit_code INT NOT NULL DEFAULT 0,
    metrics JSONB NOT NULL DEFAULT '{}'::jsonb,
    artifacts JSONB NOT NULL DEFAULT '{}'::jsonb,
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ended_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS harness_runs_candidate_idx ON harness_runs (candidate_id);

-- +goose Down
DROP INDEX IF EXISTS harness_runs_candidate_idx;
DROP TABLE IF EXISTS harness_runs;
