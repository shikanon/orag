-- +goose Up
ALTER TABLE optimization_runs
    ADD COLUMN current_task_id TEXT REFERENCES task_queue(id),
    ADD COLUMN execution_generation BIGINT NOT NULL DEFAULT 0;

INSERT INTO task_queue(
    id, tenant_id, project_id, type, pool, status, payload,
    idempotency_key, max_attempts, locked_resource_type, locked_resource_id,
    run_after, created_at, updated_at
)
SELECT
    'task_optimizer_recovery_' || md5(r.id), r.tenant_id, r.project_id,
    'optimization.run', 'optimizer', 'queued',
    jsonb_build_object('version', 1, 'run_id', r.id),
    'optimization-recovery:' || r.id, 3, 'optimization_run', r.id,
    NOW(), NOW(), NOW()
FROM optimization_runs r
WHERE r.status IN ('queued', 'running');

INSERT INTO task_events(task_id, event_type, event_data)
SELECT 'task_optimizer_recovery_' || md5(r.id), 'enqueued',
       jsonb_build_object('type', 'optimization.run', 'pool', 'optimizer', 'migration_recovery', true)
FROM optimization_runs r
WHERE r.status IN ('queued', 'running');

UPDATE optimization_candidates c
SET status='queued', updated_at=NOW()
FROM optimization_runs r
WHERE c.optimization_run_id=r.id
  AND r.status='running'
  AND c.status IN ('running', 'evaluated', 'judged')
  AND NOT (COALESCE(r.checkpoint->'completed_candidate_ids', '[]'::jsonb) ? c.id);

UPDATE optimization_runs
SET current_task_id='task_optimizer_recovery_' || md5(id),
    execution_generation=0,
    status='queued',
    checkpoint=jsonb_set(COALESCE(checkpoint, '{}'::jsonb), '{stage}', '"migration_recovery"'::jsonb, true),
    updated_at=NOW()
WHERE status IN ('queued', 'running');

UPDATE optimization_runs
SET status='canceled',
    status_reason=CASE WHEN status_reason='' THEN 'canceled during durable optimizer migration' ELSE status_reason END,
    checkpoint=jsonb_set(COALESCE(checkpoint, '{}'::jsonb), '{stage}', '"canceled"'::jsonb, true),
    updated_at=NOW()
WHERE status='canceling';

-- +goose Down
ALTER TABLE optimization_runs
    DROP COLUMN execution_generation,
    DROP COLUMN current_task_id;

DELETE FROM task_queue WHERE type='optimization.run';
