# Execution Budget Controls Design

**Status:** Implemented and verified on 2026-07-17

**Roadmap:** Stage 3 — production-pilot baseline / data consistency and execution safety

## Decision

ORAG applies independent, fail-fast in-process execution budgets to its synchronous high-cost HTTP operations: ingestion, query, evaluation and release writes. Each class has a positive deadline and fixed concurrent-slot count. There is intentionally no waiting queue.

## Behaviour

If no slot is available, the request returns `429 execution_capacity_exhausted` with `Retry-After: 1`; the handler is not called. An admitted request receives a `context.WithTimeout` child context. When that deadline expires, cooperative downstream parser, storage, model, retrieval and evaluation calls receive cancellation. If a handler returns after expiry, the API writes `504 execution_deadline_exceeded`.

## Scope and limits

The controller protects API-process memory and isolates expensive classes from each other. It does not pretend to be a distributed queue, a tenant-level fairness algorithm, or global rate limiting across replicas. Durable tutorial and optimizer execution retain their own status/CAS/cancel-resume contracts. Operators must set gateway-wide controls and calibrate these defaults to observed provider, PostgreSQL and Qdrant capacity.
