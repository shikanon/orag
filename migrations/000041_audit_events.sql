-- +goose Up
CREATE TABLE audit_events (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    project_id TEXT,
    actor_type TEXT NOT NULL,
    actor_id TEXT NOT NULL,
    action TEXT NOT NULL,
    resource_type TEXT,
    resource_id TEXT,
    outcome TEXT NOT NULL,
    trace_id TEXT,
    task_id TEXT,
    idempotency_key TEXT,
    metadata JSONB,
    error_code TEXT,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX audit_events_scope_idx
    ON audit_events(tenant_id, created_at DESC);

CREATE INDEX audit_events_project_idx
    ON audit_events(project_id, created_at DESC)
    WHERE project_id IS NOT NULL;

CREATE INDEX audit_events_resource_idx
    ON audit_events(resource_type, resource_id, created_at DESC)
    WHERE resource_type IS NOT NULL;

CREATE INDEX audit_events_action_idx
    ON audit_events(action, created_at DESC);

CREATE INDEX audit_events_trace_idx
    ON audit_events(trace_id)
    WHERE trace_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS audit_events_trace_idx;
DROP INDEX IF EXISTS audit_events_action_idx;
DROP INDEX IF EXISTS audit_events_resource_idx;
DROP INDEX IF EXISTS audit_events_project_idx;
DROP INDEX IF EXISTS audit_events_scope_idx;
DROP TABLE IF EXISTS audit_events;
