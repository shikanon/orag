# Optimizer Single-Flight State Transition Design

**Status:** Implemented and verified on 2026-07-15

**Roadmap:** Stage 3 — production-pilot baseline / data consistency and execution safety

**Issue:** [#176 Optimizer resume can start duplicate runners](https://github.com/shikanon/orag/issues/176)

## Summary

ORAG will make optimizer execution single-flight across processes by replacing blind run and candidate acquisition writes with compare-and-swap (CAS) transitions in the repository contract. PostgreSQL remains the concurrency authority; the in-memory repository implements the same semantics for tests and embedded use.

`Resume` may move only `failed`, `canceled`, or `budget_stopped` runs to `queued`. Execution may claim only a `queued` run. A selection evaluation may claim only a `queued` or `failed` candidate, while holdout evaluation may claim only the selected `scored` candidate. If another caller changed the state first, the loser receives a conflict and does not call the runner.

## Problem

`Service.Resume` currently reads a run and unconditionally writes it back as `queued`, then starts a goroutine. `Service.run` and `Service.runCandidate` also overwrite status without an expected-state predicate. Two API requests or two workers can therefore observe the same old state and both execute the same candidate.

The required invariant is:

> At most one executor may acquire a given optimization run and candidate attempt from the same persisted state, regardless of how many ORAG instances race to resume or run it.

## Goals

- Reject resume from `queued`, `running`, `canceling`, and `completed`.
- Atomically resume only `failed`, `canceled`, or `budget_stopped` runs.
- Atomically claim `queued` runs before listing or executing candidates.
- Atomically claim candidates before rate limiting or invoking a model/evaluation runner.
- Return a stable HTTP `409` conflict when the requested state transition loses a race or is invalid for the current state.
- Preserve tenant scoping and existing stored-request/config-invariant checks.
- Prove the behavior in concurrent unit tests and real PostgreSQL integration tests.

## Non-goals

- Distributed leases, heartbeats, or automatic recovery of a process that died while a run remains `running`.
- Re-running a `completed` optimization.
- Changing candidate scoring, budget accounting, holdout selection, or cleanup policy.
- Serializing different optimization runs.
- Relying on a process-local mutex or PostgreSQL advisory lock.

## Selected Approach

### Repository CAS contract

The optimizer repository gains two explicit operations:

```go
CompareAndSwapOptimizationRun(ctx, run, expectedStatus) (bool, error)
CompareAndSwapOptimizationCandidate(ctx, candidate, expectedStatus) (bool, error)
```

Each operation persists the supplied aggregate only when the stored row still has the expected status. It returns `swapped=false, nil` when no row matches. PostgreSQL implements this with `UPDATE ... WHERE ... AND status = $expected` and `RowsAffected() == 1`; the in-memory repository performs the check and update under one mutex.

The existing unconditional update methods remain for post-acquisition progress writes in this issue. CAS is mandatory at every boundary that authorizes new external work.

### Run state transitions

| Operation | Allowed persisted source | Destination | Conflict behavior |
| --- | --- | --- | --- |
| `Resume` | `failed`, `canceled`, `budget_stopped` | `queued` | Return conflict; start no goroutine |
| `run` / `RunPending` | `queued` | `running` | Return conflict; execute no candidate |

`Resume` first reads the run for tenant validation and request reconstruction, validates the immutable candidate-defining configuration, then performs CAS using the status it read. An explicitly non-resumable status is rejected before mutation. Concurrent resumes that both read the same resumable state race at CAS; exactly one succeeds.

The execution goroutine rereads the run and CAS-claims `queued -> running`. This protects both auto-start and repeated `RunPending` calls, including calls originating from different ORAG replicas.

### Candidate acquisition

The phase determines which candidate states authorize new external work:

| Phase | Allowed source | Destination |
| --- | --- | --- |
| selection | `queued`, `failed` | `running` |
| holdout | `scored` | `running` |

The candidate is CAS-claimed before rate limiting and before `Runner.RunCandidate`. A candidate already marked `running` is never reclaimed because ORAG cannot distinguish a live executor from a crashed one without a lease. The successful claim clears an earlier candidate error before retrying a failed candidate.

Run-level CAS is the primary single-flight gate. Candidate CAS is a second persisted guard that prevents duplicate external work if execution paths are added or called independently later.

## Error and HTTP Semantics

State conflicts use an optimizer sentinel wrapped as `apperrors.CodeConflict`, preserving `errors.Is` for service callers while allowing the HTTP layer to return:

- status: `409 Conflict`
- code: `optimization_state_conflict`
- message: identifies the run or candidate state that cannot be acquired

The OpenAPI resume operation documents `409`. Validation errors for changed resume configuration remain `400`; missing runs remain `404`; repository failures remain `500`.

The loser of a race is not treated as success. A conflict is operationally useful because it distinguishes “another executor owns this work” from “the new execution was accepted.”

## Failure and Concurrency Semantics

- A CAS repository error starts no new work and preserves the original error.
- A CAS miss starts no new work and returns a conflict.
- After `Resume` successfully queues a run, failure to start or complete its asynchronous execution leaves a durable `queued` or terminal state that can be inspected.
- Once a run is claimed, later progress writes remain owned by that single executor.
- Cancel racing with resume wins or loses through persisted state: if cancel changes the status first, resume CAS misses; if resume queues first, cancel may mark the queued run canceling and the execution claim then misses.
- Different run IDs remain fully concurrent.

## Alternatives Rejected

### Process-local mutex

A keyed mutex is simple but protects only one process. It does not meet the reference-deployment requirement where multiple API replicas share PostgreSQL.

### PostgreSQL advisory lock

An advisory lock couples service lifecycle to a database connection and provides no durable state explanation after disconnect. It also creates behavior that the memory repository cannot faithfully model. Conditional row updates are shorter-lived and make ownership visible in the existing status column.

### Treat duplicate resume as idempotent success

Returning `202` to every caller hides whether a new execution was accepted. A `409` gives clients and operators a truthful concurrency signal while preserving exactly-once acquisition.

### Reclaim `running` candidates

Blindly changing `running -> running` or `running -> queued` can duplicate an executor that is still alive. Safe crash recovery needs a lease/attempt generation and is intentionally deferred.

## Testing Strategy

### Service tests

- Concurrent resumes of one resumable run produce exactly one accepted transition and one runner execution.
- Resume from `queued`, `running`, `canceling`, or `completed` returns conflict and starts no runner.
- Resume from `failed`, `canceled`, and `budget_stopped` remains supported.
- Concurrent `RunPending` calls claim the run once.
- A candidate CAS miss prevents rate limiting and runner invocation.
- Selection retry claims `failed`; holdout claims `scored`; `running` is never reclaimed.
- Repository errors and conflict sentinels remain discoverable with `errors.Is`.

### PostgreSQL tests

- CAS SQL includes tenant/run or run/candidate identity plus expected status.
- `RowsAffected() == 1` returns true; zero rows returns false.
- A real PostgreSQL concurrency test launches multiple claimers and proves one winner for the same run and candidate.

### HTTP and contract tests

- Resume conflict maps to `409 optimization_state_conflict`.
- OpenAPI includes the `409` response.
- Existing `202`, `400`, and `404` behavior remains covered.

## Rollout

No schema migration is required. The change is compatible with existing status values and makes multi-replica execution safer immediately. Operators seeing `409` should fetch the run: an active/queued run is already owned, while a terminal run can be retried only after observing an allowed resumable state.

Crash recovery for runs stranded in `running` should be addressed separately with leases or explicit operator recovery rather than weakening this single-flight invariant.

## Validation

The implementation passed:

- optimizer, PostgreSQL repository, and HTTP unit suites;
- the complete optimizer race suite, including concurrent resume and `RunPending` tests;
- the repository-wide `make agent-gate` contract, SDK, unit, and vet checks;
- the complete real PostgreSQL + Qdrant integration suite;
- a 16-way real PostgreSQL race proving exactly one successful run claim and one successful candidate claim.

The two subprocess-based harness tests use a five-second success-path timeout so race-instrumented test-binary startup is not mistaken for an external harness timeout. The dedicated 10ms timeout behavior test remains unchanged.

## Acceptance Criteria

- Two simultaneous resume requests cannot start two runners for one optimization run.
- Repeated or simultaneous `RunPending` calls cannot execute the same run twice.
- A model/evaluation runner is invoked only after the candidate CAS succeeds.
- Invalid resume states return a documented `409`, with no persisted reset to `queued`.
- PostgreSQL is the authoritative concurrency boundary and real concurrent tests prove exactly one winner.
- Existing resume-after-budget, resume-after-failure/cancel, holdout, cancel, and checkpoint behavior remains passing.
