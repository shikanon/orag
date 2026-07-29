-- +goose Up
CREATE TABLE task_queue (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    project_id TEXT,
    type TEXT NOT NULL,
    pool TEXT NOT NULL,
    status TEXT NOT NULL,
    payload JSONB,
    idempotency_key TEXT,
    attempt INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 3,
    locked_resource_type TEXT,
    locked_resource_id TEXT,
    trace_id TEXT,
    created_by TEXT,
    priority INTEGER NOT NULL DEFAULT 0,
    run_after TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    error_code TEXT,
    error_message TEXT,
    last_heartbeat_at TIMESTAMPTZ,
    lease_expires_at TIMESTAMPTZ,
    lease_holder TEXT,
    result_data JSONB
);

CREATE INDEX task_queue_ready_idx
    ON task_queue(pool, status, run_after, priority DESC, created_at)
    WHERE status IN ('queued', 'failed_retryable');

CREATE INDEX task_queue_resource_lock_idx
    ON task_queue(locked_resource_type, locked_resource_id)
    WHERE status IN ('queued', 'leased', 'running', 'cancelling')
      AND locked_resource_type IS NOT NULL;

CREATE UNIQUE INDEX task_queue_idempotency_idx
    ON task_queue(tenant_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;

CREATE INDEX task_queue_project_idx
    ON task_queue(project_id, created_at DESC);

CREATE INDEX task_queue_status_pool_idx
    ON task_queue(pool, status, created_at DESC);

CREATE TABLE task_events (
    id BIGSERIAL PRIMARY KEY,
    task_id TEXT NOT NULL REFERENCES task_queue(id) ON DELETE CASCADE,
    event_type TEXT NOT NULL,
    event_data JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX task_events_task_id_idx
    ON task_events(task_id, created_at);

CREATE TABLE task_artifacts (
    id TEXT PRIMARY KEY,
    task_id TEXT NOT NULL REFERENCES task_queue(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    content_type TEXT,
    size_bytes BIGINT,
    storage_path TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX task_artifacts_task_id_idx
    ON task_artifacts(task_id);

-- +goose Down
DROP INDEX IF EXISTS task_artifacts_task_id_idx;
DROP TABLE IF EXISTS task_artifacts;

DROP INDEX IF EXISTS task_events_task_id_idx;
DROP TABLE IF EXISTS task_events;

DROP INDEX IF EXISTS task_queue_status_pool_idx;
DROP INDEX IF EXISTS task_queue_project_idx;
DROP INDEX IF EXISTS task_queue_idempotency_idx;
DROP INDEX IF EXISTS task_queue_resource_lock_idx;
DROP INDEX IF EXISTS task_queue_ready_idx;
DROP TABLE IF EXISTS task_queue;
