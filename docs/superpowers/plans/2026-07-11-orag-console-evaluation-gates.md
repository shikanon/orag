# ORAG Console Evaluation Gates Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add project-scoped evaluation policies, frozen asynchronous evaluation runs, mandatory gates, and immutable pipeline-version creation.

**Architecture:** Evaluation policies reference existing datasets but add project ownership and explicit thresholds. Starting a run freezes the draft, dataset, policy, bindings, and content hash. Version creation is a transaction allowed only when every recorded gate passed.

**Tech Stack:** Go, PostgreSQL, existing `internal/eval`, Hertz, SSE, OpenAPI, React, TanStack Query, Vitest, Playwright.

## Global Constraints

- Gates cannot be overridden through UI or API.
- Evaluation retries create linked runs and never mutate prior evidence.
- SSE reconnect observes existing run state and never resubmits work.
- Use `GOTOOLCHAIN=go1.26.4` for direct Go commands outside Make targets.

---

### Task 1: Evaluation policy and frozen-run domain

**Files:**
- Create: `internal/evaluationpolicy/types.go`, `service.go`, `service_test.go`
- Create: `migrations/000018_project_evaluation.sql`
- Create: `internal/storage/postgres/project_evaluation.go`, `project_evaluation_test.go`

**Interfaces:**
- Produces: `EvaluationPolicy`, `Gate`, `FrozenInput`, `GateResult`, and project-scoped repository methods.

- [ ] Add failing tests that reject unknown metrics, empty datasets, duplicate gates, invalid comparison operators, and foreign-project datasets.
- [ ] Run `go test ./internal/evaluationpolicy ./internal/storage/postgres -run TestEvaluation -v`; expected: FAIL.
- [ ] Implement whitelisted metric gates, immutable policy versions, JSONB frozen inputs, and append-only gate results linked to existing evaluation run IDs.
- [ ] Re-run focused tests; expected: PASS.
- [ ] Commit with `git commit -m "feat: add project evaluation policies"`.

### Task 2: Asynchronous project evaluation orchestration

**Files:**
- Create: `internal/evaluationpolicy/runner.go`, `runner_test.go`
- Modify: `internal/eval/service.go`, `internal/eval/service_test.go`
- Modify: `internal/app/app.go`

**Interfaces:**
- Produces: `Runner.Submit`, `Runner.Get`, `Runner.Subscribe`, and `Runner.Retry`.

- [ ] Add tests proving submission freezes revision and dataset items, later draft edits do not change results, reconnect returns existing progress, and retry links a new run.
- [ ] Run focused tests; expected: FAIL.
- [ ] Wrap the existing evaluation runner with persisted queued/running/completed/failed state and ordered progress events.
- [ ] Re-run tests; expected: PASS.
- [ ] Commit with `git commit -m "feat: orchestrate frozen project evaluations"`.

### Task 3: Hard-gate enforcement and immutable version creation

**Files:**
- Create: `internal/pipeline/version.go`, `version_test.go`
- Modify: `internal/storage/postgres/pipeline.go`, `pipeline_test.go`

**Interfaces:**
- Produces: `VersionService.CreateFromEvaluation(ctx, tenantID, projectID, pipelineID, runID string) (PipelineVersion, error)`.

- [ ] Add tests for incomplete run, failed gate, mismatched content hash, successful creation, and duplicate request idempotency.
- [ ] Run `go test ./internal/pipeline -run TestCreateVersion -v`; expected: FAIL.
- [ ] Insert immutable version, definition, node-schema versions, policy version, run ID, and content hash in one transaction after rechecking every gate.
- [ ] Re-run tests; expected: PASS.
- [ ] Commit with `git commit -m "feat: enforce gates for pipeline versions"`.

### Task 4: Evaluation APIs and console

**Files:**
- Create: `internal/http/project_evaluations.go`
- Modify: `internal/http/router.go`, `router_test.go`, `api/openapi.yaml`, `tests/contract/openapi_test.go`
- Create: `console/src/features/evaluations/{evaluation-page,policy-form,run-progress,gate-results,comparison-table}.tsx`
- Create tests and `console/e2e/evaluation-gates.spec.ts`

**Interfaces:**
- Produces: project evaluation policy CRUD, submit/status/events/retry, and pipeline version creation endpoints plus `/projects/:projectId/evaluations`.

- [ ] Add failing backend contract tests and frontend tests for blocked and successful gates.
- [ ] Run `make openapi-validate` and frontend evaluation tests; expected: FAIL.
- [ ] Implement handlers, SSE resume by event sequence, generated client updates, policy UI, progress UI, evidence drill-down, and candidate creation.
- [ ] Run `make openapi-validate && go test ./internal/http ./internal/evaluationpolicy ./internal/pipeline -v && npm --prefix console run test:e2e -- evaluation-gates.spec.ts`; expected: PASS.
- [ ] Commit with `git commit -m "feat: add evaluation center and hard gates"`.
