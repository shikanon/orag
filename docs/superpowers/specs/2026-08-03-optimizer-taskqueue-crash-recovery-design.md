# Optimizer Task-Queue Crash Recovery Design

**Status:** Implemented on 2026-08-03
**Issue:** #396
**Depends on:** [Optimizer single-flight state transitions](2026-07-15-optimizer-singleflight-design.md)

## Outcome

Optimizer submission and resume now create `optimization.run` work in the dedicated `optimizer` pool instead of starting a fire-and-forget goroutine. PostgreSQL writes the run, candidates, task, and `enqueued` event in one transaction. Returning `202` therefore means recoverable work is already persisted, even if the serving process exits immediately.

The task payload is deliberately versioned and contains only `run_id`. Tenant and project scope come from the task envelope, while the immutable execution request is reconstructed from the run. `locked_resource_type=optimization_run` and `locked_resource_id=<run_id>` prevent concurrent tasks for the same run.

## Ownership invariants

Every execution write is authorized by all of the following persisted facts:

1. the run's `current_task_id` equals the leased task;
2. the run's `execution_generation` equals the worker's lease generation;
3. the task still has that lease holder and generation, is leased/running (or cancelling during cleanup), and its lease has not expired.

The handler atomically advances the run to a new generation before calling the candidate runner. Generation 1 starts a queued run. A higher generation may recover a run left running by the same task after its older lease expired. The recovery transaction resets only incomplete partial candidate states (`running`, `evaluated`, or `judged`) that are absent from the completed checkpoint; scored/promoted/holdout/cleanup-complete candidates are never rerun. A late write from the old worker returns `optimization execution ownership lost`.

This gives at-least-once execution only at the explicit incomplete-candidate boundary. Provider calls that completed before their result/checkpoint was persisted may be repeated; completed checkpoint entries are exactly-once with respect to optimizer scheduling.

## Cancellation and lifecycle

Cancellation locks the run and current task in one transaction. Queued/retryable tasks, plus a leased task that has not passed the worker's initial running heartbeat, become canceled immediately with their run. An active running task and run move to cancelling; the worker cancels the handler, uses a short context that does not inherit request cancellation to persist cleanup and the canceled checkpoint, and then acknowledges task cancellation. Repeated cancellation is idempotent.

The application registers an independent optimizer worker pool before starting workers. `EXECUTION_OPTIMIZER_CONCURRENCY`, `EXECUTION_OPTIMIZER_LEASE_DURATION`, and `EXECUTION_OPTIMIZER_HEARTBEAT_INTERVAL` control it; heartbeat must be shorter than the lease. Application shutdown cancels worker contexts and waits for handlers. An interrupted task remains leased until expiry and is then recoverable by another process.

## Migration and rollback

Stop binaries that still use the old goroutine executor before applying migration `000043_optimizer_task_execution.sql`. The migration creates tasks for legacy queued/running runs, resets incomplete partial candidates, and deterministically terminates orphaned canceling runs. Starting an old binary again after migration would bypass lease ownership and is unsupported.

Safe rollback order is: stop all new workers; verify no optimizer task is leased/running/cancelling; preserve a database backup; roll back the application; then roll back the migration. Do not drop ownership columns while a new worker is active.

## Operations and verification

Monitor optimizer pool counts by task status, attempt/generation growth, heartbeat age, failed-terminal/dead-letter tasks, and divergence between terminal task and run states. Repeated generation increases identify crashes or lease sizing problems; dead letters require inspecting the task timeline and run checkpoint before manual retry.

Primary verification is `go test ./...`. PostgreSQL recovery verification additionally races two workers after forcing a lease expiry and asserts one higher generation, fenced stale writes, one call for each completed candidate, and consistent terminal task/run state. Process-level verification kills the first API at queued and running checkpoints, starts another API on the same PostgreSQL database, and polls the original run without calling resume until it is terminal.
