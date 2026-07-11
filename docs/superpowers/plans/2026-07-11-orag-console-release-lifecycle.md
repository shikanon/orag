# ORAG Console Release Lifecycle Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add immutable development-to-staging-to-production promotion, conflict-safe environment activation, append-only release history, and atomic rollback.

**Architecture:** A release service owns the environment state machine. Promotion rechecks source lineage, successful target gates, bindings, content hash, and expected target version in one transaction. Rollback creates a new audited release while atomically moving the environment pointer to a previously validated version.

**Tech Stack:** Go, PostgreSQL transactions, Hertz, OpenAPI, React, TanStack Query, Vitest, Playwright.

## Global Constraints

- Only development to staging and staging to production promotions are legal.
- Failed gates have no override path.
- Failed transactions leave the target active version unchanged.
- Rollback targets must have prior validation evidence for that environment.
- Use `GOTOOLCHAIN=go1.26.4` for direct Go commands outside Make targets.

---

### Task 1: Release state machine

**Files:**
- Create: `internal/release/types.go`, `service.go`, `service_test.go`

**Interfaces:**
- Produces: `Service.Promote(ctx, PromoteRequest) (Release, error)` and `Service.Rollback(ctx, RollbackRequest) (Release, error)`.

- [ ] Add table-driven failing tests for legal transitions, skipped environments, failed evidence, missing binding, stale expected version, previously validated rollback, and invalid rollback target.
- [ ] Run `go test ./internal/release -v`; expected: FAIL.
- [ ] Implement explicit transition validation and repository transaction interfaces; return typed `ErrGateFailed`, `ErrInvalidTransition`, `ErrConflict`, and `ErrRollbackTarget`.
- [ ] Re-run tests; expected: PASS.
- [ ] Commit with `git commit -m "feat: define release lifecycle"`.

### Task 2: Atomic release persistence and audit

**Files:**
- Create: `migrations/000019_project_releases.sql`
- Create: `internal/storage/postgres/release.go`, `release_test.go`
- Modify: `internal/storage/postgres/project.go`

**Interfaces:**
- Consumes: release repository transaction interface.
- Produces: append-only release records and compare-and-swap active version updates.

- [ ] Add failure-injection tests proving a release insert or audit failure rolls back the environment update and concurrent expected versions yield one success and one conflict.
- [ ] Run focused PostgreSQL tests; expected: FAIL.
- [ ] Add `project_releases`, release evidence JSONB, actor/reason fields, status, and indexes; update environment with `WHERE active_version_id IS NOT DISTINCT FROM expected_active_version_id`.
- [ ] Re-run tests; expected: PASS.
- [ ] Commit with `git commit -m "feat: persist atomic project releases"`.

### Task 3: Release HTTP contract and production query resolution

**Files:**
- Create: `internal/http/releases.go`
- Create: `internal/project/query_resolver.go`, `query_resolver_test.go`
- Modify: `internal/http/router.go`, `router_test.go`, `internal/app/app.go`
- Modify: `api/openapi.yaml`, `tests/contract/openapi_test.go`

**Interfaces:**
- Produces: environment listing, release listing, promote, rollback, and project query resolution endpoints.

- [ ] Add tests that direct API calls cannot bypass gates, stale promotion returns 409, production rejects drafts, and production resolves only `active_version_id`.
- [ ] Run backend and OpenAPI tests; expected: FAIL.
- [ ] Implement handlers and resolver; map typed errors to stable 400/403/409/422 responses; retain existing `/v1/query` behavior.
- [ ] Run `make openapi-validate && go test ./internal/release ./internal/project ./internal/http ./internal/app -v`; expected: PASS.
- [ ] Commit with `git commit -m "feat: expose release lifecycle APIs"`.

### Task 4: Release Center UI and full journey

**Files:**
- Create: `console/src/features/releases/{release-page,environment-card,promotion-dialog,rollback-dialog,release-history}.tsx`
- Create tests beside components
- Create: `console/e2e/release-lifecycle.spec.ts`
- Modify: `console/src/app/router.tsx`

**Interfaces:**
- Produces: `/projects/:projectId/releases` and the complete create-project-through-rollback browser journey.

- [ ] Add failing tests for disabled promotions, evidence display, stale conflict refresh, rollback reason requirement, success notification, and immutable history.
- [ ] Run `npm --prefix console test -- --run releases`; expected: FAIL.
- [ ] Implement environment cards, state-derived actions, promotion and rollback dialogs, conflict refetch, and append-only history.
- [ ] Run `npm --prefix console run typecheck && npm --prefix console test -- --run && npm --prefix console run test:e2e -- release-lifecycle.spec.ts`; expected: PASS.
- [ ] Commit with `git commit -m "feat: add release center and rollback"`.

### Task 5: Repository-wide release gate

**Files:**
- Modify: `README.md`, `docs/README.md`, `docs/api/README.md`, `Makefile`

**Interfaces:**
- Produces: documented Console setup, architecture links, and one `console-gate` Make target.

- [ ] Add a contract test requiring the new docs links and Make target.
- [ ] Run the contract test; expected: FAIL.
- [ ] Document local startup, API generation, project model, hard gates, promotion, rollback, and troubleshooting; add `console-gate` for typecheck, unit tests, build, and Playwright.
- [ ] Run `make test vet openapi-validate console-gate`; expected: all commands exit 0.
- [ ] Commit with `git commit -m "docs: add ORAG console operations guide"`.
