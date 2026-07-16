# Execution Budget Controls Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bound synchronous expensive API work with independently configurable deadlines and fail-fast backpressure.

**Architecture:** `internal/execution` owns per-operation slot accounting and derived contexts. HTTP middleware maps the four Roadmap operation classes to that controller, preserving trace/metrics middleware and returning controlled 429/504 responses.

**Tech Stack:** Go contexts/channels, Hertz middleware, existing configuration and HTTP error contracts.

## Global Constraints

- Never queue unbounded requests in process memory.
- Do not place tenant IDs, queries, prompts, documents, raw errors, or credentials in backpressure responses or metric labels.
- Keep durable job state machines responsible for their own retry/resume semantics.

### Task 1: Define and prove the execution controller

**Files:**
- Create: `internal/execution/controller.go`, `internal/execution/controller_test.go`

- [ ] Define ingestion/query/evaluation/release operation constants with timeout/concurrency budgets.
- [ ] Fail admission immediately when a class has no free slot, cancel admitted contexts at deadline, and isolate slots per class.
- [ ] Run `go test ./internal/execution`; expect PASS.

### Task 2: Apply configuration and HTTP admission

**Files:**
- Modify: `internal/config/config.go`, `.env.example`, `internal/http/router.go`
- Create: `internal/http/execution_middleware.go`, `internal/http/execution_middleware_test.go`

- [ ] Load and validate eight positive environment settings.
- [ ] Classify expensive write routes and return `429 execution_capacity_exhausted`/`Retry-After` or `504 execution_deadline_exceeded`.
- [ ] Run `go test ./internal/http ./internal/config`; expect PASS.

### Task 3: Publish the operator contract

**Files:**
- Modify: `docs/operations.md`, `ROADMAP.md`, `ROADMAP_EN.md`, `CHANGELOG.md`
- Create: `docs/superpowers/specs/2026-07-17-execution-budget-controls-design.md`

- [ ] Document defaults, retry/cancellation semantics, single-instance limit, and capacity-calibration boundary.
- [ ] Run `./scripts/build-docs-site.sh` and the full Go suite; expect PASS.
